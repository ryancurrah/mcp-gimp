package commands

import (
	"fmt"
	"regexp"
	"strings"

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

	return withUndoGroup(image, func() (any, error) {
		return addTextLayer(p, image, content)
	})
}

// addTextLayer renders add_text's text as a new layer on image.
func addTextLayer(p Params, image gimpbridge.ObjectID, content string) (any, error) {
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

	if justify := p.String("justify", ""); justify != "" {
		if err := run("gimp-text-layer-set-justification",
			gimpbridge.Args{"layer": layer, "justify": justify}); err != nil {
			return nil, err
		}
	}

	// gimp-text-font puts a new layer directly above the active one, wherever
	// that is in the stack, so the layer is moved to the position asked for.
	// -1 asks for exactly that placement, so it is left alone.
	if position := p.Int("position", 0); position != -1 {
		if err := run("gimp-image-reorder-item", gimpbridge.Args{
			"image": image, "item": layer,
			"parent": gimpbridge.ObjectID(-1), "position": position,
		}); err != nil {
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

	return result, nil
}

// lookupFont resolves a font name to the GimpFont resource GIMP 3 expects.
//
// GIMP names fonts by family and face, "Georgia Bold Italic", so a bare family
// such as "Georgia" is looked up as its Regular face. An empty name means the
// context's current font. Any other name GIMP does not have is refused, and the
// error lists the installed fonts that start with the requested family:
// rendering in a substitute face looks like success and is only noticed in the
// finished image.
func lookupFont(name string) (gimpbridge.ObjectID, error) {
	if name == "" {
		v, err := run1("gimp-context-get-font", nil)
		if err != nil {
			return 0, fmt.Errorf("cannot read the current font: %w", err)
		}

		return objectID(v)
	}

	for _, candidate := range []string{name, name + " Regular"} {
		if v, err := run1("gimp-font-get-by-name", gimpbridge.Args{"name": candidate}); err == nil {
			if id, err := objectID(v); err == nil && id > 0 {
				return id, nil
			}
		}
	}

	return 0, fmt.Errorf("font %q is not installed%s", name, fontSuggestions(name))
}

// fontSuggestions lists up to ten installed fonts whose names start with the
// first word of name, formatted for the end of an error message.
func fontSuggestions(name string) string {
	family, _, _ := strings.Cut(strings.TrimSpace(name), " ")

	out, err := gimpbridge.Run("gimp-fonts-get-list",
		gimpbridge.Args{"filter": "^" + regexp.QuoteMeta(family)})
	if err != nil || len(out) == 0 {
		return "; use list_fonts to see what is"
	}

	ids, _ := out[0].([]gimpbridge.ObjectID)

	var names []string

	for _, id := range ids {
		if n := fontName(id); n != "" {
			names = append(names, n)
		}

		if len(names) == 10 {
			break
		}
	}

	if len(names) == 0 {
		return "; use list_fonts to see what is"
	}

	return "; similar: " + strings.Join(names, ", ")
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
	image, layer, err := target(p)
	if err != nil {
		return nil, err
	}

	valid, err := run1("gimp-item-id-is-text-layer", gimpbridge.Args{"item-id": int64(layer)})
	if err != nil {
		return nil, err
	}

	if isText, _ := valid.(bool); !isText {
		return nil, fmt.Errorf("layer is not a text layer")
	}

	return withUndoGroup(image, func() (any, error) {
		return rewriteTextLayer(p, image, layer)
	})
}

// rewriteTextLayer applies edit_text's changes to a text layer.
func rewriteTextLayer(p Params, image, layer gimpbridge.ObjectID) (any, error) {
	if err := restyleText(p, layer); err != nil {
		return nil, err
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

	if err := checkTextApplied(p, layer); err != nil {
		return nil, err
	}

	// Changing the text or the font changes the rendered width, which leaves
	// a layer that was centred or right-aligned sitting at its old offset.
	// Re-aligning here keeps an edited caption where the caller put it.
	if align := p.String("align", ""); align != "" && align != "left" {
		_, y, err := drawableOffsets(layer)
		if err != nil {
			return nil, err
		}

		if err := alignLayer(image, layer, align, p.Int("x", 0), p.Int("y", y)); err != nil {
			return nil, err
		}
	}

	if err := flush(); err != nil {
		return nil, err
	}

	result := map[string]any{"status": "success", "layer_id": int(layer)}

	// A text layer's name follows its text, so an edit can rename it.
	if name, err := itemName(layer); err == nil {
		result["layer_name"] = name
	}

	// An edit reflows the layer, so report the new box the way add_text does;
	// callers lay the rest of the page out from it.
	if w, h, err := drawableSize(layer); err == nil {
		result["text_width"], result["text_height"] = w, h
	}

	if x, y, err := drawableOffsets(layer); err == nil {
		result["position"] = map[string]int{"x": x, "y": y}
	}

	return result, nil
}

// pixelUnit is GIMP's unit id for pixels, the unit add_text sizes text in.
const pixelUnit = gimpbridge.ObjectID(0)

// checkTextApplied confirms GIMP took edit_text's changes.
//
// Once a text layer's pixels have been changed directly, by a merged filter
// or by painting on it, GIMP still reports it as a text layer and accepts
// every text-layer-set call, but ignores them. Reading the properties back is
// the only way to tell.
func checkTextApplied(p Params, layer gimpbridge.ObjectID) error {
	var unchanged []string

	if content := p.String("text", ""); content != "" {
		if v, err := run1("gimp-text-layer-get-text", gimpbridge.Args{"layer": layer}); err != nil {
			return err
		} else if got, _ := v.(string); got != content {
			unchanged = append(unchanged, "text")
		}
	}

	if p.Has("size") {
		if v, err := run1("gimp-text-layer-get-font-size", gimpbridge.Args{"layer": layer}); err != nil {
			return err
		} else if got, _ := v.(float64); got != p.Float("size", 24) {
			unchanged = append(unchanged, "size")
		}
	}

	if color := p.String("color", ""); color != "" {
		want, err := gimpbridge.NormalizeColor(color)
		if err != nil {
			return err
		}

		if v, err := run1("gimp-text-layer-get-color", gimpbridge.Args{"layer": layer}); err != nil {
			return err
		} else if got, _ := v.(string); got != want {
			unchanged = append(unchanged, "color")
		}
	}

	if len(unchanged) > 0 {
		return fmt.Errorf("GIMP ignored the change to %s: this layer's pixels were "+
			"changed after its text was set, so GIMP no longer re-renders it. "+
			"Delete the layer and add the text again", strings.Join(unchanged, ", "))
	}

	return nil
}

// restyleText applies edit_text's text, justification, size and font changes,
// each only when it was given.
func restyleText(p Params, layer gimpbridge.ObjectID) error {
	if content := p.String("text", ""); content != "" {
		if err := run("gimp-text-layer-set-text",
			gimpbridge.Args{"layer": layer, "text": content}); err != nil {
			return err
		}
	}

	if justify := p.String("justify", ""); justify != "" {
		if err := run("gimp-text-layer-set-justification",
			gimpbridge.Args{"layer": layer, "justify": justify}); err != nil {
			return err
		}
	}

	if p.Has("size") {
		if err := run("gimp-text-layer-set-font-size", gimpbridge.Args{
			"layer": layer, "font-size": p.Float("size", 24), "unit": pixelUnit,
		}); err != nil {
			return err
		}
	}

	if font := p.String("font", ""); font != "" {
		fontID, err := lookupFont(font)
		if err != nil {
			return err
		}

		if err := run("gimp-text-layer-set-font",
			gimpbridge.Args{"layer": layer, "font": fontID}); err != nil {
			return err
		}
	}

	return nil
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
