// Tool definitions for SVG paths: stroking, filling and selecting curves.
//
// Each tool is its description, its argument struct and, where arguments
// have non-zero defaults, SetDefaults. Argument constraints are struct tags;
// see schema.go for what each means and CONTRIBUTING.md for where they come
// from.

package server

// registerPathTools wires up the pass-through tools that take SVG path data.
func registerPathTools(r *registrar) {
	addObject[DrawPathInput](r, toolDef{
		Name: "draw_path", Command: "draw_path",
		Required: []string{"d"}, Description: drawPathDesc,
	})
	addObject[FillPathInput](r, toolDef{
		Name: "fill_path", Command: "fill_path",
		Required: []string{"d", "color"}, Description: fillPathDesc,
	})
	addObject[SelectPathInput](r, toolDef{
		Name: "select_path", Command: "select_path",
		Required: []string{"d"}, Description: selectPathDesc,
	})
	addObject[ListPathsInput](r, toolDef{
		Name: "list_paths", Command: "list_paths",
		Required: nil, Description: listPathsDesc,
	})
	addObject[PathToSelectionInput](r, toolDef{
		Name: "path_to_selection", Command: "path_to_selection",
		Required: []string{"path_id"}, Description: pathToSelectionDesc,
	})
}

// drawPathDesc documents the draw_path tool.
const drawPathDesc = `Stroke the outline of an SVG path on a layer.

This is how to draw curves: anything a cartoon outline, a logo or a swooping
arrow needs that a straight draw_line or an ellipse cannot make. Give the
curve as SVG path data and GIMP does the geometry.

Parameters:
- d: SVG path data (M/L/H/V/C/S/Q/T/A/Z, absolute or relative), in image pixels
  with the origin top-left, e.g.
    M 100 300 C 150 100 350 100 400 300          (one cubic curve)
    M 200 200 Q 300 50 400 200 Z                  (closed quadratic)
    M 50 50 L 150 50 A 50 50 0 0 1 150 150 Z      (arc)
- color: Stroke color (CSS / hex / rgb); uses current foreground if omitted
- width: Stroke width in pixels (default 2.0)
- cap: Line end style: "round" (default), "butt" or "square"
- join: Corner style: "round" (default), "miter" or "bevel"
- antialias: Smooth the stroke's edges (default true)
- keep_path: Leave the path in the image's Paths dockable so it can be edited
  in the GUI; it starts hidden there (default false: the temporary path is
  removed after stroking)
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict, plus path_id when keep_path is true.`

// DrawPathInput holds the arguments for the draw_path tool.
type DrawPathInput struct {
	D          string   `json:"d" jsonschema:"SVG path data, as the d attribute of an SVG <path>: M/L/H/V/C/S/Q/T/A/Z, absolute or relative (lowercase). Coordinates are image pixels with the origin at the top-left, as everywhere else. Examples: \"M 100 300 C 150 100 350 100 400 300\" (one cubic curve); \"M 200 200 Q 300 50 400 200 Z\" (closed quadratic); \"M 50 50 L 150 50 A 50 50 0 0 1 150 150 Z\" (arc)"`
	Color      *string  `json:"color" jsonschema:"Stroke color (CSS / hex / rgb); uses current foreground if omitted"`
	Width      *float64 `json:"width" jsonschema:"Stroke width in pixels (default 2.0)" minimum:"0" maximum:"2000" gimp:"gimp-context-set-line-width.line-width"`
	Cap        *string  `json:"cap" jsonschema:"Line end style: \"round\" (default), \"butt\" or \"square\"" enum:"butt,round,square" gimp:"gimp-context-set-line-cap-style.cap-style"`
	Join       *string  `json:"join" jsonschema:"Corner style: \"round\" (default), \"miter\" or \"bevel\"" enum:"miter,round,bevel" gimp:"gimp-context-set-line-join-style.join-style"`
	Antialias  *bool    `json:"antialias" jsonschema:"Smooth the stroke's edges (default true)"`
	KeepPath   bool     `json:"keep_path" jsonschema:"Leave the path in the image's Paths dockable for editing in the GUI (default false: the temporary path is removed after stroking)"`
	LayerName  *string  `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int     `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int      `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the draw_path tool.
//
// Round caps and joins are the defaults because the tool's main use is
// freehand-style illustration, where miter spikes on tight curves look wrong.
func (in *DrawPathInput) SetDefaults() {
	if in.Width == nil {
		in.Width = ptr(2.0)
	}
	if in.Cap == nil {
		in.Cap = ptr("round")
	}
	if in.Join == nil {
		in.Join = ptr("round")
	}
	if in.Antialias == nil {
		in.Antialias = ptr(true)
	}
}

// fillPathDesc documents the fill_path tool.
const fillPathDesc = `Fill the interior of an SVG path with a solid color.

An open path is closed for filling, as SVG does, so the last point joins the
first. Use draw_path for the outline and select_path for anything more than a
flat fill.

Parameters:
- d: SVG path data (M/L/H/V/C/S/Q/T/A/Z, absolute or relative), in image pixels
  with the origin top-left, e.g. "M 200 200 Q 300 50 400 200 Z"
- color: Fill color (CSS name, hex, or rgb() string)
- antialias: Smooth the shape's edges (default true)
- keep_path: Leave the path in the image's Paths dockable (default false)
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict, plus path_id when keep_path is true.`

// FillPathInput holds the arguments for the fill_path tool.
type FillPathInput struct {
	D          string  `json:"d" jsonschema:"SVG path data, as the d attribute of an SVG <path>: M/L/H/V/C/S/Q/T/A/Z, absolute or relative (lowercase). Coordinates are image pixels with the origin at the top-left. An open path is closed for filling. Example: \"M 200 200 Q 300 50 400 200 Z\""`
	Color      string  `json:"color" jsonschema:"Fill color (CSS name, hex, or rgb() string)"`
	Antialias  *bool   `json:"antialias" jsonschema:"Smooth the shape's edges (default true)"`
	KeepPath   bool    `json:"keep_path" jsonschema:"Leave the path in the image's Paths dockable for editing in the GUI (default false: the temporary path is removed after filling)"`
	LayerName  *string `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the fill_path tool.
func (in *FillPathInput) SetDefaults() {
	if in.Antialias == nil {
		in.Antialias = ptr(true)
	}
}

// selectPathDesc documents the select_path tool.
const selectPathDesc = `Turn an SVG path into a selection.

The selection composes with modify_selection, fill_selection,
crop_to_selection and the rest, so this is how to outline a curved shape:
select_path, modify_selection grow, fill_selection with the outline colour,
modify_selection shrink, fill_selection with the body colour. An open path is
closed, as SVG does when filling.

Parameters:
- d: SVG path data (M/L/H/V/C/S/Q/T/A/Z, absolute or relative), in image pixels
  with the origin top-left, e.g. "M 100 300 C 150 100 350 100 400 300 Z"
- operation: "replace" (default), "add", "subtract", "intersect"
- feather: Feather radius in pixels (default 0 = no feather)
- antialias: Smooth the selection's edges (default true)
- keep_path: Leave the path in the image's Paths dockable (default false)
- image_index: Target image index (default 0)

Returns the selection bounds, plus path_id when keep_path is true.`

// SelectPathInput holds the arguments for the select_path tool.
type SelectPathInput struct {
	D          string  `json:"d" jsonschema:"SVG path data, as the d attribute of an SVG <path>: M/L/H/V/C/S/Q/T/A/Z, absolute or relative (lowercase). Coordinates are image pixels with the origin at the top-left. An open path is closed. Example: \"M 100 300 C 150 100 350 100 400 300 Z\""`
	Operation  *string `json:"operation" jsonschema:"\"replace\" (default), \"add\", \"subtract\", \"intersect\"" enum:"replace,add,subtract,intersect" gimp:"gimp-image-select-item.operation"`
	Feather    float64 `json:"feather" jsonschema:"Feather radius in pixels (default 0 = no feather)" minimum:"0" maximum:"1000" gimp:"gimp-context-set-feather-radius.feather-radius-x gimp-context-set-feather-radius.feather-radius-y"`
	Antialias  *bool   `json:"antialias" jsonschema:"Smooth the selection's edges (default true)"`
	KeepPath   bool    `json:"keep_path" jsonschema:"Leave the path in the image's Paths dockable for editing in the GUI (default false: the temporary path is removed after selecting)"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the select_path tool.
func (in *SelectPathInput) SetDefaults() {
	if in.Operation == nil {
		in.Operation = ptr("replace")
	}
	if in.Antialias == nil {
		in.Antialias = ptr(true)
	}
}

// listPathsDesc documents the list_paths tool.
const listPathsDesc = `List the paths of an image: ones drawn with the Paths tool in the GUI and
ones a path tool kept with keep_path.

Parameters:
- image_index: Target image index (default 0)

Returns: {image_id, num_paths, paths: [{path_id, name, visible}]}. Pass a
path_id to path_to_selection.`

// ListPathsInput holds the arguments for the list_paths tool.
type ListPathsInput struct {
	ImageIndex int `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// pathToSelectionDesc documents the path_to_selection tool.
const pathToSelectionDesc = `Turn an existing path into a selection.

For a path the user drew in the GUI, or one kept with keep_path; list_paths
shows their ids. For a curve given as SVG data, use select_path instead.

Parameters:
- path_id: The path, from list_paths or a keep_path result. It names the
  image too
- operation: "replace" (default), "add", "subtract", "intersect"
- feather: Feather radius in pixels (default 0 = no feather)
- antialias: Smooth the selection's edges (default true)

Returns the selection bounds.`

// PathToSelectionInput holds the arguments for the path_to_selection tool.
type PathToSelectionInput struct {
	PathID    int     `json:"path_id" jsonschema:"The path, from list_paths or a keep_path result; it names the image too"`
	Operation *string `json:"operation" jsonschema:"\"replace\" (default), \"add\", \"subtract\", \"intersect\"" enum:"replace,add,subtract,intersect" gimp:"gimp-image-select-item.operation"`
	Feather   float64 `json:"feather" jsonschema:"Feather radius in pixels (default 0 = no feather)" minimum:"0" maximum:"1000" gimp:"gimp-context-set-feather-radius.feather-radius-x gimp-context-set-feather-radius.feather-radius-y"`
	Antialias *bool   `json:"antialias" jsonschema:"Smooth the selection's edges (default true)"`
}

// SetDefaults applies the defaults documented for the path_to_selection tool.
func (in *PathToSelectionInput) SetDefaults() {
	if in.Operation == nil {
		in.Operation = ptr("replace")
	}
	if in.Antialias == nil {
		in.Antialias = ptr(true)
	}
}
