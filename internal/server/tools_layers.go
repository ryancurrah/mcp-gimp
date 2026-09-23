// Tool definitions for layers.
//
// Each tool is its description, its argument struct and, where arguments
// have non-zero defaults, SetDefaults. Argument constraints are struct tags;
// see schema.go for what each means and CONTRIBUTING.md for where they come
// from.

package server

// registerLayersTools wires up the pass-through tools for layers.
func registerLayersTools(r *registrar) {
	addObject[CreateLayerInput](r, toolDef{
		Name: "create_layer", Command: "create_layer",
		Required: nil, Description: createLayerDesc,
	})
	addObject[AddImageLayerInput](r, toolDef{
		Name: "add_image_layer", Command: "add_image_layer",
		Required: []string{"file_path"}, Description: addImageLayerDesc,
	})
	addObject[DuplicateLayerInput](r, toolDef{
		Name: "duplicate_layer", Command: "duplicate_layer",
		Required: nil, Description: duplicateLayerDesc,
	})
	addObject[DeleteLayerInput](r, toolDef{
		Name: "delete_layer", Command: "delete_layer",
		Required: nil, Description: deleteLayerDesc,
	})
	addObject[RenameLayerInput](r, toolDef{
		Name: "rename_layer", Command: "rename_layer",
		Required: []string{"new_name"}, Description: renameLayerDesc,
	})
	addObject[SetLayerPropertiesInput](r, toolDef{
		Name: "set_layer_properties", Command: "set_layer_properties",
		Required: nil, Description: setLayerPropertiesDesc,
	})
	addObject[SetLayerOffsetsInput](r, toolDef{
		Name: "set_layer_offsets", Command: "set_layer_offsets",
		Required: nil, Description: setLayerOffsetsDesc,
	})
	addObject[ReorderLayerInput](r, toolDef{
		Name: "reorder_layer", Command: "reorder_layer",
		Required: []string{"new_position"}, Description: reorderLayerDesc,
	})
	addObject[FlattenImageInput](r, toolDef{
		Name: "flatten_image", Command: "flatten_image",
		Required: nil, Description: flattenImageDesc,
	})
	addObject[MergeVisibleLayersInput](r, toolDef{
		Name: "merge_visible_layers", Command: "merge_visible_layers",
		Required: nil, Description: mergeVisibleLayersDesc,
	})
	addObject[FillLayerInput](r, toolDef{
		Name: "fill_layer", Command: "fill_layer",
		Required: []string{"color"}, Description: fillLayerDesc,
	})
	addObject[FillSelectionInput](r, toolDef{
		Name: "fill_selection", Command: "fill_selection",
		Required: nil, Description: fillSelectionDesc,
	})
}

// createLayerDesc documents the create_layer tool.
const createLayerDesc = `Create and insert a new layer into an image.

Parameters:
- name: Layer name (default "New Layer")
- width, height: Layer dimensions; defaults to image dimensions
- fill: Initial fill — "transparent" (default), "white", "black", or any CSS color
- opacity: Layer opacity 0-100 (default 100)
- blend_mode: GIMP layer mode name — "NORMAL" (default), "MULTIPLY", "SCREEN", etc.
- position: Stack position, counted from the top — 0 = top (default), 1 = below the top layer; -1 = directly above the active layer
- image_index: Target image index (default 0)

Returns: {layer_name, layer_id, width, height, position}`

// CreateLayerInput holds the arguments for the create_layer tool.
type CreateLayerInput struct {
	Name       *string  `json:"name" jsonschema:"Layer name (default \"New Layer\")"`
	Width      *int     `json:"width" jsonschema:"Layer dimensions; defaults to image dimensions"`
	Height     *int     `json:"height" jsonschema:"Layer dimensions; defaults to image dimensions"`
	Fill       *string  `json:"fill" jsonschema:"Initial fill — \"transparent\" (default), \"white\", \"black\", or any CSS color"`
	Opacity    *float64 `json:"opacity" jsonschema:"Layer opacity 0-100 (default 100)" minimum:"0" maximum:"100" gimp:"gimp-layer-new.opacity"`
	BlendMode  *string  `json:"blend_mode" jsonschema:"Layer mode, by GIMP's name for it (default \"normal\")" enum:"normal,dissolve,multiply,screen,overlay,difference,addition,subtract,darken-only,lighten-only,hsv-hue,hsv-saturation,hsl-color,hsv-value,lch-hue,lch-chroma,lch-color,lch-lightness,divide,dodge,burn,hardlight,softlight,grain-extract,grain-merge,vivid-light,pin-light,linear-light,hard-mix,exclusion,linear-burn,luma-darken-only,luma-lighten-only,luminance" gimp:"gimp-layer-new.mode"`
	Position   *int     `json:"position" jsonschema:"Stack position counted from the top: 0 = top (default); -1 = directly above the active layer"`
	ImageIndex int      `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the create_layer tool.
func (in *CreateLayerInput) SetDefaults() {
	if in.Name == nil {
		in.Name = ptr("New Layer")
	}
	if in.Fill == nil {
		in.Fill = ptr("transparent")
	}
	if in.Opacity == nil {
		in.Opacity = ptr(100.0)
	}
	if in.BlendMode == nil {
		in.BlendMode = ptr("normal")
	}
	if in.Position == nil {
		in.Position = ptr(0)
	}
}

// addImageLayerDesc documents the add_image_layer tool.
const addImageLayerDesc = `Place an image file into the open image as a new layer.

This is how one image is composited into another: every other command works within a single image, so a logo, photo or texture is brought in here rather than by opening it separately.

Parameters:
- file_path: Absolute path to the image file to place
- x, y: Top-left corner for the layer (default 0, 0)
- align: "left" (default, uses x), "center", or "right" (x is the right margin)
- width, height: Scale the layer to this size; giving one keeps the aspect ratio
- name: Name for the new layer
- opacity: Layer opacity 0-100 (default 100)
- position: Stack position, counted from the top — 0 = top (default), 1 = below the top layer; -1 = directly above the active layer
- image_index: Target image index (default 0)

Returns: {status, layer_id, width, height, position}`

// AddImageLayerInput holds the arguments for the add_image_layer tool.
type AddImageLayerInput struct {
	FilePath   string   `json:"file_path" jsonschema:"Absolute path to the image file to place"`
	X          int      `json:"x" jsonschema:"Top-left corner for the layer (default 0)"`
	Y          int      `json:"y" jsonschema:"Top-left corner for the layer (default 0)"`
	Align      *string  `json:"align" jsonschema:"Horizontal placement: \"left\" (default), \"center\", or \"right\"" enum:"left,center,right" project:"Placement relative to the canvas, which the plug-in computes."`
	Width      *int     `json:"width" jsonschema:"Scale the layer to this width; keeps aspect ratio if height is omitted"`
	Height     *int     `json:"height" jsonschema:"Scale the layer to this height; keeps aspect ratio if width is omitted"`
	Name       *string  `json:"name" jsonschema:"Name for the new layer"`
	Opacity    *float64 `json:"opacity" jsonschema:"Layer opacity 0-100 (default 100)" minimum:"0" maximum:"100" gimp:"gimp-layer-set-opacity.opacity"`
	Position   *int     `json:"position" jsonschema:"Stack position counted from the top: 0 = top (default); -1 = directly above the active layer"`
	ImageIndex int      `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the add_image_layer tool.
func (in *AddImageLayerInput) SetDefaults() {
	if in.Opacity == nil {
		in.Opacity = ptr(100.0)
	}
	if in.Position == nil {
		in.Position = ptr(0)
	}
}

// duplicateLayerDesc documents the duplicate_layer tool.
const duplicateLayerDesc = `Duplicate a layer and insert the copy above it.

Parameters:
- layer_name: Name of the layer to duplicate; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns: {layer_name, layer_id}`

// DuplicateLayerInput holds the arguments for the duplicate_layer tool.
type DuplicateLayerInput struct {
	LayerName  *string `json:"layer_name" jsonschema:"Name of the layer to duplicate; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// deleteLayerDesc documents the delete_layer tool.
const deleteLayerDesc = `Delete (remove) a layer from an image.

This is GIMP's gimp-image-remove-layer; use it rather than calling that
procedure through call_api.

Parameters:
- layer_name: Name of the layer to delete
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- layer_index: Position index of the layer (alternative to layer_name)
- image_index: Target image index (default 0)

Provide one of layer_id, layer_name or layer_index. Defaults to active layer if none is given.

Returns status dict.`

// DeleteLayerInput holds the arguments for the delete_layer tool.
type DeleteLayerInput struct {
	LayerName  *string `json:"layer_name" jsonschema:"Name of the layer to delete"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	LayerIndex *int    `json:"layer_index" jsonschema:"Position index of the layer (alternative to layer_name)"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// renameLayerDesc documents the rename_layer tool.
const renameLayerDesc = `Rename a layer.

Parameters:
- new_name: New name for the layer
- old_name: Current name of the layer to rename
- layer_index: Position index alternative to old_name
- layer_id: The layer_id another tool returned (alternative to old_name)
- image_index: Target image index (default 0)

Returns: {old_name, new_name}`

// RenameLayerInput holds the arguments for the rename_layer tool.
type RenameLayerInput struct {
	NewName    string  `json:"new_name" jsonschema:"New name for the layer"`
	OldName    *string `json:"old_name" jsonschema:"Current name of the layer to rename"`
	LayerIndex *int    `json:"layer_index" jsonschema:"Position index alternative to old_name"`
	LayerID    *int    `json:"layer_id" jsonschema:"The layer_id another tool returned (alternative to old_name); it names the image too, so image_index is not consulted"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// setLayerPropertiesDesc documents the set_layer_properties tool.
const setLayerPropertiesDesc = `Set properties on an existing layer.

Parameters:
- layer_name / layer_index: Identify the layer (defaults to active layer)
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- opacity: New opacity 0-100 (omit to leave unchanged)
- blend_mode: New GIMP layer mode name (omit to leave unchanged)
- visible: True/False visibility (omit to leave unchanged)
- image_index: Target image index (default 0)

Returns status dict.`

// SetLayerPropertiesInput holds the arguments for the set_layer_properties tool.
type SetLayerPropertiesInput struct {
	LayerName  *string  `json:"layer_name" jsonschema:"Identify the layer (defaults to active layer)"`
	LayerID    *int     `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	LayerIndex *int     `json:"layer_index" jsonschema:"Identify the layer (defaults to active layer)"`
	Opacity    *float64 `json:"opacity" jsonschema:"New opacity 0-100 (omit to leave unchanged)" minimum:"0" maximum:"100" gimp:"gimp-layer-set-opacity.opacity"`
	BlendMode  *string  `json:"blend_mode" jsonschema:"New layer mode, by GIMP's name for it (omit to leave unchanged)" enum:"normal,dissolve,multiply,screen,overlay,difference,addition,subtract,darken-only,lighten-only,hsv-hue,hsv-saturation,hsl-color,hsv-value,lch-hue,lch-chroma,lch-color,lch-lightness,divide,dodge,burn,hardlight,softlight,grain-extract,grain-merge,vivid-light,pin-light,linear-light,hard-mix,exclusion,linear-burn,luma-darken-only,luma-lighten-only,luminance" gimp:"gimp-layer-set-mode.mode"`
	Visible    *bool    `json:"visible" jsonschema:"True/False visibility (omit to leave unchanged)"`
	ImageIndex int      `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// setLayerOffsetsDesc documents the set_layer_offsets tool.
const setLayerOffsetsDesc = `Move a layer to an exact position on the canvas.

Parameters:
- x, y: New top-left corner of the layer
- align: "left" (default, uses x), "center", or "right" (x is the right margin)
- layer_name / layer_index: Identify the layer (defaults to active layer)
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns: {status, layer_id, position}`

// SetLayerOffsetsInput holds the arguments for the set_layer_offsets tool.
type SetLayerOffsetsInput struct {
	X          int     `json:"x" jsonschema:"New top-left corner of the layer"`
	Y          int     `json:"y" jsonschema:"New top-left corner of the layer"`
	Align      *string `json:"align" jsonschema:"Horizontal placement: \"left\" (default), \"center\", or \"right\"" enum:"left,center,right" project:"Placement relative to the canvas, which the plug-in computes."`
	LayerName  *string `json:"layer_name" jsonschema:"Identify the layer (defaults to active layer)"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	LayerIndex *int    `json:"layer_index" jsonschema:"Identify the layer by stack position (defaults to active layer)"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// reorderLayerDesc documents the reorder_layer tool.
const reorderLayerDesc = `Move a layer to a new stack position.

Parameters:
- new_position: Target stack position, counted from the top (0 = top)
- layer_name / layer_index: Identify the layer (defaults to active layer)
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict.`

// ReorderLayerInput holds the arguments for the reorder_layer tool.
type ReorderLayerInput struct {
	NewPosition int     `json:"new_position" jsonschema:"Target stack position, counted from the top (0 = top)"`
	LayerName   *string `json:"layer_name" jsonschema:"Identify the layer (defaults to active layer)"`
	LayerID     *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	LayerIndex  *int    `json:"layer_index" jsonschema:"Identify the layer (defaults to active layer)"`
	ImageIndex  int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// flattenImageDesc documents the flatten_image tool.
const flattenImageDesc = `Flatten all layers into a single background layer.

Parameters:
- image_index: Target image index (default 0)

Returns status dict.`

// FlattenImageInput holds the arguments for the flatten_image tool.
type FlattenImageInput struct {
	ImageIndex int `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// mergeVisibleLayersDesc documents the merge_visible_layers tool.
const mergeVisibleLayersDesc = `Merge all visible layers into a single layer.

Parameters:
- image_index: Target image index (default 0)

Returns: {layer_name, layer_id}`

// MergeVisibleLayersInput holds the arguments for the merge_visible_layers tool.
type MergeVisibleLayersInput struct {
	ImageIndex int `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// fillLayerDesc documents the fill_layer tool.
const fillLayerDesc = `Fill an entire layer with a solid color.

Parameters:
- color: Fill color as CSS name, hex, or rgb() string
- layer_name: Layer to fill; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict.`

// FillLayerInput holds the arguments for the fill_layer tool.
type FillLayerInput struct {
	Color      string  `json:"color" jsonschema:"Fill color as CSS name, hex, or rgb() string"`
	LayerName  *string `json:"layer_name" jsonschema:"Layer to fill; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// fillSelectionDesc documents the fill_selection tool.
const fillSelectionDesc = `Fill the current selection with a color or fill type.

Parameters:
- color: Fill color as CSS name, hex, or rgb() string (used when fill_type is omitted)
- fill_type: Fill type override: "foreground", "background", or "transparent"
- image_index: Target image index (default 0)
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only

Returns status dict.`

// FillSelectionInput holds the arguments for the fill_selection tool.
type FillSelectionInput struct {
	Color      *string `json:"color" jsonschema:"Fill color as CSS name, hex, or rgb() string (used when fill_type is omitted)"`
	FillType   *string `json:"fill_type" jsonschema:"Fill with \"foreground\" (default), \"background\", \"white\", \"transparent\", \"pattern\" or \"cielab-middle-gray\"" enum:"foreground,background,white,transparent,pattern,cielab-middle-gray" gimp:"gimp-drawable-edit-fill.fill-type"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
	LayerName  *string `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
}
