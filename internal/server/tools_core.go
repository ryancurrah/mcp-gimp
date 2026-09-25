// Tool definitions for the plug-in's core commands: connection, canvases, images and the PDB escape hatch.
//
// Each tool is its description, its argument struct and, where arguments
// have non-zero defaults, SetDefaults. Argument constraints are struct tags;
// see schema.go for what each means and CONTRIBUTING.md for where they come
// from.

package server

// registerCoreTools wires up the pass-through tools for the plug-in's core commands: connection, canvases, images and the PDB escape hatch.
func registerCoreTools(r *registrar) {
	addObject[NewCanvasInput](r, toolDef{
		Name: "new_canvas", Command: "new_canvas",
		Required: []string{"width", "height"}, Description: newCanvasDesc,
	})
	addObject[GetImageMetadataInput](r, toolDef{
		Name: "get_image_metadata", Command: "get_image_metadata",
		Required: nil, Description: getImageMetadataDesc,
	})
	addObject[GetGimpInfoInput](r, toolDef{
		Name: "get_gimp_info", Command: "get_gimp_info",
		Required: nil, Description: getGimpInfoDesc,
	})
	addObject[DescribeProcedureInput](r, toolDef{
		Name: "describe_procedure", Command: "describe_procedure",
		Required: []string{"api_path"}, Description: describeProcedureDesc,
	})
	addObject[OpenImageInput](r, toolDef{
		Name: "open_image", Command: "open_image",
		Required: []string{"file_path"}, Description: openImageDesc,
	})
	addObject[ListLayersInput](r, toolDef{
		Name: "list_layers", Command: "list_layers",
		Required: nil, Description: listLayersDesc,
	})
	addObject[ListImagesInput](r, toolDef{
		Name: "list_images", Command: "list_images",
		Required: nil, Description: listImagesDesc,
	})
}

// checkServerDesc documents the check_server tool.
const checkServerDesc = `Check whether the GIMP MCP plugin socket is reachable and responding.

Returns a status dict:
- connected: bool
- host / port: where it tried
- gimp_version: if connected successfully
- error: description if not connected

Use this before any other operation to verify the GIMP plugin is running.
If not connected, open GIMP and run Tools > Start MCP Server.`

// CheckServerInput holds the arguments for the check_server tool.
type CheckServerInput struct{}

// restartServerDesc documents the restart_server tool.
const restartServerDesc = `Drop and re-establish the connection to the GIMP MCP plugin.

Use this when:
- GIMP was restarted after Claude Code was already running
- The socket connection dropped mid-session
- check_server() shows not connected but GIMP is open

Returns the new connection status (same format as check_server).`

// RestartServerInput holds the arguments for the restart_server tool.
type RestartServerInput struct{}

// newCanvasDesc documents the new_canvas tool.
const newCanvasDesc = `Create a new blank canvas in GIMP and open it in a display window.

Parameters:
- width: Canvas width in pixels
- height: Canvas height in pixels
- name: Layer/image name (default: "Untitled")
- color_mode: "RGB" (default), "RGBA", "GRAY", "GRAYA"
- fill: Fill color for the background layer. Any CSS color name or
        hex string: "white" (default), "black", "transparent",
        "#FF5733", "rgb(100,200,50)", etc.
- resolution: DPI resolution (default: 72)

Returns:
- image_id: internal GIMP image ID
- layer_id: the background layer's ID, for the tools that take layer_id
- width / height: confirmed dimensions
- color_mode: confirmed mode
- display_opened: whether a GIMP window was opened

Examples:
- new_canvas(1024, 1024) — white 1024x1024 RGB canvas
- new_canvas(1920, 1080, name="Background", fill="black")
- new_canvas(512, 512, color_mode="RGBA", fill="transparent")`

// NewCanvasInput holds the arguments for the new_canvas tool.
type NewCanvasInput struct {
	Width      int     `json:"width" jsonschema:"Canvas width in pixels"`
	Height     int     `json:"height" jsonschema:"Canvas height in pixels"`
	Name       *string `json:"name" jsonschema:"Layer/image name (default: \"Untitled\")"`
	ColorMode  *string `json:"color_mode" jsonschema:"\"RGB\" (default), \"RGBA\", \"GRAY\", \"GRAYA\"" enum:"RGB,RGBA,GRAY,GRAYA" project:"The protocol names colour modes as GIMP's dialogs do; the plug-in turns each into a GimpImageBaseType and an alpha flag."`
	Fill       *string `json:"fill" jsonschema:"Fill color for the background layer. Any CSS color name or hex string: \"white\" (default), \"black\", \"transparent\", \"#FF5733\", \"rgb(100,200,50)\", etc."`
	Resolution *int    `json:"resolution" jsonschema:"DPI resolution (default: 72)"`
}

// SetDefaults applies the defaults documented for the new_canvas tool.
func (in *NewCanvasInput) SetDefaults() {
	if in.Name == nil {
		in.Name = ptr("Untitled")
	}
	if in.ColorMode == nil {
		in.ColorMode = ptr("RGB")
	}
	if in.Fill == nil {
		in.Fill = ptr("white")
	}
	if in.Resolution == nil {
		in.Resolution = ptr(72)
	}
}

// getImageMetadataDesc documents the get_image_metadata tool.
const getImageMetadataDesc = `Get metadata about the current open image in GIMP without the bitmap data.

Returns detailed information about the currently active image including:
- Image dimensions (width, height)
- Color mode and base type
- Number of layers and channels
- File information if available
- Layer structure and properties

This is much faster than get_image_bitmap() since it doesn't export the actual image data.
Perfect for when you only need to know image properties for decision making.

Returns:
- Dictionary containing comprehensive image metadata
- Raises exception if no images are open`

// GetImageMetadataInput holds the arguments for the get_image_metadata tool.
type GetImageMetadataInput struct{}

// getGimpInfoDesc documents the get_gimp_info tool.
const getGimpInfoDesc = `Get comprehensive information about the GIMP installation and environment.

Returns detailed information about GIMP that AI assistants need to understand
the current environment, including:
- GIMP version and build information
- Installation paths and directories
- Available plugins and procedures
- System configuration
- Runtime environment details

This information helps AI assistants provide better support and troubleshooting
by understanding the specific GIMP setup they're working with.

Returns:
- Dictionary containing comprehensive GIMP environment information
- Raises exception if GIMP connection fails`

// GetGimpInfoInput holds the arguments for the get_gimp_info tool.
type GetGimpInfoInput struct{}

// callAPIDesc documents the call_api tool.
const callAPIDesc = `Run a GIMP procedure from the Procedural Database (PDB).

This is the escape hatch for anything the dedicated tools do not cover. It
names a procedure and passes its arguments; it does NOT execute source code.
When a dedicated tool does the job (delete_layer rather than
gimp-image-remove-layer, set_layer_offsets rather than gimp-layer-set-offsets),
use that tool instead.

The plug-in is built against libgimp, so there is no interpreter inside GIMP
to evaluate snippets. Anything you would write in GIMP's Python-Fu console is
expressed as the procedure it calls:

    Python-Fu                      call_api
    Gimp.get_images()              api_path="gimp-get-images"
    image.get_layers()             api_path="gimp-image-get-layers",
                                   kwargs={"image": 1}
    Gimp.displays_flush()          api_path="gimp-displays-flush"

Parameters:
- api_path: The PDB procedure name, e.g. "gimp-image-get-layers". GIMP spells
            these with dashes.
- args: Positional arguments, matched in order against the procedure's
        declared arguments.
- kwargs: Arguments by name. Preferred, and required when skipping any.

Argument values:
- Images, layers, drawables, channels and fonts are passed as integer ids,
  the same ids the other tools return.
- An image is its image_id, as new_canvas, open_image and list_images report
  it. That is not the image_index the other tools take: image_index is a
  position in list_images, which shifts as images open and close, while an
  image_id stays fixed. Check list_images when unsure.
- Colors are CSS strings: "white", "#ff5733", "rgb(100,200,50)".
- Coordinate lists are flat arrays of numbers.
- Pass -1 for an object argument that should be NULL, such as a layer's
  parent.

Call describe_procedure first when you do not know a procedure's exact
argument names; GIMP rejects unknown names rather than ignoring them.

Returns:
- JSON with the procedure name and its return values. Object results are
  returned as integer ids.

Full procedure reference: https://developer.gimp.org/api/3.0/libgimp/`

// CallAPIInput holds the arguments for the call_api tool.
type CallAPIInput struct {
	APIPath string         `json:"api_path" jsonschema:"The PDB procedure name, e.g. \"gimp-image-get-layers\""`
	Args    []any          `json:"args" jsonschema:"Positional arguments matched in order against the procedure's signature"`
	Kwargs  map[string]any `json:"kwargs" jsonschema:"Arguments by name; preferred over args"`
}

// SetDefaults applies the defaults documented for the call_api tool.
func (in *CallAPIInput) SetDefaults() {
	if in.Args == nil {
		in.Args = []any{}
	}
	if in.Kwargs == nil {
		in.Kwargs = map[string]any{}
	}
}

// describeProcedureDesc documents the describe_procedure tool.
const describeProcedureDesc = `List the arguments a GIMP PDB procedure takes, as the installed GIMP declares
them.

Use this before call_api to discover exact argument names, types, ranges and
permitted names. GIMP requires names to match precisely, refuses a number
outside an argument's range, and takes an enum argument by any of its listed
choices.

Parameters:
- api_path: The PDB procedure name, e.g. "gimp-image-scale"

Returns:
- procedure: the name queried
- arguments: ordered list of {name, type, minimum, maximum, default, choices, blurb};
  the range, default and choices appear only where GIMP declares them

Example:
    describe_procedure(api_path="gimp-drawable-get-pixel")
    -> arguments: drawable (GimpDrawable), x-coord (gint), y-coord (gint)`

// DescribeProcedureInput holds the arguments for the describe_procedure tool.
type DescribeProcedureInput struct {
	APIPath string `json:"api_path" jsonschema:"The PDB procedure name, e.g. \"gimp-image-scale\""`
}

// openImageDesc documents the open_image tool.
const openImageDesc = `Open an image file in GIMP and create a display window.

Parameters:
- file_path: Absolute path to the image file to open (PNG, JPEG, TIFF, etc.)

Returns:
- image_id: internal GIMP image ID
- width / height: image dimensions in pixels
- color_mode: RGB / Grayscale / Indexed
- num_layers: number of layers in the image
- display_opened: whether a GIMP display window was created`

// OpenImageInput holds the arguments for the open_image tool.
type OpenImageInput struct {
	FilePath string `json:"file_path" jsonschema:"Absolute path to the image file to open (PNG, JPEG, TIFF, etc.)"`
}

// listLayersDesc documents the list_layers tool.
const listLayersDesc = `List all layers in an image with their properties.

Parameters:
- image_index: Target image index (default 0)

Returns: {layers: [{name, id, visible, opacity, blend_mode, width, height, has_alpha}], count}`

// ListLayersInput holds the arguments for the list_layers tool.
type ListLayersInput struct {
	ImageIndex int `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// listImagesDesc documents the list_images tool.
const listImagesDesc = `List all images currently open in GIMP.

Returns:
- images: list of {index, image_id, name, width, height, file}
- count: total number of open images

index is what the other tools take as image_index. It is a position in this
list, most recently opened first, so it shifts when an image is opened or
closed. image_id is fixed for the life of the image; call_api takes an image
by its image_id.`

// ListImagesInput holds the arguments for the list_images tool.
type ListImagesInput struct{}
