// Tool definitions for colour adjustments and filters.
//
// Each tool is its description, its argument struct and, where arguments
// have non-zero defaults, SetDefaults. Argument constraints are struct tags;
// see schema.go for what each means and CONTRIBUTING.md for where they come
// from.

package server

// registerAdjustTools wires up the pass-through tools for colour adjustments and filters.
func registerAdjustTools(r *registrar) {
	addObject[AutoLevelsInput](r, toolDef{
		Name: "auto_levels", Command: "auto_levels",
		Required: nil, Description: autoLevelsDesc,
	})
	addObject[AdjustCurvesInput](r, toolDef{
		Name: "adjust_curves", Command: "adjust_curves",
		Required: nil, Description: adjustCurvesDesc,
	})
	addObject[AdjustBrightnessContrastInput](r, toolDef{
		Name: "adjust_brightness_contrast", Command: "adjust_brightness_contrast",
		Required: nil, Description: adjustBrightnessContrastDesc,
	})
	addObject[AdjustHueSaturationInput](r, toolDef{
		Name: "adjust_hue_saturation", Command: "adjust_hue_saturation",
		Required: nil, Description: adjustHueSaturationDesc,
	})
	addObject[AdjustColorBalanceInput](r, toolDef{
		Name: "adjust_color_balance", Command: "adjust_color_balance",
		Required: nil, Description: adjustColorBalanceDesc,
	})
	addObject[SharpenInput](r, toolDef{
		Name: "sharpen", Command: "sharpen",
		Required: nil, Description: sharpenDesc,
	})
	addObject[BlurInput](r, toolDef{
		Name: "blur", Command: "blur",
		Required: nil, Description: blurDesc,
	})
	addObject[DenoiseInput](r, toolDef{
		Name: "denoise", Command: "denoise",
		Required: nil, Description: denoiseDesc,
	})
	addObject[DesaturateInput](r, toolDef{
		Name: "desaturate", Command: "desaturate",
		Required: nil, Description: desaturateDesc,
	})
	addObject[InvertColorsInput](r, toolDef{
		Name: "invert_colors", Command: "invert_colors",
		Required: nil, Description: invertColorsDesc,
	})
	addObject[ApplyGaussianBlurInput](r, toolDef{
		Name: "apply_gaussian_blur", Command: "apply_gaussian_blur",
		Required: nil, Description: applyGaussianBlurDesc,
	})
	addObject[ApplyPixelateInput](r, toolDef{
		Name: "apply_pixelate", Command: "apply_pixelate",
		Required: nil, Description: applyPixelateDesc,
	})
	addObject[ApplyEmbossInput](r, toolDef{
		Name: "apply_emboss", Command: "apply_emboss",
		Required: nil, Description: applyEmbossDesc,
	})
	addObject[ApplyNoiseInput](r, toolDef{
		Name: "apply_noise", Command: "apply_noise",
		Required: nil, Description: applyNoiseDesc,
	})
	addObject[GetHistogramInput](r, toolDef{
		Name: "get_histogram", Command: "get_histogram",
		Required: nil, Description: getHistogramDesc,
	})
}

// autoLevelsDesc documents the auto_levels tool.
const autoLevelsDesc = `Automatically stretch the tonal range of an image (auto levels / auto stretch contrast).

Parameters:
- image_index: Index of the target image (default 0)
- layer_name: Name of the layer to adjust; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only

Returns status dict.`

// AutoLevelsInput holds the arguments for the auto_levels tool.
type AutoLevelsInput struct {
	ImageIndex int     `json:"image_index" jsonschema:"Index of the target image (default 0)"`
	LayerName  *string `json:"layer_name" jsonschema:"Name of the layer to adjust; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
}

// adjustCurvesDesc documents the adjust_curves tool.
const adjustCurvesDesc = `Adjust tonal curves for a layer.

Parameters:
- preset: Built-in curve shape — "s_curve" (default), "lighten", "darken", "contrast"
- points: Custom control points as [[input, output], ...] override (overrides preset)
- channel: "value" (all), "red", "green", "blue", "alpha"
- image_index: Target image index (default 0)
- layer_name: Layer to adjust; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only

Returns status dict.`

// AdjustCurvesInput holds the arguments for the adjust_curves tool.
type AdjustCurvesInput struct {
	Preset     *string `json:"preset" jsonschema:"Built-in curve shape — \"s_curve\" (default), \"lighten\", \"darken\", \"contrast\"" enum:"s_curve,lighten,darken,contrast" project:"Control-point sets defined by this plug-in."`
	Points     []any   `json:"points" jsonschema:"Custom control points as [[input, output], ...] override (overrides preset)"`
	Channel    *string `json:"channel" jsonschema:"\"value\" (all), \"red\", \"green\", \"blue\", \"alpha\"" enum:"value,red,green,blue,alpha" gimp:"gimp-drawable-curves-spline.channel"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
	LayerName  *string `json:"layer_name" jsonschema:"Layer to adjust; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
}

// SetDefaults applies the defaults documented for the adjust_curves tool.
func (in *AdjustCurvesInput) SetDefaults() {
	if in.Preset == nil {
		in.Preset = ptr("s_curve")
	}
	if in.Channel == nil {
		in.Channel = ptr("value")
	}
}

// adjustBrightnessContrastDesc documents the adjust_brightness_contrast tool.
const adjustBrightnessContrastDesc = `Adjust brightness and contrast of a layer.

Parameters:
- brightness: -127 to +127 (default 0)
- contrast: -127 to +127 (default 0)
- image_index: Target image index (default 0)
- layer_name: Layer to adjust; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only

Returns status dict.`

// AdjustBrightnessContrastInput holds the arguments for the adjust_brightness_contrast tool.
type AdjustBrightnessContrastInput struct {
	Brightness int     `json:"brightness" jsonschema:"-127 to +127 (default 0)" minimum:"-127" maximum:"127" gimp:"gimp:brightness-contrast.brightness/127"`
	Contrast   int     `json:"contrast" jsonschema:"-127 to +127 (default 0)" minimum:"-127" maximum:"127" gimp:"gimp:brightness-contrast.contrast/127"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
	LayerName  *string `json:"layer_name" jsonschema:"Layer to adjust; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
}

// adjustHueSaturationDesc documents the adjust_hue_saturation tool.
const adjustHueSaturationDesc = `Adjust hue, saturation, and lightness of a layer.

Parameters:
- hue: Hue rotation -180 to +180 (default 0)
- saturation: Saturation shift -100 to +100 (default 0)
- lightness: Lightness shift -100 to +100 (default 0)
- color_range: "all", "red", "yellow", "green", "cyan", "blue", "magenta" (default "all")
- image_index: Target image index (default 0)
- layer_name: Layer to adjust; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only

Returns status dict.`

// AdjustHueSaturationInput holds the arguments for the adjust_hue_saturation tool.
type AdjustHueSaturationInput struct {
	Hue        float64 `json:"hue" jsonschema:"Hue rotation -180 to +180 (default 0)" minimum:"-180" maximum:"180" gimp:"gimp:hue-saturation.hue/180"`
	Saturation float64 `json:"saturation" jsonschema:"Saturation shift -100 to +100 (default 0)" minimum:"-100" maximum:"100" gimp:"gimp:hue-saturation.saturation/100"`
	Lightness  float64 `json:"lightness" jsonschema:"Lightness shift -100 to +100 (default 0)" minimum:"-100" maximum:"100" gimp:"gimp:hue-saturation.lightness/100"`
	ColorRange *string `json:"color_range" jsonschema:"\"all\", \"red\", \"yellow\", \"green\", \"cyan\", \"blue\", \"magenta\" (default \"all\")" enum:"all,red,yellow,green,cyan,blue,magenta" gimp:"gimp:hue-saturation.range"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
	LayerName  *string `json:"layer_name" jsonschema:"Layer to adjust; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
}

// SetDefaults applies the defaults documented for the adjust_hue_saturation tool.
func (in *AdjustHueSaturationInput) SetDefaults() {
	if in.ColorRange == nil {
		in.ColorRange = ptr("all")
	}
}

// adjustColorBalanceDesc documents the adjust_color_balance tool.
const adjustColorBalanceDesc = `Adjust color balance (shadows / midtones / highlights) of a layer.

Parameters:
- cyan_red: -100 to +100 (negative = cyan, positive = red; default 0)
- magenta_green: -100 to +100 (default 0)
- yellow_blue: -100 to +100 (default 0)
- range: "shadows", "midtones" (default), "highlights"
- image_index: Target image index (default 0)
- layer_name: Layer to adjust; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only

Returns status dict.`

// AdjustColorBalanceInput holds the arguments for the adjust_color_balance tool.
type AdjustColorBalanceInput struct {
	CyanRed      float64 `json:"cyan_red" jsonschema:"-100 to +100 (negative = cyan, positive = red; default 0)" minimum:"-100" maximum:"100" gimp:"gimp:color-balance.cyan-red/100"`
	MagentaGreen float64 `json:"magenta_green" jsonschema:"-100 to +100 (default 0)" minimum:"-100" maximum:"100" gimp:"gimp:color-balance.magenta-green/100"`
	YellowBlue   float64 `json:"yellow_blue" jsonschema:"-100 to +100 (default 0)" minimum:"-100" maximum:"100" gimp:"gimp:color-balance.yellow-blue/100"`
	Range        *string `json:"range" jsonschema:"\"shadows\", \"midtones\" (default), \"highlights\"" enum:"shadows,midtones,highlights" gimp:"gimp:color-balance.range"`
	ImageIndex   int     `json:"image_index" jsonschema:"Target image index (default 0)"`
	LayerName    *string `json:"layer_name" jsonschema:"Layer to adjust; defaults to active layer"`
	LayerID      *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
}

// SetDefaults applies the defaults documented for the adjust_color_balance tool.
func (in *AdjustColorBalanceInput) SetDefaults() {
	if in.Range == nil {
		in.Range = ptr("midtones")
	}
}

// sharpenDesc documents the sharpen tool.
const sharpenDesc = `Sharpen a layer using unsharp mask.

Parameters:
- amount: Sharpening strength 0-500 (default 50.0)
- radius: Blur radius for the mask in pixels (default 3.0)
- threshold: Minimum difference before sharpening is applied (default 0)
- image_index: Target image index (default 0)
- layer_name: Layer to sharpen; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only

Returns status dict.`

// SharpenInput holds the arguments for the sharpen tool.
type SharpenInput struct {
	Amount     *float64 `json:"amount" jsonschema:"Sharpening strength 0-500 (default 50.0)" minimum:"0" maximum:"500" gimp:"gegl:unsharp-mask.scale/50"`
	Radius     *float64 `json:"radius" jsonschema:"Blur radius for the mask in pixels (default 3.0)" minimum:"0" maximum:"1500" gimp:"gegl:unsharp-mask.std-dev"`
	Threshold  int      `json:"threshold" jsonschema:"Minimum difference before sharpening is applied (default 0)" minimum:"0" maximum:"255" gimp:"gegl:unsharp-mask.threshold/255"`
	ImageIndex int      `json:"image_index" jsonschema:"Target image index (default 0)"`
	LayerName  *string  `json:"layer_name" jsonschema:"Layer to sharpen; defaults to active layer"`
	LayerID    *int     `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
}

// SetDefaults applies the defaults documented for the sharpen tool.
func (in *SharpenInput) SetDefaults() {
	if in.Amount == nil {
		in.Amount = ptr(50.0)
	}
	if in.Radius == nil {
		in.Radius = ptr(3.0)
	}
}

// blurDesc documents the blur tool.
const blurDesc = `Apply Gaussian blur to a layer.

Parameters:
- radius_x: Horizontal blur radius in pixels (default 5.0)
- radius_y: Vertical blur radius in pixels (default 5.0)
- image_index: Target image index (default 0)
- layer_name: Layer to blur; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only

Returns status dict.`

// BlurInput holds the arguments for the blur tool.
type BlurInput struct {
	RadiusX    *float64 `json:"radius_x" jsonschema:"Horizontal blur radius in pixels (default 5.0)" minimum:"0" maximum:"1500" gimp:"gegl:gaussian-blur.std-dev-x"`
	RadiusY    *float64 `json:"radius_y" jsonschema:"Vertical blur radius in pixels (default 5.0)" minimum:"0" maximum:"1500" gimp:"gegl:gaussian-blur.std-dev-y"`
	ImageIndex int      `json:"image_index" jsonschema:"Target image index (default 0)"`
	LayerName  *string  `json:"layer_name" jsonschema:"Layer to blur; defaults to active layer"`
	LayerID    *int     `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
}

// SetDefaults applies the defaults documented for the blur tool.
func (in *BlurInput) SetDefaults() {
	if in.RadiusX == nil {
		in.RadiusX = ptr(5.0)
	}
	if in.RadiusY == nil {
		in.RadiusY = ptr(5.0)
	}
}

// denoiseDesc documents the denoise tool.
const denoiseDesc = `Reduce noise in a layer using GEGL noise-reduction.

Parameters:
- strength: Noise reduction strength 0-100 (default 50)
- image_index: Target image index (default 0)
- layer_name: Layer to denoise; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only

Returns status dict.`

// DenoiseInput holds the arguments for the denoise tool.
type DenoiseInput struct {
	Strength   *int    `json:"strength" jsonschema:"Noise reduction strength 0-100 (default 50)" minimum:"0" maximum:"100" gimp:"gegl:noise-reduction.iterations/3.125"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
	LayerName  *string `json:"layer_name" jsonschema:"Layer to denoise; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
}

// SetDefaults applies the defaults documented for the denoise tool.
func (in *DenoiseInput) SetDefaults() {
	if in.Strength == nil {
		in.Strength = ptr(50)
	}
}

// desaturateDesc documents the desaturate tool.
const desaturateDesc = `Convert a layer to grayscale (desaturate).

Parameters:
- mode: Desaturation algorithm — "luminosity" (default), "luma", "average", "lightness"
- image_index: Target image index (default 0)
- layer_name: Layer to desaturate; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only

Returns status dict.`

// DesaturateInput holds the arguments for the desaturate tool.
type DesaturateInput struct {
	Mode       *string `json:"mode" jsonschema:"Desaturation algorithm: \"luminance\" (default), \"luma\", \"lightness\", \"average\", \"value\"" enum:"luminance,luma,lightness,average,value" gimp:"gimp:desaturate.mode"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
	LayerName  *string `json:"layer_name" jsonschema:"Layer to desaturate; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
}

// SetDefaults applies the defaults documented for the desaturate tool.
func (in *DesaturateInput) SetDefaults() {
	if in.Mode == nil {
		in.Mode = ptr("luminance")
	}
}

// invertColorsDesc documents the invert_colors tool.
const invertColorsDesc = `Invert all colors in a layer (create a negative).

Parameters:
- image_index: Target image index (default 0)
- layer_name: Layer to invert; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only

Returns status dict.`

// InvertColorsInput holds the arguments for the invert_colors tool.
type InvertColorsInput struct {
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
	LayerName  *string `json:"layer_name" jsonschema:"Layer to invert; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
}

// applyGaussianBlurDesc documents the apply_gaussian_blur tool.
const applyGaussianBlurDesc = `Apply Gaussian blur as a destructive filter operation.

Parameters:
- radius: Blur radius in pixels (default 5.0)
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict.`

// ApplyGaussianBlurInput holds the arguments for the apply_gaussian_blur tool.
type ApplyGaussianBlurInput struct {
	Radius     *float64 `json:"radius" jsonschema:"Blur radius in pixels (default 5.0)" minimum:"0" maximum:"1500" gimp:"gegl:gaussian-blur.std-dev-x gegl:gaussian-blur.std-dev-y"`
	LayerName  *string  `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int     `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int      `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the apply_gaussian_blur tool.
func (in *ApplyGaussianBlurInput) SetDefaults() {
	if in.Radius == nil {
		in.Radius = ptr(5.0)
	}
}

// applyPixelateDesc documents the apply_pixelate tool.
const applyPixelateDesc = `Pixelate a layer using a mosaic/block effect.

Parameters:
- block_size: Size of each mosaic block in pixels (default 10)
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict.`

// ApplyPixelateInput holds the arguments for the apply_pixelate tool.
type ApplyPixelateInput struct {
	BlockSize  *int    `json:"block_size" jsonschema:"Size of each mosaic block in pixels (default 10)" minimum:"1" gimp:"gegl:pixelize.size-x gegl:pixelize.size-y"`
	LayerName  *string `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int    `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the apply_pixelate tool.
func (in *ApplyPixelateInput) SetDefaults() {
	if in.BlockSize == nil {
		in.BlockSize = ptr(10)
	}
}

// applyEmbossDesc documents the apply_emboss tool.
const applyEmbossDesc = `Apply an emboss (bas-relief) effect to a layer.

Parameters:
- azimuth: Light direction in degrees 0-360 (default 315 = top-left)
- elevation: Light elevation angle 0-90 (default 45)
- depth: Effect depth/intensity (default 2)
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict.`

// ApplyEmbossInput holds the arguments for the apply_emboss tool.
type ApplyEmbossInput struct {
	Azimuth    *float64 `json:"azimuth" jsonschema:"Light direction in degrees 0-360 (default 315 = top-left)" minimum:"0" maximum:"360" gimp:"gegl:emboss.azimuth"`
	Elevation  *float64 `json:"elevation" jsonschema:"Light elevation angle 0-180 (default 45)" minimum:"0" maximum:"180" gimp:"gegl:emboss.elevation"`
	Depth      *float64 `json:"depth" jsonschema:"Effect depth 1-100 (default 2)" minimum:"1" maximum:"100" gimp:"gegl:emboss.depth"`
	LayerName  *string  `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int     `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int      `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the apply_emboss tool.
func (in *ApplyEmbossInput) SetDefaults() {
	if in.Azimuth == nil {
		in.Azimuth = ptr(315.0)
	}
	if in.Elevation == nil {
		in.Elevation = ptr(45.0)
	}
	if in.Depth == nil {
		in.Depth = ptr(2.0)
	}
}

// applyNoiseDesc documents the apply_noise tool.
const applyNoiseDesc = `Add noise/grain to a layer.

Parameters:
- amount: Noise intensity 0.0-1.0 (default 0.2)
- layer_name: Target layer; defaults to active layer
- layer_id: The layer_id another tool returned; unlike a name it survives renames. Identify the layer one way only
- image_index: Target image index (default 0)

Returns status dict.`

// ApplyNoiseInput holds the arguments for the apply_noise tool.
type ApplyNoiseInput struct {
	Amount     *float64 `json:"amount" jsonschema:"Noise intensity 0.0-1.0 (default 0.2)" minimum:"0" maximum:"1" gimp:"gegl:noise-rgb.red gegl:noise-rgb.green gegl:noise-rgb.blue"`
	LayerName  *string  `json:"layer_name" jsonschema:"Target layer; defaults to active layer"`
	LayerID    *int     `json:"layer_id" jsonschema:"Identify the layer by the layer_id another tool returned; it names the image too, so image_index is not consulted"`
	ImageIndex int      `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the apply_noise tool.
func (in *ApplyNoiseInput) SetDefaults() {
	if in.Amount == nil {
		in.Amount = ptr(0.2)
	}
}

// getHistogramDesc documents the get_histogram tool.
const getHistogramDesc = `Get histogram statistics for a channel of the active layer.

Parameters:
- channel: "value" (all; default), "red", "green", "blue", "alpha"
- image_index: Target image index (default 0)

Returns: {mean, median, std_dev, min, max, pixels, count}`

// GetHistogramInput holds the arguments for the get_histogram tool.
type GetHistogramInput struct {
	Channel    *string `json:"channel" jsonschema:"\"value\" (all; default), \"red\", \"green\", \"blue\", \"alpha\"" enum:"value,red,green,blue,alpha" gimp:"gimp-drawable-histogram.channel"`
	ImageIndex int     `json:"image_index" jsonschema:"Target image index (default 0)"`
}

// SetDefaults applies the defaults documented for the get_histogram tool.
func (in *GetHistogramInput) SetDefaults() {
	if in.Channel == nil {
		in.Channel = ptr("value")
	}
}
