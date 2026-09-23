package commands

import (
	"fmt"

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

	if err := applyStroke(p); err != nil {
		return nil, err
	}

	if err := applyLineStroke(p); err != nil {
		return nil, err
	}

	if err := run(selectProc, shapeSelectArgs(p, selectProc, image)); err != nil {
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

// fillShape selects a shape, fills it and clears the selection.
func fillShape(p Params, selectProc string) (any, error) {
	image, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	if color := p.String("color", ""); color != "" {
		if err := run("gimp-context-set-foreground",
			gimpbridge.Args{"foreground": gimpbridge.Color(color)}); err != nil {
			return nil, err
		}
	}

	if err := run(selectProc, shapeSelectArgs(p, selectProc, image)); err != nil {
		return nil, err
	}

	if err := run("gimp-drawable-edit-fill",
		gimpbridge.Args{"drawable": drawable, "fill-type": "foreground"}); err != nil {
		return nil, err
	}

	if err := run("gimp-selection-none", gimpbridge.Args{"image": image}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
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
