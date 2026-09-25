package commands

import (
	"fmt"
	"math"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
)

//nolint:gochecknoinits // the command table is assembled from several files
func init() {
	register("scale_image", scaleImage)
	register("scale_to_fit", scaleToFit)
	register("crop_to_selection", cropToSelection)
	register("crop_to_rect", cropToRect)
	register("rotate_image", rotateImage)
	register("rotate_layer", rotateLayer)
	register("flip_image", flipImage)
	register("resize_canvas", resizeCanvas)
	register("convert_color_mode", convertColorMode)
	register("set_active_image", setActiveImage)
	register("close_image", closeImage)
}

// applyInterpolation sets the interpolation the scale procedures read from
// the context.
func applyInterpolation(p Params) error {
	name := p.String("interpolation", "")
	if name == "" {
		return nil
	}

	return run("gimp-context-set-interpolation", gimpbridge.Args{"interpolation": name})
}

// scaleImage resizes an image, optionally preserving aspect ratio.
func scaleImage(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	if err := applyInterpolation(p); err != nil {
		return nil, err
	}

	width, height, err := imageSize(image)
	if err != nil {
		return nil, err
	}

	newWidth := p.Int("width", 0)
	newHeight := p.Int("height", 0)

	if newWidth <= 0 && newHeight <= 0 {
		return nil, fmt.Errorf("width or height is required")
	}

	if p.Bool("preserve_aspect", true) {
		switch {
		case newWidth > 0 && newHeight <= 0:
			newHeight = scaleOther(newWidth, width, height)
		case newHeight > 0 && newWidth <= 0:
			newWidth = scaleOther(newHeight, height, width)
		}
	}

	if newWidth <= 0 {
		newWidth = width
	}

	if newHeight <= 0 {
		newHeight = height
	}

	if err := run("gimp-image-scale", gimpbridge.Args{
		"image": image, "new-width": newWidth, "new-height": newHeight,
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "width": newWidth, "height": newHeight}, nil
}

// anchorOffset works out where the old canvas sits inside a resized one.
func anchorOffset(anchor string, oldW, oldH, newW, newH int) (x, y int) {
	dx, dy := newW-oldW, newH-oldH

	switch anchor {
	case "top-left":
		return 0, 0
	case "top", "top-center":
		return dx / 2, 0
	case "top-right":
		return dx, 0
	case "left":
		return 0, dy / 2
	case "right":
		return dx, dy / 2
	case "bottom-left":
		return 0, dy
	case "bottom", "bottom-center":
		return dx / 2, dy
	case "bottom-right":
		return dx, dy
	default: // center
		return dx / 2, dy / 2
	}
}

// scaleOther works out the matching dimension for a proportional resize.
func scaleOther(known, knownOriginal, other int) int {
	if knownOriginal == 0 {
		return other
	}

	return max(other*known/knownOriginal, 1)
}

// scaleToFit shrinks an image to fit inside a bounding box.
func scaleToFit(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	if err := applyInterpolation(p); err != nil {
		return nil, err
	}

	if err := scaleWithin(image, p.Int("max_width", 0), p.Int("max_height", 0)); err != nil {
		return nil, err
	}

	width, height, err := imageSize(image)
	if err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "width": width, "height": height}, nil
}

// cropToSelection crops to the current selection bounds.
func cropToSelection(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	// autocrop trims uniform borders instead of using the selection.
	if p.Bool("autocrop", false) {
		if err := run("gimp-image-autocrop",
			gimpbridge.Args{"image": image, "drawable": gimpbridge.ObjectID(-1)}); err != nil {
			return nil, err
		}

		width, height, err := imageSize(image)
		if err != nil {
			return nil, err
		}

		if err := flush(); err != nil {
			return nil, err
		}

		return map[string]any{"status": "success", "width": width, "height": height}, nil
	}

	out, err := gimpbridge.Run("gimp-selection-bounds", gimpbridge.Args{"image": image})
	if err != nil {
		return nil, err
	}

	if len(out) < 5 {
		return nil, fmt.Errorf("cannot read the selection bounds")
	}

	if nonEmpty, _ := out[0].(bool); !nonEmpty {
		return nil, fmt.Errorf("there is no selection to crop to")
	}

	x1, y1 := intValue(out[1]), intValue(out[2])
	x2, y2 := intValue(out[3]), intValue(out[4])

	if err := run("gimp-image-crop", gimpbridge.Args{
		"image": image, "new-width": x2 - x1, "new-height": y2 - y1,
		"offx": x1, "offy": y1,
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{
		"status": "success", "width": x2 - x1, "height": y2 - y1,
	}, nil
}

// cropToRect crops to an explicit rectangle.
func cropToRect(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	width := p.Int("width", 0)
	height := p.Int("height", 0)

	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("width and height must be positive")
	}

	if err := run("gimp-image-crop", gimpbridge.Args{
		"image": image, "new-width": width, "new-height": height,
		"offx": p.Int("x", 0), "offy": p.Int("y", 0),
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "width": width, "height": height}, nil
}

// rotateImage rotates by a multiple of 90 degrees, or by an arbitrary angle.
func rotateImage(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	degrees := p.Float("angle", 0)

	// GimpRotationType covers the right-angle cases losslessly.
	switch degrees {
	case 90:
		err = run("gimp-image-rotate", gimpbridge.Args{"image": image, "rotate-type": "degrees90"})
	case 180:
		err = run("gimp-image-rotate", gimpbridge.Args{"image": image, "rotate-type": "degrees180"})
	case 270, -90:
		err = run("gimp-image-rotate", gimpbridge.Args{"image": image, "rotate-type": "degrees270"})
	default:
		return nil, fmt.Errorf(
			"angle must be 90, 180 or 270; to rotate by %g degrees, use rotate_layer", degrees)
	}

	if err != nil {
		return nil, err
	}

	width, height, err := imageSize(image)
	if err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{
		"status": "success", "degrees": degrees, "width": width, "height": height,
	}, nil
}

// rotateLayer turns one layer about its own centre by any angle.
//
// gimp-item-transform-rotate acts on the selection when there is one: it
// floats the rotated pixels rather than turning the layer, and its auto-center
// pivots on the selection. So a selection is refused, and the pivot is the
// layer's centre, given explicitly.
func rotateLayer(p Params) (any, error) {
	image, layer, err := target(p)
	if err != nil {
		return nil, err
	}

	bounds, err := gimpbridge.Run("gimp-selection-bounds", gimpbridge.Args{"image": image})
	if err != nil {
		return nil, err
	}

	if len(bounds) > 0 {
		if nonEmpty, _ := bounds[0].(bool); nonEmpty {
			return nil, fmt.Errorf("the image has a selection, which would be rotated " +
				"instead of the layer; clear it with select_none first")
		}
	}

	if err := applyInterpolation(p); err != nil {
		return nil, err
	}

	// adjust grows the layer to hold the rotated corners rather than clipping
	// them to the old bounds.
	if err := run("gimp-context-set-transform-resize",
		gimpbridge.Args{"transform-resize": "adjust"}); err != nil {
		return nil, err
	}

	width, height, err := drawableSize(layer)
	if err != nil {
		return nil, err
	}

	offX, offY, err := drawableOffsets(layer)
	if err != nil {
		return nil, err
	}

	degrees := p.Float("angle", 0)

	v, err := run1("gimp-item-transform-rotate", gimpbridge.Args{
		"item":        layer,
		"angle":       degrees * math.Pi / 180,
		"auto-center": false,
		"center-x":    float64(offX) + float64(width)/2,
		"center-y":    float64(offY) + float64(height)/2,
	})
	if err != nil {
		return nil, err
	}

	rotated, err := objectID(v)
	if err != nil {
		return nil, err
	}

	width, height, err = drawableSize(rotated)
	if err != nil {
		return nil, err
	}

	offX, offY, err = drawableOffsets(rotated)
	if err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{
		"status": "success", "layer_id": int(rotated), "angle": degrees,
		"width": width, "height": height,
		"position": map[string]int{"x": offX, "y": offY},
	}, nil
}

// flipImage mirrors an image.
func flipImage(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	if err := run("gimp-image-flip", gimpbridge.Args{
		"image": image, "flip-type": p.String("direction", "horizontal"),
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// resizeCanvas changes the canvas without scaling its contents.
func resizeCanvas(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	width := p.Int("width", 0)
	height := p.Int("height", 0)

	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("width and height must be positive")
	}

	srcWidth, srcHeight, err := imageSize(image)
	if err != nil {
		return nil, err
	}

	offsetX, offsetY := anchorOffset(p.String("anchor", "center"),
		srcWidth, srcHeight, width, height)

	if p.Has("offset_x") {
		offsetX = p.Int("offset_x", 0)
	}

	if p.Has("offset_y") {
		offsetY = p.Int("offset_y", 0)
	}

	if err := run("gimp-image-resize", gimpbridge.Args{
		"image": image, "new-width": width, "new-height": height,
		"offx": offsetX, "offy": offsetY,
	}); err != nil {
		return nil, err
	}

	// A larger canvas exposes empty space; fill it when asked.
	//
	// Flattening composites the layers onto the background colour, which fills
	// the newly exposed border and leaves the existing pixels alone. Filling
	// the flattened drawable instead would paint over the whole canvas.
	if fill := p.String("fill", ""); fill != "" && fill != "transparent" {
		if err := run("gimp-context-set-background",
			gimpbridge.Args{"background": gimpbridge.Color(fill)}); err != nil {
			return nil, err
		}

		if err := run("gimp-image-flatten", gimpbridge.Args{"image": image}); err != nil {
			return nil, err
		}
	}

	if p.Bool("resize_layers", false) {
		if err := run("gimp-image-resize-to-layers",
			gimpbridge.Args{"image": image}); err != nil {
			return nil, err
		}
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "width": width, "height": height}, nil
}

// convertColorMode switches an image between RGB, grayscale and indexed.
func convertColorMode(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	mode := p.String("mode", "")

	switch mode {
	case "RGB":
		err = run("gimp-image-convert-rgb", gimpbridge.Args{"image": image})
	case "GRAY", "GRAYSCALE":
		err = run("gimp-image-convert-grayscale", gimpbridge.Args{"image": image})
	case "INDEXED":
		err = run("gimp-image-convert-indexed", gimpbridge.Args{
			"image": image, "dither-type": "none", "palette-type": "generate",
			"num-cols":     p.Int("num_colors", 256),
			"alpha-dither": false, "remove-unused": true, "palette": "",
		})
	default:
		return nil, fmt.Errorf("mode must be RGB, GRAY or INDEXED")
	}

	if err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "mode": mode}, nil
}

// setActiveImage brings an image's display to the front.
func setActiveImage(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	// There is no "active image" in libgimp; commands address images by
	// index, so this reports the resolved target rather than mutating state.
	return map[string]any{
		"status": "success", "image_id": int(image),
		"image_index": p.Int("image_index", 0),
	}, nil
}

// closeImage discards an image.
func closeImage(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	if p.Bool("save_first", false) {
		if err := saveToOwnXCF(image); err != nil {
			return nil, err
		}
	} else if !p.Bool("force", false) {
		dirty, err := run1("gimp-image-is-dirty", gimpbridge.Args{"image": image})
		if err != nil {
			return nil, err
		}

		if isDirty, _ := dirty.(bool); isDirty {
			return nil, fmt.Errorf(
				"image has unsaved changes; pass save_first=true to save it, or force=true to discard them")
		}
	}

	if err := closeDisplays(image); err != nil {
		return nil, err
	}

	// Closing the last display has already deleted the image. One that is
	// still here has no display, or one this plug-in did not open.
	v, err := run1("gimp-image-id-is-valid", gimpbridge.Args{"image-id": int(image)})
	if err != nil {
		return nil, err
	}

	if valid, _ := v.(bool); valid {
		if err := run("gimp-image-delete", gimpbridge.Args{"image": image}); err != nil {
			return nil, fmt.Errorf("the image is open in a window this plug-in did not open, "+
				"which only GIMP can close; close it there: %w", err)
		}
	}

	return map[string]any{"status": "success", "image_id": int(image)}, nil
}

// saveToOwnXCF saves an image over the XCF file it was loaded from or last
// saved to.
func saveToOwnXCF(image gimpbridge.ObjectID) error {
	v, err := run1("gimp-image-get-xcf-file", gimpbridge.Args{"image": image})
	if err != nil {
		return err
	}

	path, _ := v.(string)
	if path == "" {
		return fmt.Errorf("the image has no XCF file to save to; save it with save_xcf first")
	}

	return saveAsXCF(image, path)
}
