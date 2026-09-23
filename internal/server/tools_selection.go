// Tool definitions for selections.
//
// Each tool is its description, its argument struct and, where arguments
// have non-zero defaults, SetDefaults. Argument constraints are struct tags;
// see schema.go for what each means and CONTRIBUTING.md for where they come
// from.

package server

// registerSelectionTools wires up the pass-through tools for selections.
func registerSelectionTools(r *registrar) {
	addObject[SelectRectangleInput](r, toolDef{
		Name: "select_rectangle", Command: "select_rectangle",
		Required: []string{"x", "y", "width", "height"}, Description: selectRectangleDesc,
	})
	addObject[SelectRoundedRectangleInput](r, toolDef{
		Name: "select_rounded_rectangle", Command: "select_rounded_rectangle",
		Required: []string{"x", "y", "width", "height"}, Description: selectRoundedRectangleDesc,
	})
	addObject[SelectEllipseInput](r, toolDef{
		Name: "select_ellipse", Command: "select_ellipse",
		Required: []string{"x", "y", "width", "height"}, Description: selectEllipseDesc,
	})
	addObject[SelectByColorInput](r, toolDef{
		Name: "select_by_color", Command: "select_by_color",
		Required: []string{"color"}, Description: selectByColorDesc,
	})
	addObject[SelectAllInput](r, toolDef{
		Name: "select_all", Command: "select_all",
		Required: nil, Description: selectAllDesc,
	})
	addObject[SelectNoneInput](r, toolDef{
		Name: "select_none", Command: "select_none",
		Required: nil, Description: selectNoneDesc,
	})
	addObject[InvertSelectionInput](r, toolDef{
		Name: "invert_selection", Command: "invert_selection",
		Required: nil, Description: invertSelectionDesc,
	})
	addObject[ModifySelectionInput](r, toolDef{
		Name: "modify_selection", Command: "modify_selection",
		Required: []string{"operation", "amount"}, Description: modifySelectionDesc,
	})
	addObject[GetSelectionBoundsInput](r, toolDef{
		Name: "get_selection_bounds", Command: "get_selection_bounds",
		Required: nil, Description: getSelectionBoundsDesc,
	})
}

// selectRectangleDesc documents the select_rectangle tool.
const selectRectangleDesc = `Create a rectangular selection.

Parameters:
- x, y: Top-left corner of the selection
- width, height: Dimensions of the selection
- operation: "replace" (default), "add", "subtract", "intersect"
- feather: Feather radius in pixels (default 0 = no feather)
- image_index: Target image index (default 0)

Returns status dict.`

// SelectRectangleInput holds the arguments for the select_rectangle tool.
type SelectRectangleInput struct {
	X          int     `json:"x" jsonschema:"Top-left corner of the selection"`
	Y          int     `json:"y" jsonschema:"Top-left corner of the selection"`
	Width      int     `json:"width" jsonschema:"Dimensions of the selection"`
	Height     int     `json:"height" jsonschema:"Dimensions of the selection"`
	Operation  *string `json:"operation" jsonschema:"\"replace\" (default), \"add\", \"subtract\", \"intersect\"" enum:"replace,add,subtract,intersect" gimp:"gimp-image-select-rectangle.operation"`
	Feather    float64 `json:"feather" jsonschema:"Feather radius in pixels (default 0 = no feather)"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the select_rectangle tool.
func (in *SelectRectangleInput) SetDefaults() {
	if in.Operation == nil {
		in.Operation = ptr("replace")
	}
}

// selectRoundedRectangleDesc documents the select_rounded_rectangle tool.
const selectRoundedRectangleDesc = `Create a rectangular selection with rounded corners.

Parameters:
- x, y: Top-left corner of the selection
- width, height: Dimensions of the selection
- radius: Corner radius in pixels
- radius_x, radius_y: Per-axis radii, for elliptical corners
- operation: "replace" (default), "add", "subtract", "intersect"
- feather: Feather radius in pixels (default 0)
- image_index: Target image index (default 0)

Returns status dict.`

// SelectRoundedRectangleInput holds the arguments for the select_rounded_rectangle tool.
type SelectRoundedRectangleInput struct {
	X          int     `json:"x" jsonschema:"Top-left corner of the selection"`
	Y          int     `json:"y" jsonschema:"Top-left corner of the selection"`
	Width      int     `json:"width" jsonschema:"Dimensions of the selection"`
	Height     int     `json:"height" jsonschema:"Dimensions of the selection"`
	Radius     float64 `json:"radius" jsonschema:"Corner radius in pixels; sets both axes unless radius_x/radius_y are given"`
	RadiusX    float64 `json:"radius_x" jsonschema:"Horizontal corner radius (defaults to radius)"`
	RadiusY    float64 `json:"radius_y" jsonschema:"Vertical corner radius (defaults to radius)"`
	Operation  *string `json:"operation" jsonschema:"\"replace\" (default), \"add\", \"subtract\", \"intersect\"" enum:"replace,add,subtract,intersect" gimp:"gimp-image-select-round-rectangle.operation"`
	Feather    float64 `json:"feather" jsonschema:"Feather radius in pixels (default 0 = no feather)"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the select_rounded_rectangle tool.
func (in *SelectRoundedRectangleInput) SetDefaults() {
	if in.Operation == nil {
		in.Operation = ptr("replace")
	}
}

// selectEllipseDesc documents the select_ellipse tool.
const selectEllipseDesc = `Create an elliptical selection.

Parameters:
- x, y: Top-left corner of the bounding box
- width, height: Bounding box dimensions
- operation: "replace" (default), "add", "subtract", "intersect"
- feather: Feather radius in pixels (default 0)
- image_index: Target image index (default 0)

Returns status dict.`

// SelectEllipseInput holds the arguments for the select_ellipse tool.
type SelectEllipseInput struct {
	X          int     `json:"x" jsonschema:"Top-left corner of the bounding box"`
	Y          int     `json:"y" jsonschema:"Top-left corner of the bounding box"`
	Width      int     `json:"width" jsonschema:"Bounding box dimensions"`
	Height     int     `json:"height" jsonschema:"Bounding box dimensions"`
	Operation  *string `json:"operation" jsonschema:"\"replace\" (default), \"add\", \"subtract\", \"intersect\"" enum:"replace,add,subtract,intersect" gimp:"gimp-image-select-ellipse.operation"`
	Feather    float64 `json:"feather" jsonschema:"Feather radius in pixels (default 0)"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the select_ellipse tool.
func (in *SelectEllipseInput) SetDefaults() {
	if in.Operation == nil {
		in.Operation = ptr("replace")
	}
}

// selectByColorDesc documents the select_by_color tool.
const selectByColorDesc = `Select regions by color similarity.

Parameters:
- color: Target color as CSS name, hex (#rrggbb), or rgb() string
- threshold: Color similarity tolerance 0-255 (default 15)
- operation: "replace" (default), "add", "subtract", "intersect"
- image_index: Target image index (default 0)
- layer_name: Layer to sample from; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only

Returns status dict.`

// SelectByColorInput holds the arguments for the select_by_color tool.
type SelectByColorInput struct {
	Color      string  `json:"color" jsonschema:"Target color as CSS name, hex (#rrggbb), or rgb() string"`
	Threshold  *int    `json:"threshold" jsonschema:"Color similarity tolerance 0-255 (default 15)" minimum:"0" maximum:"255" gimp:"gimp-context-set-sample-threshold-int.sample-threshold"`
	Operation  *string `json:"operation" jsonschema:"\"replace\" (default), \"add\", \"subtract\", \"intersect\"" enum:"replace,add,subtract,intersect" gimp:"gimp-image-select-color.operation"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
	LayerName  *string `json:"layer_name" jsonschema:"Layer to sample from; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
}

// SetDefaults applies the defaults documented for the select_by_color tool.
func (in *SelectByColorInput) SetDefaults() {
	if in.Threshold == nil {
		in.Threshold = ptr(15)
	}
	if in.Operation == nil {
		in.Operation = ptr("replace")
	}
}

// selectAllDesc documents the select_all tool.
const selectAllDesc = `Select the entire image canvas.

Parameters:
- image_index: Target image index (default 0)

Returns status dict.`

// SelectAllInput holds the arguments for the select_all tool.
type SelectAllInput struct {
	ImageIndex int `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// selectNoneDesc documents the select_none tool.
const selectNoneDesc = `Remove / deselect all selections.

Parameters:
- image_index: Target image index (default 0)

Returns status dict.`

// SelectNoneInput holds the arguments for the select_none tool.
type SelectNoneInput struct {
	ImageIndex int `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// invertSelectionDesc documents the invert_selection tool.
const invertSelectionDesc = `Invert the current selection (select what is not selected).

Parameters:
- image_index: Target image index (default 0)

Returns status dict.`

// InvertSelectionInput holds the arguments for the invert_selection tool.
type InvertSelectionInput struct {
	ImageIndex int `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// modifySelectionDesc documents the modify_selection tool.
const modifySelectionDesc = `Grow, shrink, feather, border, or sharpen the current selection.

Parameters:
- operation: "grow", "shrink", "feather", "border", "sharpen"
- amount: Pixel radius for grow/shrink/feather/border; ignored for sharpen
- image_index: Target image index (default 0)

Returns status dict.`

// ModifySelectionInput holds the arguments for the modify_selection tool.
type ModifySelectionInput struct {
	Operation  string  `json:"operation" jsonschema:"\"grow\", \"shrink\", \"feather\", \"border\", \"sharpen\"" enum:"grow,shrink,feather,border,sharpen" project:"Picks which gimp-selection-* procedure runs."`
	Amount     float64 `json:"amount" jsonschema:"Pixel radius for grow/shrink/feather/border; ignored for sharpen"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// getSelectionBoundsDesc documents the get_selection_bounds tool.
const getSelectionBoundsDesc = `Get the bounding rectangle of the current selection.

Parameters:
- image_index: Target image index (default 0)

Returns: {has_selection, x, y, width, height}`

// GetSelectionBoundsInput holds the arguments for the get_selection_bounds tool.
type GetSelectionBoundsInput struct {
	ImageIndex int `json:"image_index" jsonschema:"Target image index (default 0)"`
}
