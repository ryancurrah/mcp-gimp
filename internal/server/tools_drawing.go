// Tool definitions for painting, stroking and filling shapes.
//
// Each tool is its description, its argument struct and, where arguments
// have non-zero defaults, SetDefaults. Argument constraints are struct tags;
// see schema.go for what each means and CONTRIBUTING.md for where they come
// from.

package server

// registerDrawingTools wires up the pass-through tools for painting, stroking and filling shapes.
func registerDrawingTools(r *registrar) {
	addObject[GetContextStateInput](r, toolDef{
		Name: "get_context_state", Command: "get_context_state",
		Required: nil, Description: getContextStateDesc,
	})
	addObject[SetColorsInput](r, toolDef{
		Name: "set_colors", Command: "set_colors",
		Required: nil, Description: setColorsDesc,
	})
	addObject[DrawLineInput](r, toolDef{
		Name: "draw_line", Command: "draw_line",
		Required: []string{"x1", "y1", "x2", "y2"}, Description: drawLineDesc,
	})
	addObject[DrawRectangleInput](r, toolDef{
		Name: "draw_rectangle", Command: "draw_rectangle",
		Required: []string{"x", "y", "width", "height"}, Description: drawRectangleDesc,
	})
	addObject[DrawRoundedRectangleInput](r, toolDef{
		Name: "draw_rounded_rectangle", Command: "draw_rounded_rectangle",
		Required: []string{"x", "y", "width", "height"}, Description: drawRoundedRectangleDesc,
	})
	addObject[DrawEllipseInput](r, toolDef{
		Name: "draw_ellipse", Command: "draw_ellipse",
		Required: []string{"x", "y", "width", "height"}, Description: drawEllipseDesc,
	})
	addObject[FillRectangleInput](r, toolDef{
		Name: "fill_rectangle", Command: "fill_rectangle",
		Required: []string{"x", "y", "width", "height", "color"}, Description: fillRectangleDesc,
	})
	addObject[FillRoundedRectangleInput](r, toolDef{
		Name: "fill_rounded_rectangle", Command: "fill_rounded_rectangle",
		Required: []string{"x", "y", "width", "height", "color"}, Description: fillRoundedRectangleDesc,
	})
	addObject[FillEllipseInput](r, toolDef{
		Name: "fill_ellipse", Command: "fill_ellipse",
		Required: []string{"x", "y", "width", "height", "color"}, Description: fillEllipseDesc,
	})
	addObject[GradientFillInput](r, toolDef{
		Name: "gradient_fill", Command: "gradient_fill",
		Required: nil, Description: gradientFillDesc,
	})
	addObject[GetPixelColorInput](r, toolDef{
		Name: "get_pixel_color", Command: "get_pixel_color",
		Required: []string{"x", "y"}, Description: getPixelColorDesc,
	})
}

// getContextStateDesc documents the get_context_state tool.
const getContextStateDesc = `Get the current GIMP context state (colors, brush, settings).

IMPORTANT: Context state can be changed by the user in GIMP UI at any time.
Check context state before operations that depend on specific settings.

Returns information about:
- Foreground and background colors (RGB/RGBA values)
- Current brush and its properties
- Opacity setting (0-100%)
- Paint/blend mode
- Feather state and radius
- Antialiasing state

Use cases:
- Verify colors before drawing operations
- Check if feathering is enabled (avoid unwanted blurry edges)
- Ensure correct opacity and blend mode
- Detect if user changed settings in GIMP UI

Returns:
- Dictionary containing current context state
- Raises exception if unable to get context state`

// GetContextStateInput holds the arguments for the get_context_state tool.
type GetContextStateInput struct{}

// setColorsDesc documents the set_colors tool.
const setColorsDesc = `Set the GIMP foreground and/or background color.

Parameters:
- foreground: New foreground color (CSS name, hex, rgb()); omit to leave unchanged
- background: New background color; omit to leave unchanged

Returns: {foreground, background} confirmation dict.`

// SetColorsInput holds the arguments for the set_colors tool.
type SetColorsInput struct {
	Foreground *string `json:"foreground" jsonschema:"New foreground color (CSS name, hex, rgb()); omit to leave unchanged"`
	Background *string `json:"background" jsonschema:"New background color; omit to leave unchanged"`
}

// drawLineDesc documents the draw_line tool.
const drawLineDesc = `Draw a straight line on a layer.

Parameters:
- x1, y1: Start point
- x2, y2: End point
- color: Stroke color (CSS / hex / rgb); uses current foreground if omitted
- width: Stroke width in pixels (default 2.0)
- tool: "pencil" (default, hard edge) or "paintbrush" (soft edge)
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict.`

// DrawLineInput holds the arguments for the draw_line tool.
type DrawLineInput struct {
	X1         float64  `json:"x1" jsonschema:"Start point"`
	Y1         float64  `json:"y1" jsonschema:"Start point"`
	X2         float64  `json:"x2" jsonschema:"End point"`
	Y2         float64  `json:"y2" jsonschema:"End point"`
	Color      *string  `json:"color" jsonschema:"Stroke color (CSS / hex / rgb); uses current foreground if omitted"`
	Width      *float64 `json:"width" jsonschema:"Stroke width in pixels (default 2.0)"`
	Tool       *string  `json:"tool" jsonschema:"\"pencil\" (default, hard edge) or \"paintbrush\" (soft edge)" enum:"pencil,paintbrush" project:"Picks the paint procedure: gimp-pencil or gimp-paintbrush-default."`
	LayerName  *string  `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int     `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int      `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the draw_line tool.
func (in *DrawLineInput) SetDefaults() {
	if in.Width == nil {
		in.Width = ptr(2.0)
	}
	if in.Tool == nil {
		in.Tool = ptr("pencil")
	}
}

// drawRectangleDesc documents the draw_rectangle tool.
const drawRectangleDesc = `Draw a rectangle outline (stroke only) on a layer.

Parameters:
- x, y: Top-left corner
- width, height: Rectangle dimensions
- color: Stroke color; uses current foreground if omitted
- line_width: Stroke width in pixels (default 2.0)
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict.`

// DrawRectangleInput holds the arguments for the draw_rectangle tool.
type DrawRectangleInput struct {
	X          int      `json:"x" jsonschema:"Top-left corner"`
	Y          int      `json:"y" jsonschema:"Top-left corner"`
	Width      int      `json:"width" jsonschema:"Rectangle dimensions"`
	Height     int      `json:"height" jsonschema:"Rectangle dimensions"`
	Color      *string  `json:"color" jsonschema:"Stroke color; uses current foreground if omitted"`
	LineWidth  *float64 `json:"line_width" jsonschema:"Stroke width in pixels (default 2.0)"`
	LayerName  *string  `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int     `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int      `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the draw_rectangle tool.
func (in *DrawRectangleInput) SetDefaults() {
	if in.LineWidth == nil {
		in.LineWidth = ptr(2.0)
	}
}

// drawRoundedRectangleDesc documents the draw_rounded_rectangle tool.
const drawRoundedRectangleDesc = `Draw a rounded rectangle outline (stroke only) on a layer.

Parameters:
- x, y: Top-left corner
- width, height: Rectangle dimensions
- radius: Corner radius in pixels
- radius_x, radius_y: Per-axis radii, for elliptical corners
- color: Stroke color; uses current foreground if omitted
- line_width: Stroke width in pixels (default 2.0)
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict.`

// DrawRoundedRectangleInput holds the arguments for the draw_rounded_rectangle tool.
type DrawRoundedRectangleInput struct {
	X          int      `json:"x" jsonschema:"Top-left corner"`
	Y          int      `json:"y" jsonschema:"Top-left corner"`
	Width      int      `json:"width" jsonschema:"Rectangle dimensions"`
	Height     int      `json:"height" jsonschema:"Rectangle dimensions"`
	Radius     float64  `json:"radius" jsonschema:"Corner radius in pixels; sets both axes unless radius_x/radius_y are given"`
	RadiusX    float64  `json:"radius_x" jsonschema:"Horizontal corner radius (defaults to radius)"`
	RadiusY    float64  `json:"radius_y" jsonschema:"Vertical corner radius (defaults to radius)"`
	Color      *string  `json:"color" jsonschema:"Stroke color; uses current foreground if omitted"`
	LineWidth  *float64 `json:"line_width" jsonschema:"Stroke width in pixels (default 2.0)"`
	LayerName  *string  `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int     `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int      `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the draw_rounded_rectangle tool.
func (in *DrawRoundedRectangleInput) SetDefaults() {
	if in.LineWidth == nil {
		in.LineWidth = ptr(2.0)
	}
}

// drawEllipseDesc documents the draw_ellipse tool.
const drawEllipseDesc = `Draw an ellipse outline (stroke only) on a layer.

Parameters:
- x, y: Top-left corner of the bounding box
- width, height: Bounding box dimensions
- color: Stroke color; uses current foreground if omitted
- line_width: Stroke width in pixels (default 2.0)
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict.`

// DrawEllipseInput holds the arguments for the draw_ellipse tool.
type DrawEllipseInput struct {
	X          int      `json:"x" jsonschema:"Top-left corner of the bounding box"`
	Y          int      `json:"y" jsonschema:"Top-left corner of the bounding box"`
	Width      int      `json:"width" jsonschema:"Bounding box dimensions"`
	Height     int      `json:"height" jsonschema:"Bounding box dimensions"`
	Color      *string  `json:"color" jsonschema:"Stroke color; uses current foreground if omitted"`
	LineWidth  *float64 `json:"line_width" jsonschema:"Stroke width in pixels (default 2.0)"`
	LayerName  *string  `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int     `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int      `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the draw_ellipse tool.
func (in *DrawEllipseInput) SetDefaults() {
	if in.LineWidth == nil {
		in.LineWidth = ptr(2.0)
	}
}

// fillRectangleDesc documents the fill_rectangle tool.
const fillRectangleDesc = `Fill a rectangular region with a solid color.

Parameters:
- x, y: Top-left corner
- width, height: Rectangle dimensions
- color: Fill color (CSS name, hex, or rgb() string)
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict.`

// FillRectangleInput holds the arguments for the fill_rectangle tool.
type FillRectangleInput struct {
	X          int     `json:"x" jsonschema:"Top-left corner"`
	Y          int     `json:"y" jsonschema:"Top-left corner"`
	Width      int     `json:"width" jsonschema:"Rectangle dimensions"`
	Height     int     `json:"height" jsonschema:"Rectangle dimensions"`
	Color      string  `json:"color" jsonschema:"Fill color (CSS name, hex, or rgb() string)"`
	LayerName  *string `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// fillRoundedRectangleDesc documents the fill_rounded_rectangle tool.
const fillRoundedRectangleDesc = `Fill a rectangle with rounded corners.

Cards, banners, buttons and pills are this shape. A pill is a rounded
rectangle whose radius is half its height.

Parameters:
- x, y: Top-left corner
- width, height: Rectangle dimensions
- radius: Corner radius in pixels
- radius_x, radius_y: Per-axis radii, for elliptical corners
- color: Fill color (CSS name, hex, or rgb() string)
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict.`

// FillRoundedRectangleInput holds the arguments for the fill_rounded_rectangle tool.
type FillRoundedRectangleInput struct {
	X          int     `json:"x" jsonschema:"Top-left corner"`
	Y          int     `json:"y" jsonschema:"Top-left corner"`
	Width      int     `json:"width" jsonschema:"Rectangle dimensions"`
	Height     int     `json:"height" jsonschema:"Rectangle dimensions"`
	Radius     float64 `json:"radius" jsonschema:"Corner radius in pixels; sets both axes unless radius_x/radius_y are given"`
	RadiusX    float64 `json:"radius_x" jsonschema:"Horizontal corner radius (defaults to radius)"`
	RadiusY    float64 `json:"radius_y" jsonschema:"Vertical corner radius (defaults to radius)"`
	Color      string  `json:"color" jsonschema:"Fill color (CSS name, hex, or rgb() string)"`
	LayerName  *string `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// fillEllipseDesc documents the fill_ellipse tool.
const fillEllipseDesc = `Fill an elliptical region with a solid color.

Parameters:
- x, y: Top-left corner of the bounding box
- width, height: Bounding box dimensions
- color: Fill color (CSS name, hex, or rgb() string)
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict.`

// FillEllipseInput holds the arguments for the fill_ellipse tool.
type FillEllipseInput struct {
	X          int     `json:"x" jsonschema:"Top-left corner of the bounding box"`
	Y          int     `json:"y" jsonschema:"Top-left corner of the bounding box"`
	Width      int     `json:"width" jsonschema:"Bounding box dimensions"`
	Height     int     `json:"height" jsonschema:"Bounding box dimensions"`
	Color      string  `json:"color" jsonschema:"Fill color (CSS name, hex, or rgb() string)"`
	LayerName  *string `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// gradientFillDesc documents the gradient_fill tool.
const gradientFillDesc = `Fill a layer or selection with a gradient.

Parameters:
- color1: Start color (default "black")
- color2: End color (default "white")
- x1, y1: Gradient start point (default top-left 0,0)
- x2, y2: Gradient end point (defaults to bottom-right of image)
- gradient_type: "linear" (default) or "radial"
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict.`

// GradientFillInput holds the arguments for the gradient_fill tool.
type GradientFillInput struct {
	Color1       *string  `json:"color1" jsonschema:"Start color (default \"black\")"`
	Color2       *string  `json:"color2" jsonschema:"End color (default \"white\")"`
	X1           float64  `json:"x1" jsonschema:"Gradient start point (default top-left 0,0)"`
	Y1           float64  `json:"y1" jsonschema:"Gradient start point (default top-left 0,0)"`
	X2           *float64 `json:"x2" jsonschema:"Gradient end point (defaults to bottom-right of image)"`
	Y2           *float64 `json:"y2" jsonschema:"Gradient end point (defaults to bottom-right of image)"`
	GradientType *string  `json:"gradient_type" jsonschema:"Gradient shape, by GIMP's name for it: \"linear\" (default), \"radial\", \"bilinear\", \"square\", \"conical-symmetric\", and others" enum:"linear,bilinear,radial,square,conical-symmetric,conical-asymmetric,shapeburst-angular,shapeburst-spherical,shapeburst-dimpled,spiral-clockwise,spiral-anticlockwise" gimp:"gimp-drawable-edit-gradient-fill.gradient-type"`
	LayerName    *string  `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID      *int     `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex   int      `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the gradient_fill tool.
func (in *GradientFillInput) SetDefaults() {
	if in.Color1 == nil {
		in.Color1 = ptr("black")
	}
	if in.Color2 == nil {
		in.Color2 = ptr("white")
	}
	if in.GradientType == nil {
		in.GradientType = ptr("linear")
	}
}

// getPixelColorDesc documents the get_pixel_color tool.
const getPixelColorDesc = `Get the color of a single pixel.

By default this reads the composite — what the image actually looks like at
that point, with every visible layer blended. Naming a layer (by layer_name
or layer_id), or passing composite=False, reads that one layer's own pixel
instead, which can differ: a layer covered by the stack still has its own
color there.

Parameters:
- x, y: Pixel coordinates
- composite: Sample the flattened image (default true; false samples one layer)
- layer_name: Layer to sample; defaults to the active layer, and implies composite=False
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns: {color, x, y, composite} where color is a CSS "rgba(r,g,b,a)" string
with a 0-1 alpha.`

// GetPixelColorInput holds the arguments for the get_pixel_color tool.
type GetPixelColorInput struct {
	X          int     `json:"x" jsonschema:"Pixel coordinates"`
	Y          int     `json:"y" jsonschema:"Pixel coordinates"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
	LayerName  *string `json:"layer_name" jsonschema:"Layer to sample from; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	Composite  *bool   `json:"composite" jsonschema:"Sample the flattened image as displayed (default true, or false when layer_name or layer_id is given)"`
}
