// Tool definitions for multi-step operations built from several procedures.
//
// Each tool is its description, its argument struct and, where arguments
// have non-zero defaults, SetDefaults. Argument constraints are struct tags;
// see schema.go for what each means and CONTRIBUTING.md for where they come
// from.

package server

// registerCompositeTools wires up the pass-through tools for multi-step operations built from several procedures.
func registerCompositeTools(r *registrar) {
	addObject[WarpRegionInput](r, toolDef{
		Name: "warp_region", Command: "warp_region",
		Required: []string{"vectors"}, Description: warpRegionDesc,
	})
	addObject[ExportSpriteSheetInput](r, toolDef{
		Name: "export_sprite_sheet", Command: "export_sprite_sheet",
		Required: []string{"output_path"}, Description: exportSpriteSheetDesc,
	})
	addObject[ExportSocialMediaKitInput](r, toolDef{
		Name: "export_social_media_kit", Command: "export_social_media_kit",
		Required: []string{"output_dir"}, Description: exportSocialMediaKitDesc,
	})
}

// warpRegionDesc documents the warp_region tool.
const warpRegionDesc = `Warp / liquify a region of the image by pushing pixels in a direction.

Uses GEGL warp (GIMP 3 native) with plug-in-iwarp fallback. Ideal for
subtle facial expression edits — e.g. turning a neutral mouth into a smile
by pushing the mouth corners upward.

Parameters:
- vectors: List of warp stroke dicts, each with:
    - x, y      : center of the warp influence (pixels)
    - dx, dy    : push direction — negative dy = push upward
    - radius    : influence radius in pixels (default: 40)
    - amount    : deform strength 0–1 (default: 0.3)
- image_index: Which open image to edit (default: 0)
- layer_name: Target layer; omit to use the active/top layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only

Examples — make a character smile:
    warp_region(vectors=[
        {"x": 215, "y": 355, "dx":  5, "dy": -8, "radius": 18, "amount": 0.45},
        {"x": 295, "y": 355, "dx": -5, "dy": -8, "radius": 18, "amount": 0.45},
        {"x": 255, "y": 370, "dx":  0, "dy": -4, "radius": 22, "amount": 0.30},
    ])

Returns: {"warped_vectors": N}`

// WarpRegionInput holds the arguments for the warp_region tool.
type WarpRegionInput struct {
	Vectors    []WarpVector `json:"vectors" jsonschema:"Warp strokes to apply, each pushing the pixels around one point"`
	ImageIndex int          `json:"image_index" jsonschema:"Which open image to edit (default: 0)"`
	LayerName  *string      `json:"layer_name" jsonschema:"Target layer; omit to use the active/top layer"`
	LayerID    *int         `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
}

// WarpVector is one warp stroke.
type WarpVector struct {
	X      float64  `json:"x" jsonschema:"Centre of the warp influence, in pixels"`
	Y      float64  `json:"y" jsonschema:"Centre of the warp influence, in pixels"`
	DX     float64  `json:"dx" jsonschema:"Push distance along x; negative pushes left"`
	DY     float64  `json:"dy" jsonschema:"Push distance along y; negative pushes upward"`
	Radius *float64 `json:"radius,omitempty" jsonschema:"Influence radius in pixels (default 40)"`
	Amount *float64 `json:"amount,omitempty" jsonschema:"Deform strength from 0 to 1 (default 0.3)"`
}

// exportSpriteSheetDesc documents the export_sprite_sheet tool.
const exportSpriteSheetDesc = `Combine multiple frames into a sprite sheet PNG.

Parameters:
- output_path: Absolute path for the output PNG file
- columns: Number of columns in the grid (defaults to square root of frame count)
- padding: Pixel gap between frames (default 0)
- source: "layers" (each layer is a frame; default) or "images" (each open image)
- image_index: Source image when source="layers" (default 0)

Returns: {file_path, columns, rows, frame_width, frame_height, count}`

// ExportSpriteSheetInput holds the arguments for the export_sprite_sheet tool.
type ExportSpriteSheetInput struct {
	OutputPath string  `json:"output_path" jsonschema:"Absolute path for the output PNG file"`
	Columns    *int    `json:"columns" jsonschema:"Number of columns in the grid (defaults to square root of frame count)"`
	Padding    int     `json:"padding" jsonschema:"Pixel gap between frames (default 0)"`
	Source     *string `json:"source" jsonschema:"\"layers\" (each layer is a frame; default) or \"images\" (each open image)" enum:"layers,images" project:"Whether the frames are the layers of one image or several open images."`
	ImageIndex int     `json:"image_index" jsonschema:"Source image when source=\"layers\" (default 0)"`
}

// SetDefaults applies the defaults documented for the export_sprite_sheet tool.
func (in *ExportSpriteSheetInput) SetDefaults() {
	if in.Source == nil {
		in.Source = ptr("layers")
	}
}

// exportSocialMediaKitDesc documents the export_social_media_kit tool.
const exportSocialMediaKitDesc = `Export an image resized for multiple social media platforms.

Platform sizes (all in pixels):
- instagram_square: 1080x1080
- instagram_story: 1080x1920
- twitter_header: 1500x500
- facebook_cover: 820x312
- youtube_thumbnail: 1280x720

Parameters:
- output_dir: Directory to write output files
- platforms: List of platform names to export (omit for all five)
- image_index: Source image index (default 0)

Returns: {exported: [{platform, file_path, width, height}], count}`

// ExportSocialMediaKitInput holds the arguments for the export_social_media_kit tool.
type ExportSocialMediaKitInput struct {
	OutputDir  string `json:"output_dir" jsonschema:"Directory to write output files"`
	Platforms  []any  `json:"platforms" jsonschema:"List of platform names to export (omit for all five)"`
	ImageIndex int    `json:"image_index" jsonschema:"Source image index (default 0)"`
}
