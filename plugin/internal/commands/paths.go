package commands

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
)

//nolint:gochecknoinits // the command table is assembled from several files
func init() {
	register("draw_path", drawPath)
	register("fill_path", fillPath)
	register("select_path", selectPath)
	register("list_paths", listPaths)
	register("path_to_selection", pathToSelection)
}

// svgDocument wraps SVG path data in the smallest document GIMP's importer
// accepts.
//
// The characters that would need escaping inside the attribute are not valid
// in path data anyway, so they are refused rather than escaped: a bad string
// is reported, not silently altered.
func svgDocument(d string) (string, error) {
	if strings.TrimSpace(d) == "" {
		return "", fmt.Errorf(`d is required: SVG path data such as "M 10 10 L 100 100"`)
	}

	if i := strings.IndexAny(d, `"<>&`); i >= 0 {
		return "", fmt.Errorf("d contains %q, which is not valid in SVG path data", d[i])
	}

	return `<svg xmlns="http://www.w3.org/2000/svg"><path d="` + d + `"/></svg>`, nil
}

// importPath turns SVG path data into a path on the image and returns its id.
func importPath(image gimpbridge.ObjectID, d string) (gimpbridge.ObjectID, error) {
	svg, err := svgDocument(d)
	if err != nil {
		return 0, err
	}

	v, err := run1("gimp-image-import-paths-from-string", gimpbridge.Args{
		"image":  image,
		"string": svg,
		"length": -1,
		"merge":  true,
		"scale":  false,
	})
	if err != nil {
		return 0, err
	}

	ids, ok := v.([]gimpbridge.ObjectID)
	if !ok {
		return 0, fmt.Errorf("gimp-image-import-paths-from-string returned %T, want a list of paths", v)
	}

	if len(ids) != 1 {
		return 0, fmt.Errorf("GIMP made %d paths from d, want 1; check the path data", len(ids))
	}

	return ids[0], nil
}

// removePath deletes a path from its image.
func removePath(image, path gimpbridge.ObjectID) error {
	return run("gimp-image-remove-path", gimpbridge.Args{"image": image, "path": path})
}

// withPath imports d, hands the path to fn and then removes it again unless
// keep is set, on the error path too, so a failed stroke leaves no stray path
// in the Paths dockable. The path id is returned so a kept path can be
// reported.
func withPath(image gimpbridge.ObjectID, d string, keep bool,
	fn func(path gimpbridge.ObjectID) error,
) (gimpbridge.ObjectID, error) {
	path, err := importPath(image, d)
	if err != nil {
		return 0, err
	}

	err = fn(path)

	if keep {
		// GIMP imports a path hidden; a kept one is meant to be seen and
		// edited, so it is shown on the canvas like a path drawn in the GUI.
		if err == nil {
			err = run("gimp-item-set-visible", gimpbridge.Args{"item": path, "visible": true})
		}

		return path, err
	}

	if removeErr := removePath(image, path); removeErr != nil && err == nil {
		err = removeErr
	}

	return path, err
}

// pathResult is the status object the path commands return, naming the path
// when it was kept.
func pathResult(path gimpbridge.ObjectID, keep bool) map[string]any {
	result := map[string]any{"status": "success"}
	if keep {
		result["path_id"] = int(path)
	}

	return result
}

// applyPathStroke sets the context so gimp-drawable-edit-stroke-item draws a
// plain line of an exact width.
//
// Like applyLineStroke, it uses the line stroke method rather than the paint
// method, whose brush softness and spacing would make a documented pixel
// width mean something else. Unlike the shape commands, "width" here is the
// stroke width, so there is no line_width fallback.
func applyPathStroke(p Params) error {
	if color := p.String("color", ""); color != "" {
		if err := run("gimp-context-set-foreground",
			gimpbridge.Args{"foreground": gimpbridge.Color(color)}); err != nil {
			return err
		}
	}

	if err := run("gimp-context-set-stroke-method",
		gimpbridge.Args{"stroke-method": "line"}); err != nil {
		return err
	}

	if err := run("gimp-context-set-line-width",
		gimpbridge.Args{"line-width": p.Float("width", 2)}); err != nil {
		return err
	}

	if err := run("gimp-context-set-line-cap-style",
		gimpbridge.Args{"cap-style": p.String("cap", "round")}); err != nil {
		return err
	}

	if err := run("gimp-context-set-line-join-style",
		gimpbridge.Args{"join-style": p.String("join", "round")}); err != nil {
		return err
	}

	return run("gimp-context-set-antialias",
		gimpbridge.Args{"antialias": p.Bool("antialias", true)})
}

// paintedPathResult is pathResult plus what painting the path reported: the
// filled area's bounds, and whether a clear gave the layer an alpha channel.
func paintedPathResult(path gimpbridge.ObjectID, keep, alphaAdded bool,
	bounds map[string]any,
) map[string]any {
	result := pathResult(path, keep)
	if alphaAdded {
		result["alpha_added"] = true
	}

	if bounds != nil {
		result["bounds"] = bounds
	}

	return result
}

// fillPathArea selects a path, fills it, reads the filled area's bounds and
// clears the selection again, on the error path too. Bounds are nil when the
// path encloses no area, such as a single straight line.
func fillPathArea(image, drawable, path gimpbridge.ObjectID, fill string,
) (alphaAdded bool, bounds map[string]any, err error) {
	if err := run("gimp-image-select-item", gimpbridge.Args{
		"image": image, "operation": "replace", "item": path,
	}); err != nil {
		return false, nil, err
	}

	defer func() {
		err = errors.Join(err, run("gimp-selection-none", gimpbridge.Args{"image": image}))
	}()

	if alphaAdded, err = fillSelectionWith(drawable, fill); err != nil {
		return false, nil, err
	}

	state, err := selectionState(image)
	if err != nil {
		return alphaAdded, nil, err
	}

	if nonEmpty, _ := state["non_empty"].(bool); nonEmpty {
		bounds = map[string]any{
			"x": state["x1"], "y": state["y1"], "width": state["width"], "height": state["height"],
		}
	}

	return alphaAdded, bounds, nil
}

// paintPath fills and outlines an imported path as the spec says.
//
// The outline's context is set first, so a value GIMP refuses fails the call
// before the fill has changed any pixels. The fill clears the selection
// before the outline is drawn: GIMP clips a path stroke to the selection, so
// stroking with the path still selected would lose the outline's outer half.
func paintPath(image, drawable, path gimpbridge.ObjectID, spec paintSpec,
) (alphaAdded bool, bounds map[string]any, err error) {
	if spec.Stroke != "" {
		if err := applyOutline(spec); err != nil {
			return false, nil, err
		}
	}

	if spec.Fill != "" {
		if alphaAdded, bounds, err = fillPathArea(image, drawable, path, spec.Fill); err != nil {
			return false, nil, err
		}
	}

	if spec.Stroke == "" {
		return alphaAdded, bounds, nil
	}

	return alphaAdded, bounds, run("gimp-drawable-edit-stroke-item",
		gimpbridge.Args{"drawable": drawable, "item": path})
}

// drawPath strokes the outline of an SVG path on a layer, filling it first
// when asked.
func drawPath(p Params) (any, error) {
	image, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	fill := p.String("fill", "")
	if err := (paintSpec{Fill: fill}).checkColors(); err != nil {
		return nil, err
	}

	return withUndoGroup(image, func() (any, error) {
		// The fill sets its colour on a copy of the context, so the
		// foreground set here, or the current one when color is left out, is
		// still the stroke colour afterwards.
		if err := applyPathStroke(p); err != nil {
			return nil, err
		}

		keep := p.Bool("keep_path", false)

		var (
			alphaAdded bool
			bounds     map[string]any
		)

		path, err := withPath(image, p.String("d", ""), keep, func(path gimpbridge.ObjectID) error {
			if fill != "" {
				var err error
				if alphaAdded, bounds, err = fillPathArea(image, drawable, path, fill); err != nil {
					return err
				}
			}

			return run("gimp-drawable-edit-stroke-item",
				gimpbridge.Args{"drawable": drawable, "item": path})
		})
		if err != nil {
			return nil, err
		}

		if err := flush(); err != nil {
			return nil, err
		}

		return paintedPathResult(path, keep, alphaAdded, bounds), nil
	})
}

// fillPath fills the interior of an SVG path with a solid colour, outlining
// it too when asked.
func fillPath(p Params) (any, error) {
	image, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	if p.String("color", "") == "" {
		return nil, fmt.Errorf("color is required")
	}

	spec, err := paintSpecFrom(p)
	if err != nil {
		return nil, err
	}

	if err := spec.checkColors(); err != nil {
		return nil, err
	}

	return withUndoGroup(image, func() (any, error) {
		if err := run("gimp-context-set-antialias",
			gimpbridge.Args{"antialias": p.Bool("antialias", true)}); err != nil {
			return nil, err
		}

		keep := p.Bool("keep_path", false)

		var (
			alphaAdded bool
			bounds     map[string]any
		)

		path, err := withPath(image, p.String("d", ""), keep, func(path gimpbridge.ObjectID) error {
			var err error

			alphaAdded, bounds, err = paintPath(image, drawable, path, spec)

			return err
		})
		if err != nil {
			return nil, err
		}

		if err := flush(); err != nil {
			return nil, err
		}

		return paintedPathResult(path, keep, alphaAdded, bounds), nil
	})
}

// selectPath renders an SVG path into the selection.
func selectPath(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	if err := applySelectionContext(p); err != nil {
		return nil, err
	}

	keep := p.Bool("keep_path", false)

	path, err := withPath(image, p.String("d", ""), keep, func(path gimpbridge.ObjectID) error {
		return run("gimp-image-select-item", gimpbridge.Args{
			"image": image, "operation": p.String("operation", "replace"), "item": path,
		})
	})
	if err != nil {
		return nil, err
	}

	state, err := selectionState(image)
	if err != nil {
		return nil, err
	}

	if keep {
		state["path_id"] = int(path)
	}

	return state, nil
}

// listPaths lists the paths of an image, such as ones drawn in the GUI or
// kept by draw_path.
func listPaths(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	v, err := run1("gimp-image-get-paths", gimpbridge.Args{"image": image})
	if err != nil {
		return nil, err
	}

	ids, ok := v.([]gimpbridge.ObjectID)
	if !ok {
		return nil, fmt.Errorf("gimp-image-get-paths returned %T, want a list of paths", v)
	}

	described := make([]map[string]any, 0, len(ids))

	for _, id := range ids {
		name, err := itemName(id)
		if err != nil {
			return nil, err
		}

		visible, err := run1("gimp-item-get-visible", gimpbridge.Args{"item": id})
		if err != nil {
			return nil, err
		}

		described = append(described, map[string]any{
			"path_id": int(id), "name": name, "visible": visible,
		})
	}

	return map[string]any{"image_id": int(image), "num_paths": len(ids), "paths": described}, nil
}

// pathByID checks that id names a path in some open image and returns that
// image. Path ids are unique across images, so the id names its image too.
func pathByID(id int) (image, path gimpbridge.ObjectID, err error) {
	if id <= 0 || id > math.MaxInt32 {
		return 0, 0, fmt.Errorf("path_id %d is not a path id", id)
	}

	v, err := run1("gimp-item-id-is-path", gimpbridge.Args{"item-id": int64(id)})
	if err != nil {
		return 0, 0, err
	}

	if isPath, _ := v.(bool); !isPath {
		return 0, 0, fmt.Errorf("path_id %d is not a path in any open image; list_paths shows them", id)
	}

	path = gimpbridge.ObjectID(id)

	v, err = run1("gimp-item-get-image", gimpbridge.Args{"item": path})
	if err != nil {
		return 0, 0, err
	}

	image, err = objectID(v)
	if err != nil {
		return 0, 0, err
	}

	return image, path, nil
}

// pathToSelection renders an existing path into the selection.
func pathToSelection(p Params) (any, error) {
	image, path, err := pathByID(p.Int("path_id", 0))
	if err != nil {
		return nil, err
	}

	if err := applySelectionContext(p); err != nil {
		return nil, err
	}

	if err := run("gimp-image-select-item", gimpbridge.Args{
		"image": image, "operation": p.String("operation", "replace"), "item": path,
	}); err != nil {
		return nil, err
	}

	return selectionState(image)
}
