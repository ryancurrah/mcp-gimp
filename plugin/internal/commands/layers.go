package commands

import (
	"fmt"
	"strings"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
)

//nolint:gochecknoinits // the command table is assembled from several files
func init() {
	register("create_layer", createLayer)
	register("duplicate_layer", duplicateLayer)
	register("delete_layer", deleteLayer)
	register("rename_layer", renameLayer)
	register("set_layer_properties", setLayerProperties)
	register("reorder_layer", reorderLayer)
	register("flatten_image", flattenImage)
	register("merge_visible_layers", mergeVisibleLayers)
	register("fill_layer", fillLayer)
	register("fill_selection", fillSelection)
	register("set_layer_offsets", setLayerOffsets)
	register("add_image_layer", addImageLayer)
}

// layerModeNormal is GIMP 3's GIMP_LAYER_MODE_NORMAL. GIMP 2's plain 0 is
// NORMAL_LEGACY in GIMP 3, which composites differently, so every layer this
// plug-in creates uses this value.
const layerModeNormal = 28

// layerModes maps protocol blend mode names onto GimpLayerMode.
var layerModes = map[string]int{
	"normal":        layerModeNormal,
	"dissolve":      1,
	"multiply":      30,
	"screen":        31,
	"overlay":       23,
	"difference":    34,
	"addition":      33,
	"subtract":      35,
	"darken-only":   36,
	"lighten-only":  37,
	"hue":           41,
	"saturation":    42,
	"color":         43,
	"value":         44,
	"divide":        32,
	"dodge":         38,
	"burn":          39,
	"hard-light":    40,
	"soft-light":    45,
	"grain-extract": 46,
	"grain-merge":   47,
}

// layerMode resolves a blend mode name.
//
// The lookup is case-insensitive because the tool schema documents these modes
// in upper case ("NORMAL", "MULTIPLY") and supplies "NORMAL" as the default
// when a caller omits blend_mode, while the table is keyed in lower case.
func layerMode(name string) (int, error) {
	if name == "" {
		return layerModeNormal, nil
	}

	mode, ok := layerModes[strings.ToLower(name)]
	if !ok {
		return 0, fmt.Errorf("unknown blend mode %q", name)
	}

	return mode, nil
}

// createLayer adds a new layer to an image.
func createLayer(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	imgWidth, imgHeight, err := imageSize(image)
	if err != nil {
		return nil, err
	}

	width := p.Int("width", imgWidth)
	height := p.Int("height", imgHeight)

	mode, err := layerMode(p.String("blend_mode", "normal"))
	if err != nil {
		return nil, err
	}

	name := p.String("name", "Layer")

	// RGBA_IMAGE so the layer can hold transparency.
	v, err := run1("gimp-layer-new", gimpbridge.Args{
		"image": image, "name": name, "width": width, "height": height,
		"type": 1, "opacity": p.Float("opacity", 100), "mode": mode,
	})
	if err != nil {
		return nil, err
	}

	layer, err := objectID(v)
	if err != nil {
		return nil, err
	}

	if err := run("gimp-image-insert-layer", gimpbridge.Args{
		"image": image, "layer": layer,
		"parent": gimpbridge.ObjectID(-1), "position": p.Int("position", 0),
	}); err != nil {
		return nil, err
	}

	if fill := p.String("fill", ""); fill != "" {
		if err := fillDrawable(layer, fill, isTransparentFill(fill)); err != nil {
			return nil, err
		}
	} else {
		// A fresh layer is otherwise undefined; clear it to transparent.
		if err := run("gimp-drawable-fill",
			gimpbridge.Args{"drawable": layer, "fill-type": fillTransparent}); err != nil {
			return nil, err
		}
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{
		"layer_id": int(layer), "name": name, "width": width, "height": height,
	}, nil
}

// duplicateLayer copies a layer in place.
func duplicateLayer(p Params) (any, error) {
	image, layer, err := target(p)
	if err != nil {
		return nil, err
	}

	v, err := run1("gimp-layer-copy", gimpbridge.Args{"layer": layer})
	if err != nil {
		return nil, err
	}

	copyID, err := objectID(v)
	if err != nil {
		return nil, err
	}

	if name := p.String("new_name", ""); name != "" {
		if err := run("gimp-item-set-name",
			gimpbridge.Args{"item": copyID, "name": name}); err != nil {
			return nil, err
		}
	}

	if err := run("gimp-image-insert-layer", gimpbridge.Args{
		"image": image, "layer": copyID,
		"parent": gimpbridge.ObjectID(-1), "position": 0,
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	name, _ := itemName(copyID)

	return map[string]any{"layer_id": int(copyID), "name": name}, nil
}

// deleteLayer removes a layer.
func deleteLayer(p Params) (any, error) {
	image, layer, err := target(p)
	if err != nil {
		return nil, err
	}

	if err := run("gimp-image-remove-layer",
		gimpbridge.Args{"image": image, "layer": layer}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "layer_id": int(layer)}, nil
}

// renameLayer changes a layer's name.
func renameLayer(p Params) (any, error) {
	_, layer, err := target(p)
	if err != nil {
		return nil, err
	}

	name := p.String("new_name", "")
	if name == "" {
		return nil, fmt.Errorf("new_name is required")
	}

	// rename_layer identifies the layer with old_name rather than the
	// layer_name the other layer commands use.
	if old := p.String("old_name", ""); old != "" && !p.Has("layer_index") {
		image, err := imageAt(p.Int("image_index", 0))
		if err != nil {
			return nil, err
		}

		if layer, err = resolveDrawable(image, old); err != nil {
			return nil, err
		}
	}

	if err := run("gimp-item-set-name",
		gimpbridge.Args{"item": layer, "name": name}); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "layer_id": int(layer), "name": name}, nil
}

// setLayerProperties updates opacity, visibility, blend mode or lock state.
func setLayerProperties(p Params) (any, error) {
	_, layer, err := target(p)
	if err != nil {
		return nil, err
	}

	if p.Has("opacity") {
		if err := run("gimp-layer-set-opacity",
			gimpbridge.Args{"layer": layer, "opacity": p.Float("opacity", 100)}); err != nil {
			return nil, err
		}
	}

	if p.Has("visible") {
		if err := run("gimp-item-set-visible",
			gimpbridge.Args{"item": layer, "visible": p.Bool("visible", true)}); err != nil {
			return nil, err
		}
	}

	if p.Has("blend_mode") {
		mode, err := layerMode(p.String("blend_mode", "normal"))
		if err != nil {
			return nil, err
		}

		if err := run("gimp-layer-set-mode",
			gimpbridge.Args{"layer": layer, "mode": mode}); err != nil {
			return nil, err
		}
	}

	if p.Has("lock_alpha") {
		if err := run("gimp-layer-set-lock-alpha",
			gimpbridge.Args{"layer": layer, "lock-alpha": p.Bool("lock_alpha", false)}); err != nil {
			return nil, err
		}
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "layer_id": int(layer)}, nil
}

// reorderLayer moves a layer within the stack.
func reorderLayer(p Params) (any, error) {
	image, layer, err := target(p)
	if err != nil {
		return nil, err
	}

	if err := run("gimp-image-reorder-item", gimpbridge.Args{
		"image": image, "item": layer,
		"parent": gimpbridge.ObjectID(-1), "position": p.Int("new_position", p.Int("position", 0)),
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "layer_id": int(layer)}, nil
}

// flattenImage collapses every layer into one.
func flattenImage(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	if err := run("gimp-image-flatten", gimpbridge.Args{"image": image}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// mergeVisibleLayers merges the visible layers into one.
func mergeVisibleLayers(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	// Merge type 0 is EXPAND-AS-NECESSARY.
	if err := run("gimp-image-merge-visible-layers",
		gimpbridge.Args{"image": image, "merge-type": 0}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// fillLayer floods a whole layer with a colour.
func fillLayer(p Params) (any, error) {
	_, layer, err := target(p)
	if err != nil {
		return nil, err
	}

	color := p.String("color", "white")

	if err := fillDrawable(layer, color, isTransparentFill(color)); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "layer_id": int(layer), "color": color}, nil
}

// fillSelection fills the current selection.
//
// With no colour the foreground is used, matching the reference behaviour of
// passing the colour through unset.
func fillSelection(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	if color := p.String("color", ""); color != "" {
		if err := run("gimp-context-set-foreground",
			gimpbridge.Args{"foreground": gimpbridge.Color(color)}); err != nil {
			return nil, err
		}
	}

	fillType, err := fillTypeFor(p.String("fill_type", ""))
	if err != nil {
		return nil, err
	}

	if err := run("gimp-drawable-edit-fill",
		gimpbridge.Args{"drawable": drawable, "fill-type": fillType}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "layer_id": int(drawable)}, nil
}

// setLayerOffsets moves a layer to an exact position on the canvas.
//
// Placing a layer is otherwise only reachable through call_api, and it is
// needed after anything whose size is known only once it is rendered.
func setLayerOffsets(p Params) (any, error) {
	image, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	x, y := p.Int("x", 0), p.Int("y", 0)

	if align := p.String("align", ""); align != "" && align != "left" {
		if err := alignLayer(image, drawable, align, x, y); err != nil {
			return nil, err
		}
	} else if err := run("gimp-layer-set-offsets",
		gimpbridge.Args{"layer": drawable, "offx": x, "offy": y}); err != nil {
		return nil, err
	}

	offX, offY, err := drawableOffsets(drawable)
	if err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{
		"status": "success", "layer_id": int(drawable),
		"position": map[string]int{"x": offX, "y": offY},
	}, nil
}

// addImageLayer places an image file into the open image as a new layer.
//
// Compositing one image into another is otherwise impossible through this
// protocol: every other command works within a single image.
func addImageLayer(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	path := p.String("file_path", "")
	if path == "" {
		return nil, fmt.Errorf("file_path is required")
	}

	v, err := run1("gimp-file-load-layer", gimpbridge.Args{
		"run-mode": runNonInteractive,
		"image":    image,
		"file":     path,
	})
	if err != nil {
		return nil, err
	}

	layer, err := objectID(v)
	if err != nil {
		return nil, err
	}

	if err := run("gimp-image-insert-layer", gimpbridge.Args{
		"image": image, "layer": layer,
		"parent":   gimpbridge.ObjectID(-1),
		"position": p.Int("position", -1),
	}); err != nil {
		return nil, err
	}

	if name := p.String("name", ""); name != "" {
		if err := run("gimp-item-set-name",
			gimpbridge.Args{"item": layer, "name": name}); err != nil {
			return nil, err
		}
	}

	// Scaling happens before placement so an alignment sees the final width.
	if err := scaleLoadedLayer(p, layer); err != nil {
		return nil, err
	}

	if err := alignLayer(image, layer, p.String("align", "left"),
		p.Int("x", 0), p.Int("y", 0)); err != nil {
		return nil, err
	}

	if p.String("align", "left") == "left" {
		if err := run("gimp-layer-set-offsets", gimpbridge.Args{
			"layer": layer, "offx": p.Int("x", 0), "offy": p.Int("y", 0),
		}); err != nil {
			return nil, err
		}
	}

	if opacity := p.Float("opacity", 100); opacity != 100 {
		if err := run("gimp-layer-set-opacity",
			gimpbridge.Args{"layer": layer, "opacity": opacity}); err != nil {
			return nil, err
		}
	}

	width, height, err := drawableSize(layer)
	if err != nil {
		return nil, err
	}

	offX, offY, err := drawableOffsets(layer)
	if err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{
		"status": "success", "layer_id": int(layer),
		"width": width, "height": height,
		"position": map[string]int{"x": offX, "y": offY},
	}, nil
}

// scaleLoadedLayer resizes a freshly loaded layer to the requested size,
// keeping its aspect ratio when only one dimension is given.
func scaleLoadedLayer(p Params, layer gimpbridge.ObjectID) error {
	want, wantHeight := p.Int("width", 0), p.Int("height", 0)
	if want <= 0 && wantHeight <= 0 {
		return nil
	}

	current, currentHeight, err := drawableSize(layer)
	if err != nil {
		return err
	}

	switch {
	case want <= 0:
		want = scaleOther(wantHeight, currentHeight, current)
	case wantHeight <= 0:
		wantHeight = scaleOther(want, current, currentHeight)
	}

	return run("gimp-layer-scale", gimpbridge.Args{
		"layer": layer, "new-width": want, "new-height": wantHeight,
		"local-origin": false,
	})
}
