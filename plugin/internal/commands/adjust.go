package commands

import (
	"fmt"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
)

//nolint:gochecknoinits // the command table is assembled from several files
func init() {
	register("auto_levels", autoLevels)
	register("adjust_brightness_contrast", adjustBrightnessContrast)
	register("adjust_hue_saturation", adjustHueSaturation)
	register("adjust_color_balance", adjustColorBalance)
	register("adjust_curves", adjustCurves)
	register("desaturate", desaturate)
	register("invert_colors", invertColors)
	register("sharpen", sharpen)
	register("blur", blurImage)
	register("denoise", denoise)
	register("apply_gaussian_blur", applyGaussianBlur)
	register("apply_pixelate", applyPixelate)
	register("apply_emboss", applyEmboss)
	register("apply_noise", applyNoise)
	register("get_histogram", getHistogram)
}

// histogramChannels maps protocol channel names onto GimpHistogramChannel.
var histogramChannels = map[string]int{
	"value": 0,
	"red":   1,
	"green": 2,
	"blue":  3,
	"alpha": 4,
}

// runGeglFilter applies a GEGL operation to a drawable.
func runGeglFilter(drawable gimpbridge.ObjectID, operation string, settings map[string]float64) error {
	return gimpbridge.ApplyGEGL(drawable, operation, settings)
}

// autoLevels stretches the tonal range.
func autoLevels(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	if err := run("gimp-drawable-levels-stretch",
		gimpbridge.Args{"drawable": drawable}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// adjustBrightnessContrast shifts brightness and contrast.
//
// The PDB takes -0.5..0.5; the protocol uses -127..127 like the GIMP dialog.
func adjustBrightnessContrast(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	if err := run("gimp-drawable-brightness-contrast", gimpbridge.Args{
		"drawable":   drawable,
		"brightness": p.Float("brightness", 0) / 254.0,
		"contrast":   p.Float("contrast", 0) / 254.0,
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// hueRanges maps protocol colour range names onto GimpHueRange.
var hueRanges = map[string]int{
	"all": 0, "red": 1, "yellow": 2, "green": 3,
	"cyan": 4, "blue": 5, "magenta": 6,
}

// adjustHueSaturation shifts hue, saturation and lightness.
func adjustHueSaturation(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	hueRange, ok := hueRanges[p.String("color_range", "all")]
	if !ok {
		return nil, fmt.Errorf("unknown color_range %q", p.String("color_range", ""))
	}

	if err := run("gimp-drawable-hue-saturation", gimpbridge.Args{
		"drawable":   drawable,
		"hue-range":  hueRange,
		"hue-offset": p.Float("hue", 0),
		"lightness":  p.Float("lightness", 0),
		"saturation": p.Float("saturation", 0),
		"overlap":    0.0,
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// transferModes maps protocol tonal range names onto GimpTransferMode.
var transferModes = map[string]int{
	"shadows": 0, "midtones": 1, "highlights": 2,
}

// adjustColorBalance shifts the colour balance of a tonal range.
func adjustColorBalance(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	mode, ok := transferModes[p.String("range", "midtones")]
	if !ok {
		return nil, fmt.Errorf("range must be shadows, midtones or highlights")
	}

	if err := run("gimp-drawable-color-balance", gimpbridge.Args{
		"drawable":      drawable,
		"transfer-mode": mode,
		"preserve-lum":  p.Bool("preserve_luminosity", true),
		"cyan-red":      p.Float("cyan_red", 0),
		"magenta-green": p.Float("magenta_green", 0),
		"yellow-blue":   p.Float("yellow_blue", 0),
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// curvePresets are the built-in control point sets, as input/output pairs in
// the 0..1 range.
var curvePresets = map[string][]float64{
	"s_curve":  {0, 0, 0.25, 0.15, 0.75, 0.85, 1, 1},
	"contrast": {0, 0, 0.3, 0.2, 0.7, 0.8, 1, 1},
	"lighten":  {0, 0.1, 0.5, 0.65, 1, 1},
	"darken":   {0, 0, 0.5, 0.35, 1, 0.9},
}

// adjustCurves applies a tone curve.
func adjustCurves(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	channel, ok := histogramChannels[p.String("channel", "value")]
	if !ok {
		return nil, fmt.Errorf("unknown channel %q", p.String("channel", ""))
	}

	points := p.Floats("points")
	preset := p.String("preset", "s_curve")

	if len(points) == 0 {
		points, ok = curvePresets[preset]
		if !ok {
			return nil, fmt.Errorf("unknown preset %q", preset)
		}
	}

	if len(points) < 4 || len(points)%2 != 0 {
		return nil, fmt.Errorf("points must be x,y pairs with at least two points")
	}

	if err := run("gimp-drawable-curves-spline", gimpbridge.Args{
		"drawable": drawable,
		"channel":  channel,
		"points":   gimpbridge.Doubles(points),
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "preset": preset}, nil
}

// desaturateModes maps protocol mode names onto GimpDesaturateMode.
var desaturateModes = map[string]int{
	"lightness": 0, "luma": 1, "average": 2,
	"luminance": 3, "value": 4,
}

// desaturate removes colour.
func desaturate(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	mode, ok := desaturateModes[p.String("mode", "luminance")]
	if !ok {
		return nil, fmt.Errorf("unknown desaturate mode %q", p.String("mode", ""))
	}

	if err := run("gimp-drawable-desaturate",
		gimpbridge.Args{"drawable": drawable, "desaturate-mode": mode}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// invertColors inverts the drawable.
func invertColors(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	if err := run("gimp-drawable-invert",
		gimpbridge.Args{"drawable": drawable, "linear": false}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// sharpen applies an unsharp mask.
func sharpen(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	if err := runGeglFilter(drawable, "gegl:unsharp-mask", map[string]float64{
		"std-dev":   p.Float("radius", 3.0),
		"scale":     p.Float("amount", 50.0) / 50.0,
		"threshold": p.Float("threshold", 0) / 255.0,
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// blurImage applies a gaussian blur with independent axes.
func blurImage(p Params) (any, error) {
	x := p.Float("radius_x", 5.0)

	return gaussianBlur(p, x, p.Float("radius_y", x))
}

// applyGaussianBlur blurs with a single radius.
func applyGaussianBlur(p Params) (any, error) {
	radius := p.Float("radius", 5.0)

	return gaussianBlur(p, radius, radius)
}

// gaussianBlur blurs the target drawable.
func gaussianBlur(p Params, radiusX, radiusY float64) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	if err := runGeglFilter(drawable, "gegl:gaussian-blur", map[string]float64{
		"std-dev-x": radiusX,
		"std-dev-y": radiusY,
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "radius_x": radiusX, "radius_y": radiusY}, nil
}

// denoise smooths noise while keeping edges.
func denoise(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	if err := runGeglFilter(drawable, "gegl:noise-reduction", map[string]float64{
		"iterations": p.Float("strength", 4),
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// applyPixelate blocks the drawable into large pixels.
func applyPixelate(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	size := p.Float("block_size", 10)

	if err := runGeglFilter(drawable, "gegl:pixelize", map[string]float64{
		"size-x": size,
		"size-y": size,
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "block_size": size}, nil
}

// applyEmboss raises edges into relief.
func applyEmboss(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	if err := runGeglFilter(drawable, "gegl:emboss", map[string]float64{
		"azimuth":   p.Float("azimuth", 30),
		"elevation": p.Float("elevation", 45),
		"depth":     p.Float("depth", 20),
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// applyNoise scatters random noise over the drawable.
func applyNoise(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	amount := p.Float("amount", 0.2)

	if err := runGeglFilter(drawable, "gegl:noise-rgb", map[string]float64{
		"red": amount, "green": amount, "blue": amount,
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "amount": amount}, nil
}

// getHistogram reports channel statistics.
func getHistogram(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	channel, ok := histogramChannels[p.String("channel", "value")]
	if !ok {
		return nil, fmt.Errorf("unknown channel %q", p.String("channel", ""))
	}

	out, err := gimpbridge.Run("gimp-drawable-histogram", gimpbridge.Args{
		"drawable":    drawable,
		"channel":     channel,
		"start-range": 0.0,
		"end-range":   1.0,
	})
	if err != nil {
		return nil, err
	}

	keys := []string{"mean", "std_dev", "median", "pixels", "count", "percentile"}
	result := map[string]any{"channel": p.String("channel", "value")}

	for i, key := range keys {
		if i < len(out) {
			result[key] = out[i]
		}
	}

	return result, nil
}
