// Tool definitions for resizing, cropping, rotating and history.
//
// Each tool is its description, its argument struct and, where arguments
// have non-zero defaults, SetDefaults. Argument constraints are struct tags;
// see schema.go for what each means and CONTRIBUTING.md for where they come
// from.

package server

// registerTransformTools wires up the pass-through tools for resizing, cropping, rotating and history.
func registerTransformTools(r *registrar) {
	addObject[ScaleImageInput](r, toolDef{
		Name: "scale_image", Command: "scale_image",
		Required: []string{"width", "height"}, Description: scaleImageDesc,
	})
	addObject[ScaleToFitInput](r, toolDef{
		Name: "scale_to_fit", Command: "scale_to_fit",
		Required: []string{"max_width", "max_height"}, Description: scaleToFitDesc,
	})
	addObject[CropToSelectionInput](r, toolDef{
		Name: "crop_to_selection", Command: "crop_to_selection",
		Required: nil, Description: cropToSelectionDesc,
	})
	addObject[CropToRectInput](r, toolDef{
		Name: "crop_to_rect", Command: "crop_to_rect",
		Required: []string{"x", "y", "width", "height"}, Description: cropToRectDesc,
	})
	addObject[RotateImageInput](r, toolDef{
		Name: "rotate_image", Command: "rotate_image",
		Required: []string{"angle"}, Description: rotateImageDesc,
	})
	addObject[RotateLayerInput](r, toolDef{
		Name: "rotate_layer", Command: "rotate_layer",
		Required: []string{"angle"}, Description: rotateLayerDesc,
	})
	addObject[FlipImageInput](r, toolDef{
		Name: "flip_image", Command: "flip_image",
		Required: nil, Description: flipImageDesc,
	})
	addObject[ResizeCanvasInput](r, toolDef{
		Name: "resize_canvas", Command: "resize_canvas",
		Required: []string{"width", "height"}, Description: resizeCanvasDesc,
	})
	addObject[SetActiveImageInput](r, toolDef{
		Name: "set_active_image", Command: "set_active_image",
		Required: []string{"image_index"}, Description: setActiveImageDesc,
	})
	addObject[ConvertColorModeInput](r, toolDef{
		Name: "convert_color_mode", Command: "convert_color_mode",
		Required: []string{"mode"}, Description: convertColorModeDesc,
	})
	addObject[CloseImageInput](r, toolDef{
		Name: "close_image", Command: "close_image",
		Required: nil, Description: closeImageDesc,
	})
}

// scaleImageDesc documents the scale_image tool.
const scaleImageDesc = `Scale an image to exact pixel dimensions.

Parameters:
- width: Target width in pixels
- height: Target height in pixels
- interpolation: "cubic" (default), "linear", "none"
- image_index: Target image index (default 0)

Returns: {status, width, height}`

// ScaleImageInput holds the arguments for the scale_image tool.
type ScaleImageInput struct {
	Width         int     `json:"width" jsonschema:"Target width in pixels"`
	Height        int     `json:"height" jsonschema:"Target height in pixels"`
	Interpolation *string `json:"interpolation" jsonschema:"\"cubic\" (default), \"linear\", \"none\", \"nohalo\", \"lohalo\"" enum:"cubic,linear,none,nohalo,lohalo" gimp:"gimp-context-set-interpolation.interpolation"`
	ImageIndex    int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the scale_image tool.
func (in *ScaleImageInput) SetDefaults() {
	if in.Interpolation == nil {
		in.Interpolation = ptr("cubic")
	}
}

// scaleToFitDesc documents the scale_to_fit tool.
const scaleToFitDesc = `Scale an image to fit within a bounding box, preserving aspect ratio.

Parameters:
- max_width: Maximum allowed width in pixels
- max_height: Maximum allowed height in pixels
- interpolation: "cubic" (default), "linear", "none"
- image_index: Target image index (default 0)

Returns: {status, width, height} — final dimensions after scaling`

// ScaleToFitInput holds the arguments for the scale_to_fit tool.
type ScaleToFitInput struct {
	MaxWidth      int     `json:"max_width" jsonschema:"Maximum allowed width in pixels"`
	MaxHeight     int     `json:"max_height" jsonschema:"Maximum allowed height in pixels"`
	Interpolation *string `json:"interpolation" jsonschema:"\"cubic\" (default), \"linear\", \"none\", \"nohalo\", \"lohalo\"" enum:"cubic,linear,none,nohalo,lohalo" gimp:"gimp-context-set-interpolation.interpolation"`
	ImageIndex    int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the scale_to_fit tool.
func (in *ScaleToFitInput) SetDefaults() {
	if in.Interpolation == nil {
		in.Interpolation = ptr("cubic")
	}
}

// cropToSelectionDesc documents the crop_to_selection tool.
const cropToSelectionDesc = `Crop the image canvas to the current selection bounds.

Parameters:
- autocrop: If True, auto-detect crop bounds instead of using selection (default False)
- image_index: Target image index (default 0)

Returns: {status, x, y, width, height} — crop region applied`

// CropToSelectionInput holds the arguments for the crop_to_selection tool.
type CropToSelectionInput struct {
	Autocrop   bool `json:"autocrop" jsonschema:"If True, auto-detect crop bounds instead of using selection (default False)"`
	ImageIndex int  `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// cropToRectDesc documents the crop_to_rect tool.
const cropToRectDesc = `Crop the image canvas to an explicit rectangle.

Parameters:
- x, y: Top-left corner of the crop rectangle
- width, height: Dimensions of the crop rectangle
- image_index: Target image index (default 0)

Returns: {status, x, y, width, height}`

// CropToRectInput holds the arguments for the crop_to_rect tool.
type CropToRectInput struct {
	X          int `json:"x" jsonschema:"Top-left corner of the crop rectangle"`
	Y          int `json:"y" jsonschema:"Top-left corner of the crop rectangle"`
	Width      int `json:"width" jsonschema:"Dimensions of the crop rectangle"`
	Height     int `json:"height" jsonschema:"Dimensions of the crop rectangle"`
	ImageIndex int `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// rotateImageDesc documents the rotate_image tool.
const rotateImageDesc = `Rotate the entire image, canvas included, by a right angle.

The rotation is lossless. Any other angle is refused; to tilt one layer, use
rotate_layer.

Parameters:
- angle: Clockwise rotation in degrees: 90, 180 or 270 (-90 is taken as 270)
- image_index: Target image index (default 0)

Returns: {status, degrees, width, height}`

// RotateImageInput holds the arguments for the rotate_image tool.
type RotateImageInput struct {
	Angle      float64 `json:"angle" jsonschema:"Clockwise rotation in degrees: 90, 180 or 270 (-90 is taken as 270); any other angle is refused"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// rotateLayerDesc documents the rotate_layer tool.
const rotateLayerDesc = `Rotate one layer about its own centre by any angle.

The layer grows to hold its rotated corners, so nothing is clipped; the
canvas is unchanged. A rotated text layer can no longer be changed with
edit_text, so set its text, font and colour before rotating it.

Refused while the image has a selection, because GIMP would rotate the
selected pixels as a floating selection instead of the layer. Call
select_none first.

Parameters:
- angle: Clockwise rotation in degrees; negative turns anticlockwise
- interpolation: "cubic" (default), "linear", "none", "nohalo", "lohalo"
- layer_name / layer_index: Identify the layer (defaults to active layer)
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns: {status, layer_id, angle, width, height, position}`

// RotateLayerInput holds the arguments for the rotate_layer tool.
type RotateLayerInput struct {
	Angle         float64 `json:"angle" jsonschema:"Clockwise rotation in degrees; negative turns anticlockwise"`
	Interpolation *string `json:"interpolation" jsonschema:"\"cubic\" (default), \"linear\", \"none\", \"nohalo\", \"lohalo\"" enum:"cubic,linear,none,nohalo,lohalo" gimp:"gimp-context-set-interpolation.interpolation"`
	LayerName     *string `json:"layer_name" jsonschema:"Identify the layer (defaults to active layer)"`
	LayerID       *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	LayerIndex    *int    `json:"layer_index" jsonschema:"Identify the layer by stack position (defaults to active layer)"`
	ImageIndex    int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the rotate_layer tool.
func (in *RotateLayerInput) SetDefaults() {
	if in.Interpolation == nil {
		in.Interpolation = ptr("cubic")
	}
}

// flipImageDesc documents the flip_image tool.
const flipImageDesc = `Flip the entire image horizontally or vertically.

Parameters:
- direction: "horizontal" (default) or "vertical"
- image_index: Target image index (default 0)

Returns status dict.`

// FlipImageInput holds the arguments for the flip_image tool.
type FlipImageInput struct {
	Direction  *string `json:"direction" jsonschema:"\"horizontal\" (default) or \"vertical\"" enum:"horizontal,vertical" gimp:"gimp-image-flip.flip-type"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the flip_image tool.
func (in *FlipImageInput) SetDefaults() {
	if in.Direction == nil {
		in.Direction = ptr("horizontal")
	}
}

// resizeCanvasDesc documents the resize_canvas tool.
const resizeCanvasDesc = `Resize the image canvas without scaling the content.

Parameters:
- width, height: New canvas dimensions in pixels
- anchor: Position of existing content — "center" (default), "top-left", "top",
          "top-right", "left", "right", "bottom-left", "bottom", "bottom-right"
- fill: Color for new canvas areas — CSS color or "transparent"
- image_index: Target image index (default 0)

Returns: {status, width, height, offset_x, offset_y}`

// ResizeCanvasInput holds the arguments for the resize_canvas tool.
type ResizeCanvasInput struct {
	Width      int     `json:"width" jsonschema:"New canvas dimensions in pixels"`
	Height     int     `json:"height" jsonschema:"New canvas dimensions in pixels"`
	Anchor     *string `json:"anchor" jsonschema:"Position of existing content — \"center\" (default), \"top-left\", \"top\", \"top-right\", \"left\", \"right\", \"bottom-left\", \"bottom\", \"bottom-right\"" enum:"center,top-left,top,top-right,left,right,bottom-left,bottom,bottom-right" project:"Where the old canvas sits; the plug-in turns it into offsets."`
	Fill       *string `json:"fill" jsonschema:"Color for new canvas areas — CSS color or \"transparent\""`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the resize_canvas tool.
func (in *ResizeCanvasInput) SetDefaults() {
	if in.Anchor == nil {
		in.Anchor = ptr("center")
	}
	if in.Fill == nil {
		in.Fill = ptr("transparent")
	}
}

// setActiveImageDesc documents the set_active_image tool.
const setActiveImageDesc = `Raise a specific image to the front / make it active in GIMP.

Parameters:
- image_index: Index of the image to activate (from list_images)

Returns status dict.`

// SetActiveImageInput holds the arguments for the set_active_image tool.
type SetActiveImageInput struct {
	ImageIndex int `json:"image_index" jsonschema:"Index of the image to activate (from list_images)"`
}

// convertColorModeDesc documents the convert_color_mode tool.
const convertColorModeDesc = `Convert an image to a different color mode.

Parameters:
- mode: "RGB", "GRAY", or "INDEXED"
- num_colors: Number of colors for INDEXED mode (default 256)
- image_index: Target image index (default 0)

Returns status dict.`

// ConvertColorModeInput holds the arguments for the convert_color_mode tool.
type ConvertColorModeInput struct {
	Mode       string `json:"mode" jsonschema:"\"RGB\", \"GRAY\", or \"INDEXED\"" enum:"RGB,GRAY,INDEXED" project:"Picks gimp-image-convert-rgb, -grayscale or -indexed."`
	NumColors  *int   `json:"num_colors" jsonschema:"Number of colors for INDEXED mode (default 256)"`
	ImageIndex int    `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the convert_color_mode tool.
func (in *ConvertColorModeInput) SetDefaults() {
	if in.NumColors == nil {
		in.NumColors = ptr(256)
	}
}

// closeImageDesc documents the close_image tool.
const closeImageDesc = `Close an image and its window.

An image with unsaved changes is refused unless save_first or force is given.

Only the windows this plug-in opened (through new_canvas or open_image, since
GIMP was last started) can be closed. An image the user opened in GIMP is
refused; close it in GIMP instead.

Parameters:
- image_index: Index of the image to close (default 0)
- save_first: Save over the image's XCF file first (default False); refused if it has none, so use save_xcf for a new image
- force: Close even with unsaved changes, discarding them (default False)

Returns: {status, image_id}`

// CloseImageInput holds the arguments for the close_image tool.
type CloseImageInput struct {
	ImageIndex int  `json:"image_index" jsonschema:"Index of the image to close (default 0)"`
	SaveFirst  bool `json:"save_first" jsonschema:"Save over the image's XCF file first (default False); refused if it has none, so use save_xcf for a new image"`
	Force      bool `json:"force" jsonschema:"Close even with unsaved changes, discarding them (default False)"`
}
