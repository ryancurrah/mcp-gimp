package commands

import (
	"fmt"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
)

// openImages returns the ids of every image currently open, most recent first,
// which is the ordering the image_index parameter indexes into.
func openImages() ([]gimpbridge.ObjectID, error) {
	out, err := gimpbridge.Run("gimp-get-images", nil)
	if err != nil {
		return nil, err
	}

	if len(out) == 0 {
		return nil, nil
	}

	ids, ok := out[0].([]gimpbridge.ObjectID)
	if !ok {
		return nil, fmt.Errorf("gimp-get-images returned %T", out[0])
	}

	return ids, nil
}

// imageAt resolves an image_index into an image id.
func imageAt(index int) (gimpbridge.ObjectID, error) {
	images, err := openImages()
	if err != nil {
		return 0, err
	}

	if len(images) == 0 {
		return 0, fmt.Errorf("no images are open in GIMP")
	}

	if index < 0 || index >= len(images) {
		return 0, fmt.Errorf("image_index %d is out of range; %d image(s) open", index, len(images))
	}

	return images[index], nil
}

// layersOf lists an image's layers, top to bottom.
func layersOf(image gimpbridge.ObjectID) ([]gimpbridge.ObjectID, error) {
	out, err := gimpbridge.Run("gimp-image-get-layers", gimpbridge.Args{"image": image})
	if err != nil {
		return nil, err
	}

	if len(out) == 0 {
		return nil, nil
	}

	ids, ok := out[0].([]gimpbridge.ObjectID)
	if !ok {
		return nil, fmt.Errorf("gimp-image-get-layers returned %T", out[0])
	}

	return ids, nil
}

// itemName reads an item's name.
func itemName(item gimpbridge.ObjectID) (string, error) {
	v, err := run1("gimp-item-get-name", gimpbridge.Args{"item": item})
	if err != nil {
		return "", err
	}

	name, _ := v.(string)

	return name, nil
}

// resolveDrawable picks the layer a command should act on: the named layer if
// layer_name was given, otherwise the image's selected layer, otherwise the
// top layer.
func resolveDrawable(image gimpbridge.ObjectID, layerName string) (gimpbridge.ObjectID, error) {
	layers, err := layersOf(image)
	if err != nil {
		return 0, err
	}

	if len(layers) == 0 {
		return 0, fmt.Errorf("image has no layers")
	}

	if layerName != "" {
		for _, layer := range layers {
			name, err := itemName(layer)
			if err != nil {
				return 0, err
			}

			if name == layerName {
				return layer, nil
			}
		}

		return 0, fmt.Errorf("no layer named %q", layerName)
	}

	selected, err := gimpbridge.Run("gimp-image-get-selected-layers",
		gimpbridge.Args{"image": image})
	if err == nil && len(selected) > 0 {
		if ids, ok := selected[0].([]gimpbridge.ObjectID); ok && len(ids) > 0 {
			return ids[0], nil
		}
	}

	return layers[0], nil
}

// runNonInteractive is GimpRunMode's NONINTERACTIVE member. Run modes are
// passed to the PDB as plain integers, so the value is named here rather than
// spelled out at each call site.
const runNonInteractive = 1

// target resolves the image and drawable a command operates on from the
// conventional image_index, layer_name and layer_index parameters, and focuses
// the layer it resolved.
func target(p Params) (image, drawable gimpbridge.ObjectID, err error) {
	image, err = imageAt(p.Int("image_index", 0))
	if err != nil {
		return 0, 0, err
	}

	if p.Has("layer_index") {
		drawable, err = layerAt(image, p.Int("layer_index", 0))
	} else {
		drawable, err = resolveDrawable(image, p.String("layer_name", ""))
	}

	if err != nil {
		return 0, 0, err
	}

	if err := focusLayer(image, drawable); err != nil {
		return 0, 0, err
	}

	return image, drawable, nil
}

// focusLayer makes a layer the image's selected layer.
//
// GIMP bounds several editing operations by the selected layers rather than by
// the drawable handed to them: stroking a selection, for one, is clipped to
// the selected layer's extents, so a command that targets one layer while
// another is selected draws in the wrong place. Resolving a target therefore
// also focuses it, which is what clicking the layer in the Layers dialog does
// before an edit.
func focusLayer(image, layer gimpbridge.ObjectID) error {
	return run("gimp-image-set-selected-layers", gimpbridge.Args{
		"image": image, "layers": gimpbridge.Items{layer},
	})
}

// layerAt resolves a layer by its position in the stack, top first.
func layerAt(image gimpbridge.ObjectID, index int) (gimpbridge.ObjectID, error) {
	layers, err := layersOf(image)
	if err != nil {
		return 0, err
	}

	if index < 0 || index >= len(layers) {
		return 0, fmt.Errorf("layer_index %d is out of range; %d layer(s)", index, len(layers))
	}

	return layers[index], nil
}

// flush pushes pending drawing operations to the display.
func flush() error {
	return run("gimp-displays-flush", nil)
}

// imageSize reads an image's pixel dimensions.
func imageSize(image gimpbridge.ObjectID) (width, height int, err error) {
	w, err := run1("gimp-image-get-width", gimpbridge.Args{"image": image})
	if err != nil {
		return 0, 0, err
	}

	h, err := run1("gimp-image-get-height", gimpbridge.Args{"image": image})
	if err != nil {
		return 0, 0, err
	}

	wi, _ := w.(int64)
	hi, _ := h.(int64)

	return int(wi), int(hi), nil
}
