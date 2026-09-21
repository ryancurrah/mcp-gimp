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
	register("fill_rectangle", fillRectangle)
	register("fill_ellipse", fillEllipse)
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

// drawRectangle strokes a rectangle outline.
func drawRectangle(p Params) (any, error) {
	return strokeShape(p, "gimp-image-select-rectangle")
}

// drawEllipse strokes an ellipse outline.
func drawEllipse(p Params) (any, error) {
	return strokeShape(p, "gimp-image-select-ellipse")
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

	args := gimpbridge.Args{
		"image":     image,
		"operation": channelOpReplace,
		"x":         p.Float("x", 0),
		"y":         p.Float("y", 0),
		"width":     p.Float("width", 0),
		"height":    p.Float("height", 0),
	}

	if err := run(selectProc, args); err != nil {
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

	args := gimpbridge.Args{
		"image":     image,
		"operation": channelOpReplace,
		"x":         p.Float("x", 0),
		"y":         p.Float("y", 0),
		"width":     p.Float("width", 0),
		"height":    p.Float("height", 0),
	}

	if err := run(selectProc, args); err != nil {
		return nil, err
	}

	if err := run("gimp-drawable-edit-fill",
		gimpbridge.Args{"drawable": drawable, "fill-type": 0}); err != nil {
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

// gradientTypes maps protocol gradient names onto GimpGradientType.
var gradientTypes = map[string]int{
	"linear":     0,
	"bilinear":   1,
	"radial":     2,
	"square":     3,
	"conical":    5,
	"shapeburst": 6,
	"spiral":     10,
}

// gradientFill paints a gradient across the drawable.
func gradientFill(p Params) (any, error) {
	image, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	shape, ok := gradientTypes[p.String("gradient_type", "linear")]
	if !ok {
		return nil, fmt.Errorf("unknown gradient_type %q", p.String("gradient_type", ""))
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
		"gradient-type":         shape,
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
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	v, err := run1("gimp-drawable-get-pixel", gimpbridge.Args{
		"drawable": drawable,
		"x-coord":  p.Int("x", 0),
		"y-coord":  p.Int("y", 0),
	})
	if err != nil {
		return nil, err
	}

	css, _ := v.(string)

	return map[string]any{"color": css, "x": p.Int("x", 0), "y": p.Int("y", 0)}, nil
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
