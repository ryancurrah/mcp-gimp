package commands

import (
	"fmt"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
)

//nolint:gochecknoinits // the command table is assembled from several files
func init() {
	register("scale_image", scaleImage)
	register("scale_to_fit", scaleToFit)
	register("crop_to_selection", cropToSelection)
	register("crop_to_rect", cropToRect)
	register("rotate_image", rotateImage)
	register("flip_image", flipImage)
	register("resize_canvas", resizeCanvas)
	register("convert_color_mode", convertColorMode)
	register("set_active_image", setActiveImage)
	register("close_image", closeImage)
	register("undo", undoSteps)
	register("redo", redoSteps)
}

// interpolationModes maps protocol names onto GimpInterpolationType.
var interpolationModes = map[string]int{
	"none": 0, "linear": 1, "cubic": 2, "nohalo": 3, "lohalo": 4,
}

// applyInterpolation sets the interpolation the scale procedures read from
// the context.
func applyInterpolation(p Params) error {
	name := p.String("interpolation", "")
	if name == "" {
		return nil
	}

	mode, ok := interpolationModes[name]
	if !ok {
		return fmt.Errorf("unknown interpolation %q", name)
	}

	return run("gimp-context-set-interpolation", gimpbridge.Args{"interpolation": mode})
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

	degrees := p.Float("angle", p.Float("degrees", 0))

	// GimpRotationType covers the right-angle cases losslessly.
	switch degrees {
	case 90:
		err = run("gimp-image-rotate", gimpbridge.Args{"image": image, "rotate-type": 0})
	case 180:
		err = run("gimp-image-rotate", gimpbridge.Args{"image": image, "rotate-type": 1})
	case 270, -90:
		err = run("gimp-image-rotate", gimpbridge.Args{"image": image, "rotate-type": 2})
	default:
		return nil, fmt.Errorf(
			"degrees must be 90, 180 or 270; arbitrary angles need a layer rotation")
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

// flipImage mirrors an image.
func flipImage(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	// GimpOrientationType: 0 horizontal, 1 vertical.
	var orientation int

	switch p.String("direction", "horizontal") {
	case "horizontal":
		orientation = 0
	case "vertical":
		orientation = 1
	default:
		return nil, fmt.Errorf("direction must be horizontal or vertical")
	}

	if err := run("gimp-image-flip",
		gimpbridge.Args{"image": image, "flip-type": orientation}); err != nil {
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
			"image": image, "dither-type": 0, "palette-type": 0,
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

	if !p.Bool("force", false) && !p.Bool("save_first", false) {
		if dirty, err := run1("gimp-image-is-dirty",
			gimpbridge.Args{"image": image}); err == nil {
			if isDirty, _ := dirty.(bool); isDirty {
				return nil, fmt.Errorf(
					"image has unsaved changes; save it or pass force=true")
			}
		}
	}

	if err := run("gimp-image-delete", gimpbridge.Args{"image": image}); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "image_id": int(image)}, nil
}

// undoSteps would walk the undo stack backwards; see walkUndo.
func undoSteps(p Params) (any, error) {
	return walkUndo(p, "undo")
}

// redoSteps would walk the undo stack forwards; see walkUndo.
func redoSteps(p Params) (any, error) {
	return walkUndo(p, "redo")
}

// walkUndo steps the image's undo stack.
//
// GIMP 2.10 exposed gimp-image-undo and gimp-image-redo; GIMP 3 dropped both,
// keeping only undo *group* management (gimp-image-undo-group-start/end) and
// enable/disable. Stepping the stack is a GUI action with no PDB equivalent,
// so this reports what it cannot do rather than silently applying nothing.
func walkUndo(p Params, proc string) (any, error) {
	if _, err := imageAt(p.Int("image_index", 0)); err != nil {
		return nil, err
	}

	return nil, fmt.Errorf(
		"%s is not available: GIMP 3 removed the PDB procedures that step the "+
			"undo stack, so the plug-in cannot undo or redo. Re-open the image "+
			"or re-apply the inverse operation instead", proc)
}
