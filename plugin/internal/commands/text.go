package commands

import (
	"fmt"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
)

//nolint:gochecknoinits // the command table is assembled from several files
func init() {
	register("add_text", addText)
	register("edit_text", editText)
	register("list_fonts", listFonts)
}

// addText places a text layer on the image.
func addText(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	content := p.String("text", "")
	if content == "" {
		return nil, fmt.Errorf("text is required")
	}

	if color := p.String("color", ""); color != "" {
		if err := run("gimp-context-set-foreground",
			gimpbridge.Args{"foreground": gimpbridge.Color(color)}); err != nil {
			return nil, err
		}
	}

	size := p.Float("size", 24)
	font := p.String("font", "")

	fontID, err := lookupFont(font)
	if err != nil {
		return nil, err
	}

	v, err := run1("gimp-text-font", gimpbridge.Args{
		"image":     image,
		"drawable":  gimpbridge.ObjectID(-1), // create a new text layer
		"x":         p.Float("x", 0),
		"y":         p.Float("y", 0),
		"text":      content,
		"border":    -1,
		"antialias": p.Bool("antialias", true),
		"size":      size,
		"font":      fontID,
	})
	if err != nil {
		return nil, err
	}

	layer, err := objectID(v)
	if err != nil {
		return nil, err
	}

	if name := p.String("layer_name", ""); name != "" {
		if err := run("gimp-item-set-name",
			gimpbridge.Args{"item": layer, "name": name}); err != nil {
			return nil, err
		}
	}

	// Aligning needs the rendered width, which GIMP only knows once the layer
	// exists, so the layer is placed and then moved.
	if err := alignLayer(image, layer, p.String("align", "left"),
		p.Int("x", 0), p.Int("y", 0)); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	result := map[string]any{
		"status": "success", "layer_id": int(layer),
		"text": content, "font": fontName(fontID), "size": size,
	}

	if name, err := itemName(layer); err == nil {
		result["layer_name"] = name
	}

	// Callers lay text out from the rendered size, which GIMP only knows once
	// the layer exists, so report it alongside where the layer landed.
	if w, h, err := drawableSize(layer); err == nil {
		result["text_width"], result["text_height"] = w, h
	}

	if x, y, err := drawableOffsets(layer); err == nil {
		result["position"] = map[string]int{"x": x, "y": y}
	}

	// Say so when the requested family was not available.
	if font != "" && result["font"] != font {
		result["font_requested"] = font
	}

	return result, nil
}

// lookupFont resolves a font name to the GimpFont resource GIMP 3 expects.
//
// A name that is not installed falls back to GIMP's current font rather than
// failing the whole operation: which families exist varies by machine, and
// losing the text entirely is worse than rendering it in another face. The
// caller is told which font was actually used.
func lookupFont(name string) (gimpbridge.ObjectID, error) {
	if name != "" {
		if v, err := run1("gimp-font-get-by-name", gimpbridge.Args{"name": name}); err == nil {
			if id, err := objectID(v); err == nil {
				return id, nil
			}
		}
	}

	v, err := run1("gimp-context-get-font", nil)
	if err != nil {
		return 0, fmt.Errorf("cannot read the current font: %w", err)
	}

	return objectID(v)
}

// alignLayer positions a layer horizontally within the image.
//
// "left" leaves it at x. "center" ignores x and centres the layer on the image.
// "right" treats x as a margin from the right edge. Vertical placement is
// always y, since text is laid out line by line.
func alignLayer(image, layer gimpbridge.ObjectID, align string, x, y int) error {
	if align == "" || align == "left" {
		return nil
	}

	imageWidth, _, err := imageSize(image)
	if err != nil {
		return err
	}

	layerWidth, _, err := drawableSize(layer)
	if err != nil {
		return err
	}

	x, err = alignOffset(align, imageWidth, layerWidth, x)
	if err != nil {
		return err
	}

	return run("gimp-layer-set-offsets",
		gimpbridge.Args{"layer": layer, "offx": x, "offy": y})
}

// alignOffset works out the x a layer needs to sit left, centred or right
// within a width. For "right", x is the margin held from the right edge.
func alignOffset(align string, imageWidth, layerWidth, x int) (int, error) {
	switch align {
	case "", "left":
		return x, nil
	case "center", "centre":
		return (imageWidth - layerWidth) / 2, nil
	case "right":
		return imageWidth - layerWidth - x, nil
	default:
		return 0, fmt.Errorf("unknown align %q; use left, center or right", align)
	}
}

// drawableSize reports a drawable's pixel dimensions.
func drawableSize(drawable gimpbridge.ObjectID) (width, height int, err error) {
	w, err := run1("gimp-drawable-get-width", gimpbridge.Args{"drawable": drawable})
	if err != nil {
		return 0, 0, err
	}

	h, err := run1("gimp-drawable-get-height", gimpbridge.Args{"drawable": drawable})
	if err != nil {
		return 0, 0, err
	}

	wi, _ := w.(int64)
	hi, _ := h.(int64)

	return int(wi), int(hi), nil
}

// drawableOffsets reports where a drawable sits on the canvas.
func drawableOffsets(drawable gimpbridge.ObjectID) (x, y int, err error) {
	out, err := gimpbridge.Run("gimp-drawable-get-offsets",
		gimpbridge.Args{"drawable": drawable})
	if err != nil {
		return 0, 0, err
	}

	if len(out) < 2 {
		return 0, 0, fmt.Errorf("gimp-drawable-get-offsets returned %d values", len(out))
	}

	xi, _ := out[0].(int64)
	yi, _ := out[1].(int64)

	return int(xi), int(yi), nil
}

// fontName reads a font resource's name.
func fontName(id gimpbridge.ObjectID) string {
	v, err := run1("gimp-resource-get-name", gimpbridge.Args{"resource": id})
	if err != nil {
		return ""
	}

	name, _ := v.(string)

	return name
}

// editText rewrites an existing text layer.
func editText(p Params) (any, error) {
	_, layer, err := target(p)
	if err != nil {
		return nil, err
	}

	if valid, err := run1("gimp-item-is-text-layer",
		gimpbridge.Args{"item": layer}); err == nil {
		if isText, _ := valid.(bool); !isText {
			return nil, fmt.Errorf("layer is not a text layer")
		}
	}

	if content := p.String("text", ""); content != "" {
		if err := run("gimp-text-layer-set-text",
			gimpbridge.Args{"layer": layer, "text": content}); err != nil {
			return nil, err
		}
	}

	if p.Has("size") {
		if err := run("gimp-text-layer-set-font-size", gimpbridge.Args{
			"layer": layer, "font-size": p.Float("size", 24), "unit": 0,
		}); err != nil {
			return nil, err
		}
	}

	if font := p.String("font", ""); font != "" {
		fontID, err := lookupFont(font)
		if err != nil {
			return nil, err
		}

		if err := run("gimp-text-layer-set-font",
			gimpbridge.Args{"layer": layer, "font": fontID}); err != nil {
			return nil, err
		}
	}

	if err := flush(); err != nil {
		return nil, err
	}

	if color := p.String("color", ""); color != "" {
		if err := run("gimp-text-layer-set-color",
			gimpbridge.Args{"layer": layer, "color": gimpbridge.Color(color)}); err != nil {
			return nil, err
		}
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "layer_id": int(layer)}, nil
}

// listFonts reports the fonts GIMP can use.
func listFonts(p Params) (any, error) {
	out, err := gimpbridge.Run("gimp-fonts-get-list",
		gimpbridge.Args{"filter": p.String("filter", "")})
	if err != nil {
		return nil, err
	}

	var names []string

	if len(out) > 0 {
		ids, _ := out[0].([]gimpbridge.ObjectID)

		for _, id := range ids {
			if name := fontName(id); name != "" {
				names = append(names, name)
			}
		}
	}

	return map[string]any{"fonts": names, "count": len(names)}, nil
}
