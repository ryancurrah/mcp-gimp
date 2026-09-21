package commands

import (
	"fmt"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
)

//nolint:gochecknoinits // the command table is assembled from several files
func init() {
	register("select_rectangle", selectRectangle)
	register("select_rounded_rectangle", selectRoundedRectangle)
	register("select_ellipse", selectEllipse)
	register("select_by_color", selectByColor)
	register("select_all", selectAll)
	register("select_none", selectNone)
	register("invert_selection", invertSelection)
	register("modify_selection", modifySelection)
	register("get_selection_bounds", getSelectionBounds)
}

// GimpChannelOps values, in the order libgimp declares them. They are passed
// to the PDB as plain integers, so the order matters: ADD is 0, not REPLACE.
const (
	channelOpAdd = iota
	channelOpSubtract
	channelOpReplace
	channelOpIntersect
)

// channelOps maps the protocol's selection operation names onto
// GimpChannelOps.
var channelOps = map[string]int{
	"add":       channelOpAdd,
	"subtract":  channelOpSubtract,
	"replace":   channelOpReplace,
	"intersect": channelOpIntersect,
}

// channelOp resolves an operation name.
func channelOp(name string) (int, error) {
	op, ok := channelOps[name]
	if !ok {
		return 0, fmt.Errorf("unknown selection operation %q", name)
	}

	return op, nil
}

// applySelectionContext sets the feather and antialias state that the shape
// selection procedures read.
//
// These are not arguments of gimp-image-select-*; GIMP takes them from the
// paint context, so they have to be set before the selection is made.
func applySelectionContext(p Params) error {
	radius := p.Float("feather_radius", 0)
	feather := p.Bool("feather", false) || radius > 0

	if err := run("gimp-context-set-feather",
		gimpbridge.Args{"feather": feather}); err != nil {
		return err
	}

	if feather {
		if err := run("gimp-context-set-feather-radius", gimpbridge.Args{
			"feather-radius-x": radius, "feather-radius-y": radius,
		}); err != nil {
			return err
		}
	}

	return run("gimp-context-set-antialias",
		gimpbridge.Args{"antialias": p.Bool("antialias", true)})
}

// selectRectangle selects a rectangular region.
func selectRectangle(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	op, err := channelOp(p.String("operation", "replace"))
	if err != nil {
		return nil, err
	}

	if err := applySelectionContext(p); err != nil {
		return nil, err
	}

	if err := run("gimp-image-select-rectangle", gimpbridge.Args{
		"image":     image,
		"operation": op,
		"x":         p.Float("x", 0),
		"y":         p.Float("y", 0),
		"width":     p.Float("width", 0),
		"height":    p.Float("height", 0),
	}); err != nil {
		return nil, err
	}

	return selectionState(image)
}

// selectRoundedRectangle selects a rectangle with rounded corners.
func selectRoundedRectangle(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	op, err := channelOp(p.String("operation", "replace"))
	if err != nil {
		return nil, err
	}

	if err := applySelectionContext(p); err != nil {
		return nil, err
	}

	radius := p.Float("radius", 0)

	if err := run(selectRoundRectangleProc, gimpbridge.Args{
		"image":           image,
		"operation":       op,
		"x":               p.Float("x", 0),
		"y":               p.Float("y", 0),
		"width":           p.Float("width", 0),
		"height":          p.Float("height", 0),
		"corner-radius-x": p.Float("radius_x", radius),
		"corner-radius-y": p.Float("radius_y", radius),
	}); err != nil {
		return nil, err
	}

	return selectionState(image)
}

// selectEllipse selects an elliptical region.
func selectEllipse(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	op, err := channelOp(p.String("operation", "replace"))
	if err != nil {
		return nil, err
	}

	if err := applySelectionContext(p); err != nil {
		return nil, err
	}

	if err := run("gimp-image-select-ellipse", gimpbridge.Args{
		"image":     image,
		"operation": op,
		"x":         p.Float("x", 0),
		"y":         p.Float("y", 0),
		"width":     p.Float("width", 0),
		"height":    p.Float("height", 0),
	}); err != nil {
		return nil, err
	}

	return selectionState(image)
}

// selectByColor selects pixels matching a colour.
func selectByColor(p Params) (any, error) {
	image, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	op, err := channelOp(p.String("operation", "replace"))
	if err != nil {
		return nil, err
	}

	color := p.String("color", "")
	if color == "" {
		return nil, fmt.Errorf("color is required")
	}

	if err := run("gimp-context-set-antialias",
		gimpbridge.Args{"antialias": p.Bool("antialias", true)}); err != nil {
		return nil, err
	}

	if err := run("gimp-context-set-sample-threshold-int",
		gimpbridge.Args{"sample-threshold": p.Int("threshold", 15)}); err != nil {
		return nil, err
	}

	if err := run("gimp-image-select-color", gimpbridge.Args{
		"image":     image,
		"operation": op,
		"drawable":  drawable,
		"color":     gimpbridge.Color(color),
	}); err != nil {
		return nil, err
	}

	return selectionState(image)
}

// selectAll selects the whole canvas.
func selectAll(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	if err := run("gimp-selection-all", gimpbridge.Args{"image": image}); err != nil {
		return nil, err
	}

	return selectionState(image)
}

// selectNone clears the selection.
func selectNone(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	if err := run("gimp-selection-none", gimpbridge.Args{"image": image}); err != nil {
		return nil, err
	}

	return selectionState(image)
}

// invertSelection swaps selected and unselected areas.
func invertSelection(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	if err := run("gimp-selection-invert", gimpbridge.Args{"image": image}); err != nil {
		return nil, err
	}

	return selectionState(image)
}

// modifySelection grows, shrinks, feathers, sharpens or borders the selection.
func modifySelection(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	operation := p.String("operation", "")
	amount := p.Float("amount", 0)

	var runErr error

	switch operation {
	case "grow":
		runErr = run("gimp-selection-grow", gimpbridge.Args{"image": image, "steps": int(amount)})
	case "shrink":
		runErr = run("gimp-selection-shrink", gimpbridge.Args{"image": image, "steps": int(amount)})
	case "feather":
		runErr = run("gimp-selection-feather", gimpbridge.Args{"image": image, "radius": amount})
	case "sharpen":
		runErr = run("gimp-selection-sharpen", gimpbridge.Args{"image": image})
	case "border":
		runErr = run("gimp-selection-border", gimpbridge.Args{"image": image, "radius": int(amount)})
	default:
		return nil, fmt.Errorf(
			"unknown operation %q; use grow, shrink, feather, sharpen or border", operation)
	}

	if runErr != nil {
		return nil, runErr
	}

	return selectionState(image)
}

// getSelectionBounds reports the selection rectangle.
func getSelectionBounds(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	return selectionState(image)
}

// selectionState reads the current selection bounds.
func selectionState(image gimpbridge.ObjectID) (any, error) {
	out, err := gimpbridge.Run("gimp-selection-bounds", gimpbridge.Args{"image": image})
	if err != nil {
		return nil, err
	}

	if len(out) < 5 {
		return map[string]any{"non_empty": false}, nil
	}

	nonEmpty, _ := out[0].(bool)
	x1 := intValue(out[1])
	y1 := intValue(out[2])
	x2 := intValue(out[3])
	y2 := intValue(out[4])

	return map[string]any{
		"non_empty": nonEmpty,
		"x1":        x1,
		"y1":        y1,
		"x2":        x2,
		"y2":        y2,
		"width":     x2 - x1,
		"height":    y2 - y1,
	}, nil
}

// intValue coerces a PDB numeric result to int.
func intValue(v gimpbridge.Value) int {
	switch n := v.(type) {
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
