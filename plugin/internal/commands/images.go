package commands

import (
	"fmt"
	"math"
	"sync"

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

// runNonInteractive is GimpRunMode's NONINTERACTIVE member, by the name GIMP
// gives it.
const runNonInteractive = "noninteractive"

// target resolves the image and drawable a command operates on from the
// conventional image_index, layer_id, layer_index and layer_name parameters,
// and focuses the layer it resolved.
func target(p Params) (image, drawable gimpbridge.ObjectID, err error) {
	return targetBy(p, "layer_name")
}

// targetBy is target for a command that takes the layer's name under another
// key, as rename_layer does with old_name.
//
// At most one way of naming the layer may be given. A layer_id names its image
// as well, since ids are unique across images, so image_index is not consulted
// for it.
func targetBy(p Params, nameKey string) (image, drawable gimpbridge.ObjectID, err error) {
	given := 0

	for _, key := range []string{"layer_id", "layer_index", nameKey} {
		if p.Has(key) {
			given++
		}
	}

	if given > 1 {
		return 0, 0, fmt.Errorf("give only one of layer_id, layer_index and %s", nameKey)
	}

	if p.Has("layer_id") {
		image, drawable, err = layerByID(p.Int("layer_id", 0))
	} else {
		image, err = imageAt(p.Int("image_index", 0))
		if err != nil {
			return 0, 0, err
		}

		if p.Has("layer_index") {
			drawable, err = layerAt(image, p.Int("layer_index", 0))
		} else {
			drawable, err = resolveDrawable(image, p.String(nameKey, ""))
		}
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

// layerByID resolves a layer by the id the other commands report, along with
// the image it belongs to.
func layerByID(id int) (image, layer gimpbridge.ObjectID, err error) {
	if id <= 0 || id > math.MaxInt32 {
		return 0, 0, fmt.Errorf("layer_id %d is not a layer id", id)
	}

	v, err := run1("gimp-item-id-is-layer", gimpbridge.Args{"item-id": int64(id)})
	if err != nil {
		return 0, 0, err
	}

	if isLayer, _ := v.(bool); !isLayer {
		return 0, 0, fmt.Errorf("layer_id %d is not a layer in any open image", id)
	}

	layer = gimpbridge.ObjectID(id)

	v, err = run1("gimp-item-get-image", gimpbridge.Args{"item": layer})
	if err != nil {
		return 0, 0, err
	}

	image, err = objectID(v)
	if err != nil {
		return 0, 0, err
	}

	return image, layer, nil
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

// displays records the displays this plug-in opened, by image. GIMP 3's PDB
// cannot list an image's displays, and an image with a display cannot be
// deleted, so close_image can only close the windows it knows about.
var displays = struct { //nolint:gochecknoglobals // plug-in lifetime state
	sync.Mutex

	byImage map[gimpbridge.ObjectID][]gimpbridge.ObjectID
}{byImage: map[gimpbridge.ObjectID][]gimpbridge.ObjectID{}}

// openDisplay opens a window on image and remembers it for close_image.
func openDisplay(image gimpbridge.ObjectID) error {
	v, err := run1("gimp-display-new", gimpbridge.Args{"image": image})
	if err != nil {
		return err
	}

	display, err := objectID(v)
	if err != nil {
		return err
	}

	displays.Lock()
	defer displays.Unlock()

	displays.byImage[image] = append(displays.byImage[image], display)

	return nil
}

// closeDisplays closes the windows this plug-in opened on image, skipping any
// the user has already closed. Closing an image's last display deletes the
// image, whether or not it has unsaved changes.
func closeDisplays(image gimpbridge.ObjectID) error {
	displays.Lock()
	defer displays.Unlock()

	for _, display := range displays.byImage[image] {
		v, err := run1("gimp-display-id-is-valid", gimpbridge.Args{"display-id": int(display)})
		if err != nil {
			return err
		}

		if valid, _ := v.(bool); !valid {
			continue
		}

		if err := run("gimp-display-delete", gimpbridge.Args{"display": display}); err != nil {
			return err
		}
	}

	delete(displays.byImage, image)

	return nil
}
