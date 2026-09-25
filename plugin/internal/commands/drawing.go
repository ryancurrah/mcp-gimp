package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
)

//nolint:gochecknoinits // the command table is assembled from several files
func init() {
	register("set_colors", setColors)
	register("draw_line", drawLine)
	register("draw_rectangle", drawRectangle)
	register("draw_ellipse", drawEllipse)
	register("draw_rounded_rectangle", drawRoundedRectangle)
	register("fill_rectangle", fillRectangle)
	register("fill_ellipse", fillEllipse)
	register("fill_rounded_rectangle", fillRoundedRectangle)
	register("gradient_fill", gradientFill)
	register("get_pixel_color", getPixelColor)
	register("get_context_state", getContextState)
}

// setColors sets the foreground and background colours.
func setColors(p Params) (any, error) {
	result := map[string]any{"status": "success"}

	if fg := p.String("foreground", ""); fg != "" {
		if err := run("gimp-context-set-foreground",
			gimpbridge.Args{"foreground": gimpbridge.Color(fg)}); err != nil {
			return nil, err
		}

		result["foreground"] = fg
	}

	if bg := p.String("background", ""); bg != "" {
		if err := run("gimp-context-set-background",
			gimpbridge.Args{"background": gimpbridge.Color(bg)}); err != nil {
			return nil, err
		}

		result["background"] = bg
	}

	return result, nil
}

// applyStroke sets the brush size and colour used by the stroke procedures.
func applyStroke(p Params) error {
	if color := p.String("color", ""); color != "" {
		if err := run("gimp-context-set-foreground",
			gimpbridge.Args{"foreground": gimpbridge.Color(color)}); err != nil {
			return err
		}
	}

	// draw_line calls the stroke width "width"; the shape commands call it
	// "line_width" because "width" is the shape's size.
	width := p.Float("line_width", 0)
	if width <= 0 {
		width = p.Float("width", 0)
	}

	if width > 0 {
		if err := run("gimp-context-set-brush-size",
			gimpbridge.Args{"size": width}); err != nil {
			return err
		}
	}

	return nil
}

// drawLine strokes a straight line, or a polyline when points are given.
func drawLine(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	if err := applyStroke(p); err != nil {
		return nil, err
	}

	coords := p.Floats("points")
	if len(coords) == 0 {
		coords = []float64{
			p.Float("x1", 0), p.Float("y1", 0),
			p.Float("x2", 0), p.Float("y2", 0),
		}
	}

	if len(coords) < 4 || len(coords)%2 != 0 {
		return nil, fmt.Errorf("need at least two x,y pairs to draw a line")
	}

	// The tool decides how the stroke is rendered: pencil gives hard edges,
	// paintbrush gives antialiased ones.
	var proc string

	switch p.String("tool", "pencil") {
	case "pencil":
		proc = "gimp-pencil"
	case "paintbrush", "brush":
		proc = "gimp-paintbrush-default"
	case "airbrush":
		proc = "gimp-airbrush-default"
	case "eraser":
		proc = "gimp-eraser-default"
	default:
		return nil, fmt.Errorf("unknown tool %q", p.String("tool", ""))
	}

	if err := run(proc, gimpbridge.Args{
		"drawable": drawable, "strokes": gimpbridge.Doubles(coords),
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "points": len(coords) / 2}, nil
}

// selectRoundRectangleProc is the PDB procedure behind every rounded
// rectangle command; it takes two extra corner-radius arguments.
const selectRoundRectangleProc = "gimp-image-select-round-rectangle"

// drawRectangle strokes a rectangle outline.
func drawRectangle(p Params) (any, error) {
	return strokeShape(p, "gimp-image-select-rectangle")
}

// drawEllipse strokes an ellipse outline.
func drawEllipse(p Params) (any, error) {
	return strokeShape(p, "gimp-image-select-ellipse")
}

// drawRoundedRectangle strokes a rounded rectangle outline.
func drawRoundedRectangle(p Params) (any, error) {
	return strokeShape(p, selectRoundRectangleProc)
}

// applyLineStroke makes gimp-drawable-edit-stroke-selection draw a plain line
// of an exact width.
//
// The default stroke method drags the active brush along the outline, so the
// result carries the brush's soft edge and spacing: the width varies around
// the shape, and at larger radii and widths the outline visibly breaks up.
// A shape command documents line_width in pixels, so it has to stroke a line.
func applyLineStroke(p Params) error {
	if err := run("gimp-context-set-stroke-method",
		gimpbridge.Args{"stroke-method": "line"}); err != nil {
		return err
	}

	width := p.Float("line_width", 0)
	if width <= 0 {
		width = p.Float("width", 0)
	}

	if width <= 0 {
		width = 2
	}

	return run("gimp-context-set-line-width", gimpbridge.Args{"line-width": width})
}

// shapeSelectArgs builds the arguments for the gimp-image-select-* procedure
// behind a shape command.
func shapeSelectArgs(p Params, selectProc string, image gimpbridge.ObjectID) gimpbridge.Args {
	args := gimpbridge.Args{
		"image":     image,
		"operation": "replace",
		"x":         p.Float("x", 0),
		"y":         p.Float("y", 0),
		"width":     p.Float("width", 0),
		"height":    p.Float("height", 0),
	}

	if selectProc == selectRoundRectangleProc {
		radius := p.Float("radius", 0)
		args["corner-radius-x"] = p.Float("radius_x", radius)
		args["corner-radius-y"] = p.Float("radius_y", radius)
	}

	return args
}

// strokeShape selects a shape then strokes its outline, restoring an empty
// selection afterwards so the shape does not linger as a selection.
func strokeShape(p Params, selectProc string) (any, error) {
	image, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	return withUndoGroup(image, func() (any, error) {
		if err := applyStroke(p); err != nil {
			return nil, err
		}

		if err := applyLineStroke(p); err != nil {
			return nil, err
		}

		if err := selectForDrawing(true, func() error {
			return run(selectProc, shapeSelectArgs(p, selectProc, image))
		}); err != nil {
			return nil, err
		}

		if err := run("gimp-drawable-edit-stroke-selection",
			gimpbridge.Args{"drawable": drawable}); err != nil {
			return nil, err
		}

		if err := run("gimp-selection-none", gimpbridge.Args{"image": image}); err != nil {
			return nil, err
		}

		if err := flush(); err != nil {
			return nil, err
		}

		return map[string]any{"status": "success"}, nil
	})
}

// paintSpec is what a filled shape paints: a fill, an outline or both.
type paintSpec struct {
	// Fill is a CSS colour, "transparent" to clear the shape to alpha, or
	// empty for no fill.
	Fill string
	// Stroke is the outline's CSS colour, or empty for no outline.
	Stroke      string
	StrokeWidth float64
	Join        string
	// Antialias smooths the shape's edges and its outline. Only fill_path
	// takes it as an argument; every other shape is smooth.
	Antialias bool
}

// paintSpecFrom reads the fill and outline arguments the fill tools and
// draw_shapes share. The fill tools call the fill "color", as the rest of the
// tool set does, and the outline "stroke_color", so neither can be mistaken
// for the other.
func paintSpecFrom(p Params) (paintSpec, error) {
	spec := paintSpec{
		Fill:        p.String("color", ""),
		Stroke:      p.String("stroke_color", ""),
		StrokeWidth: p.Float("stroke_width", 2),
		Join:        p.String("stroke_join", "round"),
		Antialias:   p.Bool("antialias", true),
	}

	if spec.Fill == "" && spec.Stroke == "" {
		return paintSpec{}, fmt.Errorf("give color to fill the shape, stroke_color to outline it, or both")
	}

	if isTransparentFill(spec.Stroke) {
		return paintSpec{}, fmt.Errorf(
			"stroke_color %q cannot erase; give color=\"transparent\" to clear the shape", spec.Stroke)
	}

	return spec, nil
}

// clearsToAlpha reports whether a fill colour asks for the shape to be
// cleared rather than painted. Only "transparent" does: fill_layer also reads
// "none" that way, but in SVG, where the shape tools' path data comes from,
// "none" means no fill at all, so it is left to fail as a colour.
func clearsToAlpha(fill string) bool {
	return strings.EqualFold(fill, "transparent")
}

// checkColors parses the spec's colours before anything is drawn, so a bad
// one is refused with the canvas untouched rather than after the fill.
func (spec paintSpec) checkColors() error {
	for _, color := range []string{spec.Fill, spec.Stroke} {
		if color == "" || clearsToAlpha(color) {
			continue
		}

		if _, err := gimpbridge.NormalizeColor(color); err != nil {
			return err
		}
	}

	return nil
}

// withContext runs fn on a copy of GIMP's context, which is put back
// afterwards, on the error path too.
func withContext(fn func() error) error {
	if err := run("gimp-context-push", nil); err != nil {
		return err
	}

	return errors.Join(fn(), run("gimp-context-pop", nil))
}

// selectForDrawing makes a drawing command's own selection with feathering
// off, on a copy of the context.
//
// GIMP's select procedures take feather and antialias from the context
// rather than as arguments, so without this a feather set earlier, by a
// select tool, call_api or the GUI, would soften the shape being drawn.
func selectForDrawing(antialias bool, selectFn func() error) error {
	return withContext(func() error {
		if err := run("gimp-context-set-feather",
			gimpbridge.Args{"feather": false}); err != nil {
			return err
		}

		if err := run("gimp-context-set-antialias",
			gimpbridge.Args{"antialias": antialias}); err != nil {
			return err
		}

		return selectFn()
	})
}

// fillSelectionWith fills the selection on drawable, or clears it to
// transparency when the colour is "transparent".
//
// The fill colour is set on a copy of the context, so the foreground an
// outline or a draw_path without a color strokes with is still in place
// afterwards, and no fill colour is left behind for later commands.
//
// Clearing a layer without an alpha channel would paint the background
// colour, so the layer is given one first; the result reports whether that
// happened.
func fillSelectionWith(drawable gimpbridge.ObjectID, fill string) (alphaAdded bool, err error) {
	if !clearsToAlpha(fill) {
		return false, withContext(func() error {
			if err := run("gimp-context-set-foreground",
				gimpbridge.Args{"foreground": gimpbridge.Color(fill)}); err != nil {
				return err
			}

			return run("gimp-drawable-edit-fill",
				gimpbridge.Args{"drawable": drawable, "fill-type": "foreground"})
		})
	}

	alphaAdded, err = ensureAlpha(drawable)
	if err != nil {
		return false, err
	}

	return alphaAdded, run("gimp-drawable-edit-fill",
		gimpbridge.Args{"drawable": drawable, "fill-type": "transparent"})
}

// ensureAlpha adds an alpha channel to a layer that has none and reports
// whether it did.
func ensureAlpha(layer gimpbridge.ObjectID) (bool, error) {
	v, err := run1("gimp-drawable-has-alpha", gimpbridge.Args{"drawable": layer})
	if err != nil {
		return false, err
	}

	hasAlpha, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("gimp-drawable-has-alpha returned %T, want a boolean", v)
	}

	if hasAlpha {
		return false, nil
	}

	return true, run("gimp-layer-add-alpha", gimpbridge.Args{"layer": layer})
}

// applyOutline sets the context so the stroke procedures draw a plain line in
// the spec's colour, width and join.
//
// It uses the line stroke method for the reason applyLineStroke does. The
// cap is set as well, round like draw_path's default, so an outline never
// depends on what an earlier command left in the context.
func applyOutline(spec paintSpec) error {
	if err := run("gimp-context-set-foreground",
		gimpbridge.Args{"foreground": gimpbridge.Color(spec.Stroke)}); err != nil {
		return err
	}

	if err := run("gimp-context-set-stroke-method",
		gimpbridge.Args{"stroke-method": "line"}); err != nil {
		return err
	}

	if err := run("gimp-context-set-line-width",
		gimpbridge.Args{"line-width": spec.StrokeWidth}); err != nil {
		return err
	}

	if err := run("gimp-context-set-line-join-style",
		gimpbridge.Args{"join-style": spec.Join}); err != nil {
		return err
	}

	if err := run("gimp-context-set-antialias",
		gimpbridge.Args{"antialias": spec.Antialias}); err != nil {
		return err
	}

	return run("gimp-context-set-line-cap-style", gimpbridge.Args{"cap-style": "round"})
}

// paintShape selects a rectangle, rounded rectangle or ellipse, fills it and
// outlines it as the spec says, then clears the selection, on the error path
// too, so a failure leaves nothing selected.
//
// The outline's context is set first, so a value GIMP refuses fails the call
// before the fill has changed any pixels. GIMP strokes a selection on both
// sides of its edge, so the outline is centred on the shape's edge the way
// draw_path's is on its path.
func paintShape(image, drawable gimpbridge.ObjectID, p Params, selectProc string,
	spec paintSpec,
) (alphaAdded bool, err error) {
	if spec.Stroke != "" {
		if err := applyOutline(spec); err != nil {
			return false, err
		}
	}

	if err := selectForDrawing(spec.Antialias, func() error {
		return run(selectProc, shapeSelectArgs(p, selectProc, image))
	}); err != nil {
		return false, err
	}

	defer func() {
		err = errors.Join(err, run("gimp-selection-none", gimpbridge.Args{"image": image}))
	}()

	if spec.Fill != "" {
		if alphaAdded, err = fillSelectionWith(drawable, spec.Fill); err != nil {
			return false, err
		}
	}

	if spec.Stroke == "" {
		return alphaAdded, nil
	}

	return alphaAdded, run("gimp-drawable-edit-stroke-selection",
		gimpbridge.Args{"drawable": drawable})
}

// paintResult is the status object the fill commands return, noting when a
// clear gave the layer an alpha channel, since that changes the layer for
// every later edit.
func paintResult(alphaAdded bool) map[string]any {
	result := map[string]any{"status": "success"}
	if alphaAdded {
		result["alpha_added"] = true
	}

	return result
}

// fillRectangle fills a rectangular area.
func fillRectangle(p Params) (any, error) {
	return fillShape(p, "gimp-image-select-rectangle")
}

// fillEllipse fills an elliptical area.
func fillEllipse(p Params) (any, error) {
	return fillShape(p, "gimp-image-select-ellipse")
}

// fillRoundedRectangle fills a rectangle with rounded corners.
func fillRoundedRectangle(p Params) (any, error) {
	return fillShape(p, selectRoundRectangleProc)
}

// fillShape selects a shape, fills it, outlines it if asked and clears the
// selection.
func fillShape(p Params, selectProc string) (any, error) {
	image, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	spec, err := paintSpecFrom(p)
	if err != nil {
		return nil, err
	}

	if err := spec.checkColors(); err != nil {
		return nil, err
	}

	return withUndoGroup(image, func() (any, error) {
		alphaAdded, err := paintShape(image, drawable, p, selectProc, spec)
		if err != nil {
			return nil, err
		}

		if err := flush(); err != nil {
			return nil, err
		}

		return paintResult(alphaAdded), nil
	})
}

// gradientFill paints a gradient across the drawable.
func gradientFill(p Params) (any, error) {
	image, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	if start := p.String("color1", p.String("start_color", "")); start != "" {
		if err := run("gimp-context-set-foreground",
			gimpbridge.Args{"foreground": gimpbridge.Color(start)}); err != nil {
			return nil, err
		}
	}

	if end := p.String("color2", p.String("end_color", "")); end != "" {
		if err := run("gimp-context-set-background",
			gimpbridge.Args{"background": gimpbridge.Color(end)}); err != nil {
			return nil, err
		}
	}

	width, height, err := imageSize(image)
	if err != nil {
		return nil, err
	}

	// Default to a gradient spanning the canvas left to right.
	x2 := p.Float("x2", float64(width))
	y2 := p.Float("y2", 0)

	if !p.Has("x2") && !p.Has("y2") {
		x2, y2 = float64(width), 0
	}

	if err := run("gimp-context-set-gradient-fg-bg-rgb", nil); err != nil {
		return nil, err
	}

	if err := run("gimp-drawable-edit-gradient-fill", gimpbridge.Args{
		"drawable":              drawable,
		"gradient-type":         p.String("gradient_type", "linear"),
		"offset":                0.0,
		"supersample":           false,
		"supersample-max-depth": 3,
		"supersample-threshold": 0.2,
		"dither":                true,
		"x1":                    p.Float("x1", 0),
		"y1":                    p.Float("y1", 0),
		"x2":                    x2,
		"y2":                    y2,
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	_ = height

	return map[string]any{"status": "success", "gradient_type": p.String("gradient_type", "linear")}, nil
}

// getPixelColor samples one pixel.
func getPixelColor(p Params) (any, error) {
	image, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	x, y := p.Int("x", 0), p.Int("y", 0)

	// Reading a pixel is usually a way of checking what the image looks like,
	// so the composite is the default. Naming a layer says the caller wants
	// that one layer's own pixel, which is a different question: a layer the
	// stack hides still has its own colour there.
	composite := p.Bool("composite", !p.Has("layer_name") && !p.Has("layer_id"))

	var v gimpbridge.Value

	if composite {
		v, err = run1("gimp-image-pick-color", gimpbridge.Args{
			"image":     image,
			"drawables": gimpbridge.Items{drawable},
			"x":         float64(x),
			"y":         float64(y),
			// sample-merged is what reads the composite rather than the
			// drawables, which are then only used to satisfy the signature.
			"sample-merged":  true,
			"sample-average": false,
			"average-radius": 0.0,
		})
	} else {
		v, err = run1("gimp-drawable-get-pixel", gimpbridge.Args{
			"drawable": drawable,
			"x-coord":  x,
			"y-coord":  y,
		})
	}

	if err != nil {
		return nil, err
	}

	css, _ := v.(string)

	return map[string]any{"color": css, "x": x, "y": y, "composite": composite}, nil
}

// getContextState reports the paint context the drawing commands inherit.
func getContextState(_ Params) (any, error) {
	state := map[string]any{}

	for key, proc := range map[string]string{
		"foreground": "gimp-context-get-foreground",
		"background": "gimp-context-get-background",
		"opacity":    "gimp-context-get-opacity",
		"brush_size": "gimp-context-get-brush-size",
		"antialias":  "gimp-context-get-antialias",
		"feather":    "gimp-context-get-feather",
	} {
		if v, err := run1(proc, nil); err == nil {
			state[key] = v
		}
	}

	if v, err := run1("gimp-context-get-brush", nil); err == nil {
		if id, ok := v.(gimpbridge.ObjectID); ok {
			state["brush_id"] = int(id)
		} else {
			state["brush"] = v
		}
	}

	return state, nil
}
