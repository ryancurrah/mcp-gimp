// Tool definitions for reading pixels back out of GIMP and writing files.
//
// Each tool is its description, its argument struct and, where arguments
// have non-zero defaults, SetDefaults. Argument constraints are struct tags;
// see schema.go for what each means and CONTRIBUTING.md for where they come
// from.

package server

// registerBitmapTools wires up the pass-through tools for reading pixels back out of GIMP and writing files.
func registerBitmapTools(r *registrar) {
	addImage[GetImageBitmapInput](r, toolDef{
		Name: "get_image_bitmap", Command: "get_image_bitmap",
		Required: nil, Description: getImageBitmapDesc,
	})
	addObject[SaveXcfInput](r, toolDef{
		Name: "save_xcf", Command: "save_xcf",
		Required: []string{"file_path"}, Description: saveXcfDesc,
	})
	addObject[ExportImageInput](r, toolDef{
		Name: "export_image", Command: "export_image",
		Required: []string{"file_path"}, Description: exportImageDesc,
	})
}

// getImageBitmapDesc documents the get_image_bitmap tool.
const getImageBitmapDesc = `Get the current open image in GIMP as an Image object with optional scaling and region selection.

No size restrictions — pass any max_width/max_height you need.
For large images, omit max_width/max_height to get the full resolution.

Supports two main use cases:
1. Full image with optional scaling (pass max_width/max_height)
2. Region extraction with optional scaling (pass region dict)

Parameters:
- image_index: Target image index (default 0)
- max_width, max_height: Target dimensions for scaling (aspect-ratio preserved).
  Omit for full resolution.
- region: Dictionary with keys:
    - origin_x, origin_y: Top-left corner of region to extract
    - width, height: Dimensions of region to extract
    - max_width, max_height: Optional scaling for the extracted region

Examples:
- Full image at full res: get_image_bitmap()
- Full image scaled: get_image_bitmap(max_width=2048, max_height=2048)
- Region: get_image_bitmap(region={"origin_x": 0, "origin_y": 0, "width": 512, "height": 512})

Returns:
- Image object containing PNG data in MCP-compliant format
- Includes width, height, and base64-encoded image data

The returned Image object automatically handles base64 encoding and MIME types
according to the Model Context Protocol specification.

Raises:
- RuntimeError if no image is open, region is invalid, or export fails`

// GetImageBitmapInput holds the arguments for the get_image_bitmap tool.
type GetImageBitmapInput struct {
	MaxWidth   *int         `json:"max_width" jsonschema:"Target dimensions for scaling (aspect-ratio preserved). Omit for full resolution."`
	MaxHeight  *int         `json:"max_height" jsonschema:"Target dimensions for scaling (aspect-ratio preserved). Omit for full resolution."`
	Region     *ImageRegion `json:"region" jsonschema:"Region of the image to extract; omit to take the whole image"`
	ImageIndex int          `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// ImageRegion is the part of the image get_image_bitmap extracts.
type ImageRegion struct {
	OriginX   *int `json:"origin_x,omitempty" jsonschema:"Left edge of the region to extract, in pixels"`
	OriginY   *int `json:"origin_y,omitempty" jsonschema:"Top edge of the region to extract, in pixels"`
	Width     int  `json:"width" jsonschema:"Width of the region to extract; must be positive"`
	Height    int  `json:"height" jsonschema:"Height of the region to extract; must be positive"`
	MaxWidth  *int `json:"max_width,omitempty" jsonschema:"Scale the extracted region down to this width"`
	MaxHeight *int `json:"max_height,omitempty" jsonschema:"Scale the extracted region down to this height"`
}

// getStateSnapshotDesc documents the get_state_snapshot tool.
const getStateSnapshotDesc = `Return a live visual snapshot of the current image state — no file save needed.

AI agents call this to get immediate visual feedback after any edit operation,
letting them verify results and decide next steps without saving to disk.

Parameters:
- image_index: Which open image to snapshot (default: 0 = most recent)
- max_size: Maximum width/height of the returned preview in pixels (default: 512)
- region: Optional dict {x, y, width, height} to zoom into a specific area
          e.g. {"x": 200, "y": 300, "width": 100, "height": 80} for mouth area
- label: Optional annotation label (logged but not drawn — for agent bookkeeping)

Returns:
- PNG image of the current GIMP canvas state (with alpha if present)

Typical agent workflow:
    1. open_image / new_canvas
    2. <edit operations>
    3. get_state_snapshot()          ← see result, decide next step
    4. <more edits>
    5. get_state_snapshot(region={"x":200,"y":300,"width":100,"height":80})
    6. export_image when satisfied`

// GetStateSnapshotInput holds the arguments for the get_state_snapshot tool.
type GetStateSnapshotInput struct {
	ImageIndex int             `json:"image_index" jsonschema:"Which open image to snapshot (default: 0 = most recent)"`
	MaxSize    *int            `json:"max_size" jsonschema:"Maximum width/height of the returned preview in pixels (default: 512)"`
	Region     *SnapshotRegion `json:"region" jsonschema:"Area to zoom into; omit to snapshot the whole canvas"`
	Label      string          `json:"label" jsonschema:"Optional annotation label (logged but not drawn — for agent bookkeeping)"`
}

// SetDefaults applies the defaults documented for the get_state_snapshot tool.
func (in *GetStateSnapshotInput) SetDefaults() {
	if in.MaxSize == nil {
		in.MaxSize = ptr(512)
	}
}

// SnapshotRegion is the area get_state_snapshot zooms into.
type SnapshotRegion struct {
	X      *int `json:"x,omitempty" jsonschema:"Left edge of the area to zoom into, in pixels"`
	Y      *int `json:"y,omitempty" jsonschema:"Top edge of the area to zoom into, in pixels"`
	Width  *int `json:"width,omitempty" jsonschema:"Width of the area to zoom into; defaults to max_size"`
	Height *int `json:"height,omitempty" jsonschema:"Height of the area to zoom into; defaults to max_size"`
}

// saveXcfDesc documents the save_xcf tool.
const saveXcfDesc = `Save the current image as a GIMP XCF file (preserves all layers and metadata).

This is File > Save: the image remembers the file and counts as saved, so
close_image accepts it and its save_first saves back to the same file.

Parameters:
- file_path: Absolute path for the output file; must end in .xcf
- image_index: Index of the image to save (default 0 = first open image)

Returns:
- status: "success" or "error"
- file_path: confirmed output path`

// SaveXcfInput holds the arguments for the save_xcf tool.
type SaveXcfInput struct {
	FilePath   string `json:"file_path" jsonschema:"Absolute path for the output file; must end in .xcf"`
	ImageIndex int    `json:"image_index" jsonschema:"Index of the image to save (default 0 = first open image)"`
}

// exportImageDesc documents the export_image tool.
const exportImageDesc = `Export the current image to a raster file (PNG, JPEG, WEBP, TIFF).

The export is made from a copy of the image, so the open image keeps its
layers whether or not the copy is flattened.

Parameters:
- file_path: Absolute path for the output file. Its extension picks the
             format: .png, .jpg/.jpeg, .webp, .tif/.tiff
- format: "png", "jpeg", "webp" or "tiff"; optional, and refused if it
          disagrees with the extension
- quality: JPEG/WEBP quality 1-100 (default 90; ignored for PNG/TIFF)
- flatten: Flatten the exported copy (default True); the open image is never flattened
- image_index: Index of the image to export (default 0)

Returns:
- status, file_path, format, file_size_bytes`

// ExportImageInput holds the arguments for the export_image tool.
type ExportImageInput struct {
	FilePath   string  `json:"file_path" jsonschema:"Absolute path for the output file"`
	Format     *string `json:"format" jsonschema:"\"png\", \"jpeg\", \"webp\" or \"tiff\"; optional, since the file extension decides the format, and refused if it disagrees with it" enum:"png,jpeg,webp,tiff" project:"GIMP picks the exporter from the file extension; this only confirms it."`
	Quality    *int    `json:"quality" jsonschema:"JPEG/WEBP quality 1-100, or 0 to use the format default (default 90; ignored for PNG/TIFF)" minimum:"0" maximum:"100" sentinel:"0" gimp:"file-jpeg-export.quality/100 file-webp-export.quality"`
	Flatten    *bool   `json:"flatten" jsonschema:"Flatten the exported copy (default True); the open image is never flattened"`
	ImageIndex int     `json:"image_index" jsonschema:"Index of the image to export (default 0)"`
}

// SetDefaults applies the defaults documented for the export_image tool.
func (in *ExportImageInput) SetDefaults() {
	if in.Quality == nil {
		in.Quality = ptr(90)
	}
	if in.Flatten == nil {
		in.Flatten = ptr(true)
	}
}
