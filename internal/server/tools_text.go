// Tool definitions for text layers and fonts.
//
// Each tool is its description, its argument struct and, where arguments
// have non-zero defaults, SetDefaults. Argument constraints are struct tags;
// see schema.go for what each means and CONTRIBUTING.md for where they come
// from.

package server

// registerTextTools wires up the pass-through tools for text layers and fonts.
func registerTextTools(r *registrar) {
	addObject[AddTextInput](r, toolDef{
		Name: "add_text", Command: "add_text",
		Required: []string{"text"}, Description: addTextDesc,
	})
	addObject[EditTextInput](r, toolDef{
		Name: "edit_text", Command: "edit_text",
		Required: nil, Description: editTextDesc,
	})
	addObject[ListFontsInput](r, toolDef{
		Name: "list_fonts", Command: "list_fonts",
		Required: nil, Description: listFontsDesc,
	})
}

// addTextDesc documents the add_text tool.
const addTextDesc = `Add a text layer to an image.

Parameters:
- text: The text string to render; separate lines with \n
- x, y: Position of the text layer's top-left corner (default 0, 0)
- font: Font name as list_fonts reports it, e.g. "Georgia Bold Italic"
        (default "Sans-serif"). A family name such as "Georgia" picks its
        Regular face. A font GIMP does not have is refused, with the
        closest matches in the error.
- size: Font size in pixels (default 24)
- color: Text color (CSS name, hex, or rgb() string; default "black")
- align: "left" (default, places the text at x), "center" to centre it on the
         image, or "right" to place it x pixels in from the right edge
- justify: How lines line up within multi-line text: "left" (default),
           "center", "right" or "fill". align moves the whole layer; justify
           arranges the lines inside it.
- position: Stack position counted from the top — 0 = top (default);
            -1 = directly above the active layer
- image_index: Target image index (default 0)

Centring needs the rendered width, so prefer align over measuring the text
and repositioning the layer yourself.

Returns: {layer_name, layer_id, font, text_width, text_height, position}
Use layer_id to refer to the layer afterwards: a text layer's name follows
its text, so it changes when the text is edited.`

// AddTextInput holds the arguments for the add_text tool.
type AddTextInput struct {
	Text       string  `json:"text" jsonschema:"The text string to render; separate lines with \\n"`
	X          int     `json:"x" jsonschema:"Position of the text layer's top-left corner (default 0, 0)"`
	Y          int     `json:"y" jsonschema:"Position of the text layer's top-left corner (default 0, 0)"`
	Font       *string `json:"font" jsonschema:"Font name as list_fonts reports it (default \"Sans-serif\"); a family name picks its Regular face, and an unknown font is refused"`
	Size       *int    `json:"size" jsonschema:"Font size in pixels (default 24)"`
	Color      *string `json:"color" jsonschema:"Text color (CSS name, hex, or rgb() string; default \"black\")"`
	Align      *string `json:"align" jsonschema:"Horizontal placement: \"left\" (default, uses x), \"center\", or \"right\" (x is the right margin)" enum:"left,center,right" project:"Placement relative to the canvas, which the plug-in computes."`
	Justify    *string `json:"justify" jsonschema:"How lines line up within multi-line text (default \"left\")" enum:"left,right,center,fill" gimp:"gimp-text-layer-set-justification.justify"`
	Position   *int    `json:"position" jsonschema:"Stack position counted from the top: 0 = top (default); -1 = directly above the active layer"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the add_text tool.
func (in *AddTextInput) SetDefaults() {
	if in.Font == nil {
		in.Font = ptr("Sans-serif")
	}
	if in.Size == nil {
		in.Size = ptr(24)
	}
	if in.Color == nil {
		in.Color = ptr("black")
	}
	if in.Justify == nil {
		in.Justify = ptr("left")
	}
	if in.Position == nil {
		in.Position = ptr(0)
	}
}

// editTextDesc documents the edit_text tool.
const editTextDesc = `Edit an existing text layer's content or formatting.

Changing the text, font or size reflows the layer. Pass align to keep a
centred or right-aligned caption in place; without it the layer keeps its
old offset and appears to drift as the width changes.

Parameters:
- layer_id: The layer_id add_text returned. Prefer it to layer_name: a text
            layer's name follows its text, so it changes with each edit.
- layer_name: Name of the text layer to edit
- text: New text content (omit to leave unchanged)
- font: New font name as list_fonts reports it (omit to leave unchanged);
        an unknown font is refused
- size: New font size in pixels (omit to leave unchanged)
- color: New text color (omit to leave unchanged)
- justify: How lines line up within the layer: "left", "center", "right"
           or "fill" (omit to leave unchanged)
- align: Re-apply "left", "center" or "right" after the edit
- x: Margin used by align (default 0)
- image_index: Target image index (default 0)

Identify the layer one way only; with neither, the active layer is edited.

Filters applied through this server stay editable on a text layer. A text
layer whose pixels were changed directly (painted on, or a filter merged in
GIMP) no longer re-renders, so an edit to it is refused; add the text again.

Returns: {status, layer_id, layer_name, text_width, text_height, position}`

// EditTextInput holds the arguments for the edit_text tool.
type EditTextInput struct {
	LayerID    *int     `json:"layer_id" jsonschema:"The layer_id add_text returned; it names the image too, so image_index is not consulted"`
	LayerName  *string  `json:"layer_name" jsonschema:"Name of the text layer to edit"`
	Text       *string  `json:"text" jsonschema:"New text content (omit to leave unchanged)"`
	Font       *string  `json:"font" jsonschema:"New font name as list_fonts reports it (omit to leave unchanged); an unknown font is refused"`
	Size       *float64 `json:"size" jsonschema:"New font size in pixels (omit to leave unchanged)"`
	Color      *string  `json:"color" jsonschema:"New text color (omit to leave unchanged)"`
	Justify    *string  `json:"justify" jsonschema:"How lines line up within the layer (omit to leave unchanged)" enum:"left,right,center,fill" gimp:"gimp-text-layer-set-justification.justify"`
	ImageIndex int      `json:"image_index" jsonschema:"Target image index (default 0)"`
	Align      *string  `json:"align" jsonschema:"Re-apply this alignment after the edit: \"left\", \"center\" or \"right\"" enum:"left,center,right" project:"Placement relative to the canvas, which the plug-in computes."`
	X          int      `json:"x" jsonschema:"Left margin for align=\"left\", or right margin for align=\"right\""`
}

// listFontsDesc documents the list_fonts tool.
const listFontsDesc = `List available fonts installed in GIMP.

A bare install can report well over a thousand fonts, so pass a filter
rather than reading the whole list.

Parameters:
- filter: Case-insensitive regular expression, e.g. "snell" or "^Avenir.*Bold$"

Returns: {fonts: [font_name, ...], count}`

// ListFontsInput holds the arguments for the list_fonts tool.
type ListFontsInput struct {
	Filter *string `json:"filter" jsonschema:"Case-insensitive regular expression matched against font names"`
}
