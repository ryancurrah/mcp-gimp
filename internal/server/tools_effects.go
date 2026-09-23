// Tool definitions for GEGL effects and batch exports.
//
// Each tool is its description, its argument struct and, where arguments
// have non-zero defaults, SetDefaults. Argument constraints are struct tags;
// see schema.go for what each means and CONTRIBUTING.md for where they come
// from.

package server

// registerEffectsTools wires up the pass-through tools for GEGL effects and batch exports.
func registerEffectsTools(r *registrar) {
	addObject[BatchExportInput](r, toolDef{
		Name: "batch_export", Command: "batch_export",
		Required: []string{"output_dir"}, Description: batchExportDesc,
	})
	addObject[ApplyDropShadowInput](r, toolDef{
		Name: "apply_drop_shadow", Command: "apply_drop_shadow",
		Required: nil, Description: applyDropShadowDesc,
	})
	addObject[ApplyGeglFilterInput](r, toolDef{
		Name: "apply_gegl_filter", Command: "apply_gegl_filter",
		Required: []string{"operation"}, Description: applyGeglFilterDesc,
	})
	addObject[DescribeOperationInput](r, toolDef{
		Name: "describe_operation", Command: "describe_operation",
		Required: []string{"operation"}, Description: describeOperationDesc,
	})
	addObject[ApplyVignetteInput](r, toolDef{
		Name: "apply_vignette", Command: "apply_vignette",
		Required: nil, Description: applyVignetteDesc,
	})
	addObject[ExportIconSizesInput](r, toolDef{
		Name: "export_icon_sizes", Command: "export_icon_sizes",
		Required: []string{"output_dir"}, Description: exportIconSizesDesc,
	})
	addObject[ExportWebOptimizedInput](r, toolDef{
		Name: "export_web_optimized", Command: "export_web_optimized",
		Required: []string{"output_dir"}, Description: exportWebOptimizedDesc,
	})
	addObject[BatchResizeInput](r, toolDef{
		Name: "batch_resize", Command: "batch_resize",
		Required: nil, Description: batchResizeDesc,
	})
}

// batchExportDesc documents the batch_export tool.
const batchExportDesc = `Export all open images (or a specific one) to a directory.

Parameters:
- output_dir: Directory to write exported files into
- format: "png", "jpeg", "webp", "tiff" (default "png")
- quality: JPEG/WEBP quality (default 90)
- name_pattern: Filename template — use {name} for image name, {index} for position
- image_index: If set, export only that image; omit to export all open images

Returns:
- exported: list of {file_path, name, width, height}
- count: number of files written
- errors: list of any export errors`

// BatchExportInput holds the arguments for the batch_export tool.
type BatchExportInput struct {
	OutputDir   string  `json:"output_dir" jsonschema:"Directory to write exported files into"`
	Format      *string `json:"format" jsonschema:"\"png\", \"jpeg\", \"webp\", \"tiff\" (default \"png\")" enum:"png,jpeg,webp,tiff" project:"Chooses the file extension, from which GIMP picks the exporter."`
	Quality     *int    `json:"quality" jsonschema:"JPEG/WEBP quality (default 90)" minimum:"0" maximum:"100" sentinel:"0" gimp:"file-jpeg-export.quality/100 file-webp-export.quality"`
	NamePattern *string `json:"name_pattern" jsonschema:"Filename template — use {name} for image name, {index} for position"`
	ImageIndex  *int    `json:"image_index" jsonschema:"If set, export only that image; omit to export all open images"`
}

// SetDefaults applies the defaults documented for the batch_export tool.
func (in *BatchExportInput) SetDefaults() {
	if in.Format == nil {
		in.Format = ptr("png")
	}
	if in.Quality == nil {
		in.Quality = ptr(90)
	}
	if in.NamePattern == nil {
		in.NamePattern = ptr("{name}")
	}
}

// applyDropShadowDesc documents the apply_drop_shadow tool.
const applyDropShadowDesc = `Apply a drop shadow effect to a layer.

Parameters:
- offset_x, offset_y: Shadow offset in pixels (default 5, 5)
- blur_radius: Shadow softness radius (default 10)
- color: Shadow color (default "black")
- opacity: Shadow opacity 0-100 (default 60)
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict.`

// ApplyDropShadowInput holds the arguments for the apply_drop_shadow tool.
type ApplyDropShadowInput struct {
	OffsetX    *int     `json:"offset_x" jsonschema:"Shadow offset in pixels (default 5, 5)" gimp:"gegl:dropshadow.x"`
	OffsetY    *int     `json:"offset_y" jsonschema:"Shadow offset in pixels (default 5, 5)" gimp:"gegl:dropshadow.y"`
	BlurRadius *float64 `json:"blur_radius" jsonschema:"Shadow softness radius (default 10)" minimum:"0" gimp:"gegl:dropshadow.radius"`
	Color      *string  `json:"color" jsonschema:"Shadow color (default \"black\")"`
	Opacity    *float64 `json:"opacity" jsonschema:"Shadow opacity 0-100 (default 60)" minimum:"0" maximum:"100" gimp:"gegl:dropshadow.opacity/100"`
	LayerName  *string  `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int     `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int      `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the apply_drop_shadow tool.
func (in *ApplyDropShadowInput) SetDefaults() {
	if in.OffsetX == nil {
		in.OffsetX = ptr(5)
	}
	if in.OffsetY == nil {
		in.OffsetY = ptr(5)
	}
	if in.BlurRadius == nil {
		in.BlurRadius = ptr(10.0)
	}
	if in.Color == nil {
		in.Color = ptr("black")
	}
	if in.Opacity == nil {
		in.Opacity = ptr(60.0)
	}
}

// applyGeglFilterDesc documents the apply_gegl_filter tool.
const applyGeglFilterDesc = `Run any GEGL operation on a layer.

The dedicated effect tools cover the common operations; this reaches the rest of GEGL's catalogue, which call_api cannot, because GIMP 3 applies GEGL through libgimp rather than the PDB.

Only numeric properties can be set. Operations whose properties are colours or enums are not reachable this way, except where the operation samples a colour from the paint context.

Parameters:
- operation: GEGL operation name, e.g. "gegl:cartoon", "gegl:oilify", "gegl:posterize"
- settings: Numeric properties for the operation, e.g. {"mask-radius": 7.0, "pct-black": 0.2}
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Examples:
- apply_gegl_filter(operation="gegl:cartoon", settings={"mask-radius": 7.0, "pct-black": 0.2})
- apply_gegl_filter(operation="gegl:oilify", settings={"mask-radius": 4.0})

Returns status dict.`

// ApplyGeglFilterInput holds the arguments for the apply_gegl_filter tool.
type ApplyGeglFilterInput struct {
	Operation  string             `json:"operation" jsonschema:"GEGL operation name, e.g. \"gegl:cartoon\""`
	Settings   map[string]float64 `json:"settings" jsonschema:"Numeric properties for the operation, e.g. {\"mask-radius\": 7.0}"`
	LayerName  *string            `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int               `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int                `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// describeOperationDesc documents the describe_operation tool.
const describeOperationDesc = `Report what a GEGL operation will accept: every property, its type, its range, its default and its permitted values.

This is the only authoritative account of an operation. GIMP's published reference documents the PDB procedures rather than the filters, and GEGL's documents libgegl rather than the operation catalogue, so an operation's ranges and choices appear in neither. Because this asks the running GIMP, the answer describes the version actually installed - which is what makes it the thing to re-run after a GIMP upgrade.

It reports the properties of GIMP's filter configuration, which is what apply_gegl_filter and the effect tools set. That differs from the bare GEGL operation: GIMP re-declares an enum property as a named choice, which has to be set by name rather than as a number.

Parameters:
- operation: GEGL operation name, e.g. "gegl:vignette", "gimp:curves"
- layer_name: Layer to inspect against; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns: {operation, count, properties: [{name, type, minimum, maximum, default, choices, blurb}]}`

// DescribeOperationInput holds the arguments for the describe_operation tool.
type DescribeOperationInput struct {
	Operation  string  `json:"operation" jsonschema:"GEGL operation name, e.g. \"gegl:vignette\""`
	LayerName  *string `json:"layer_name" jsonschema:"Layer to inspect against; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// applyVignetteDesc documents the apply_vignette tool.
const applyVignetteDesc = `Apply a vignette darkening effect around the edges of a layer.

Parameters:
- radius: Size of the clear area, 0-3; larger fades less (default 1.5)
- softness: Width of the fade, 0-1 (default 0.8)
- shape: "circle" (default), "square", "diamond", "horizontal", "vertical"
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict.`

// ApplyVignetteInput holds the arguments for the apply_vignette tool.
type ApplyVignetteInput struct {
	Radius     *float64 `json:"radius" jsonschema:"Size of the clear area, 0-3; larger fades less (default 1.5)" minimum:"0" maximum:"3" gimp:"gegl:vignette.radius"`
	Softness   *float64 `json:"softness" jsonschema:"Width of the fade, 0-1 (default 0.8)" minimum:"0" maximum:"1" gimp:"gegl:vignette.softness"`
	Shape      *string  `json:"shape" jsonschema:"Vignette shape — \"circle\" (default), \"square\", \"diamond\", \"horizontal\", \"vertical\"" enum:"circle,square,diamond,horizontal,vertical" gimp:"gegl:vignette.shape"`
	LayerName  *string  `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int     `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int      `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the apply_vignette tool.
func (in *ApplyVignetteInput) SetDefaults() {
	if in.Radius == nil {
		in.Radius = ptr(1.5)
	}
	if in.Softness == nil {
		in.Softness = ptr(0.8)
	}
	if in.Shape == nil {
		in.Shape = ptr("circle")
	}
}

// exportIconSizesDesc documents the export_icon_sizes tool.
const exportIconSizesDesc = `Export an image as a complete icon set for Android or iOS.

Android sizes: 48 (mdpi), 72 (hdpi), 96 (xhdpi), 144 (xxhdpi),
               192 (xxxhdpi), 512 (Play Store)
iOS sizes: 20x1/2/3, 29x1/2/3, 40x2/3, 60x2/3, 76x1/2, 83.5x2, 1024x1

Parameters:
- output_dir: Directory to write icon files into
- platform: "android" (default) or "ios"
- source_image_index: Image to use as source (default 0)
- format: Output format — "png" (default)

Returns: {exported: [{size, file_path}], count, platform}`

// ExportIconSizesInput holds the arguments for the export_icon_sizes tool.
type ExportIconSizesInput struct {
	OutputDir        string  `json:"output_dir" jsonschema:"Directory to write icon files into"`
	Platform         *string `json:"platform" jsonschema:"\"android\" (default) or \"ios\"" enum:"android,ios" project:"Selects one of the plug-in's icon size lists."`
	SourceImageIndex int     `json:"source_image_index" jsonschema:"Image to use as source (default 0)"`
	Format           *string `json:"format" jsonschema:"Output format — \"png\" (default)"`
}

// SetDefaults applies the defaults documented for the export_icon_sizes tool.
func (in *ExportIconSizesInput) SetDefaults() {
	if in.Platform == nil {
		in.Platform = ptr("android")
	}
	if in.Format == nil {
		in.Format = ptr("png")
	}
}

// exportWebOptimizedDesc documents the export_web_optimized tool.
const exportWebOptimizedDesc = `Export an image as both JPEG and PNG, choosing the smaller format.

Parameters:
- output_dir: Directory to write output files
- jpeg_quality: JPEG quality 1-100 (default 85)
- png_compression: PNG compression level 0-9 (default 9)
- max_width / max_height: Optional scaling before export
- image_index: Source image index (default 0)

Returns: {jpeg_path, jpeg_size, png_path, png_size, recommendation}`

// ExportWebOptimizedInput holds the arguments for the export_web_optimized tool.
type ExportWebOptimizedInput struct {
	OutputDir      string `json:"output_dir" jsonschema:"Directory to write output files"`
	JPEGQuality    *int   `json:"jpeg_quality" jsonschema:"JPEG quality 1-100, or 0 to use the format default (default 85)" minimum:"0" maximum:"100" sentinel:"0" gimp:"file-jpeg-export.quality/100 file-webp-export.quality"`
	PNGCompression *int   `json:"png_compression" jsonschema:"PNG compression level 0-9, or -1 to use the format default (default 9)" minimum:"-1" maximum:"9" sentinel:"-1" gimp:"file-png-export.compression"`
	MaxWidth       *int   `json:"max_width" jsonschema:"Optional scaling before export"`
	MaxHeight      *int   `json:"max_height" jsonschema:"Optional scaling before export"`
	ImageIndex     int    `json:"image_index" jsonschema:"Source image index (default 0)"`
}

// SetDefaults applies the defaults documented for the export_web_optimized tool.
func (in *ExportWebOptimizedInput) SetDefaults() {
	if in.JPEGQuality == nil {
		in.JPEGQuality = ptr(85)
	}
	if in.PNGCompression == nil {
		in.PNGCompression = ptr(9)
	}
}

// batchResizeDesc documents the batch_resize tool.
const batchResizeDesc = `Resize all open images to a common target size.

Parameters:
- width / height: Target dimensions in pixels (provide one or both)
- scale_factor: Proportional scale (e.g. 0.5 = 50%); overrides width/height if set
- maintain_aspect: Preserve aspect ratio when only one dimension is given (default True)

Returns: {results: [{image_id, old_width, old_height, new_width, new_height}], count}`

// BatchResizeInput holds the arguments for the batch_resize tool.
type BatchResizeInput struct {
	Width          *int     `json:"width" jsonschema:"Target dimensions in pixels (provide one or both)"`
	Height         *int     `json:"height" jsonschema:"Target dimensions in pixels (provide one or both)"`
	ScaleFactor    *float64 `json:"scale_factor" jsonschema:"Proportional scale (e.g. 0.5 = 50%); overrides width/height if set"`
	MaintainAspect *bool    `json:"maintain_aspect" jsonschema:"Preserve aspect ratio when only one dimension is given (default True)"`
}

// SetDefaults applies the defaults documented for the batch_resize tool.
func (in *BatchResizeInput) SetDefaults() {
	if in.MaintainAspect == nil {
		in.MaintainAspect = ptr(true)
	}
}
