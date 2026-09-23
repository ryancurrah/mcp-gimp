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

// runGeglFilter applies a GEGL operation to a drawable.
func runGeglFilter(drawable gimpbridge.ObjectID, operation string, settings map[string]float64) error {
	return gimpbridge.ApplyGEGL(drawable, operation, settings)
}

// runGeglFilterWithChoices runs an operation that also takes a property named
// by one of a fixed set of choices, which GIMP 3 exposes as a string.
func runGeglFilterWithChoices(drawable gimpbridge.ObjectID, operation string,
	settings map[string]float64, choices map[string]string,
) error {
	return gimpbridge.ApplyGEGLWithChoices(drawable, operation, settings, choices)
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

// unitScale converts a protocol value measured on -full..full onto the
// -1..1 the gimp: filters take, clamped so an out-of-range value cannot be
// discarded and silently restore the filter's own default.
func unitScale(v, full float64) float64 {
	return min(max(v/full, -1), 1)
}

// adjustBrightnessContrast shifts brightness and contrast.
//
// The protocol uses -127..127 like the GIMP dialog; the filter takes -1..1.
func adjustBrightnessContrast(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	if err := runGeglFilter(drawable, "gimp:brightness-contrast", map[string]float64{
		"brightness": unitScale(p.Float("brightness", 0), 127),
		"contrast":   unitScale(p.Float("contrast", 0), 127),
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// adjustHueSaturation shifts hue, saturation and lightness.
func adjustHueSaturation(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	// The protocol keeps the GIMP dialog's units: hue in degrees, the other
	// two as percentages. The filter takes all three on -1..1.
	if err := runGeglFilterWithChoices(drawable, "gimp:hue-saturation", map[string]float64{
		"hue":        unitScale(p.Float("hue", 0), 180),
		"lightness":  unitScale(p.Float("lightness", 0), 100),
		"saturation": unitScale(p.Float("saturation", 0), 100),
		"overlap":    0,
	}, map[string]string{"range": p.String("color_range", "all")}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// adjustColorBalance shifts the colour balance of a tonal range.
func adjustColorBalance(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	preserve := 0.0
	if p.Bool("preserve_luminosity", true) {
		preserve = 1
	}

	if err := runGeglFilterWithChoices(drawable, "gimp:color-balance", map[string]float64{
		"preserve-luminosity": preserve,
		"cyan-red":            unitScale(p.Float("cyan_red", 0), 100),
		"magenta-green":       unitScale(p.Float("magenta_green", 0), 100),
		"yellow-blue":         unitScale(p.Float("yellow_blue", 0), 100),
	}, map[string]string{"range": p.String("range", "midtones")}); err != nil {
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

	points := p.Floats("points")
	preset := p.String("preset", "s_curve")

	if len(points) == 0 {
		var ok bool

		points, ok = curvePresets[preset]
		if !ok {
			return nil, fmt.Errorf("unknown preset %q", preset)
		}
	}

	if len(points) < 4 || len(points)%2 != 0 {
		return nil, fmt.Errorf("points must be x,y pairs with at least two points")
	}

	// gimp-drawable-curves-spline is deprecated in favour of the gimp:curves
	// filter, which is not reachable from here: its curve property holds a
	// GimpCurve object, and the filter bridge carries only numbers and the
	// names of choices. The procedure still works, and takes the control
	// points directly, so it stays until the bridge can build one.
	if err := run("gimp-drawable-curves-spline", gimpbridge.Args{
		"drawable": drawable,
		"channel":  p.String("channel", "value"),
		"points":   gimpbridge.Doubles(points),
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "preset": preset}, nil
}

// desaturate removes colour.
func desaturate(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	if err := runGeglFilterWithChoices(drawable, "gimp:desaturate", nil,
		map[string]string{"mode": p.String("mode", "luminance")}); err != nil {
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

	// The procedure inverted with linear: false, which is the gamma variant.
	if err := runGeglFilter(drawable, "gegl:invert-gamma", nil); err != nil {
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

	// The protocol takes a 0-100 strength; the operation counts iterations,
	// of which it accepts 0 to 32. Passing the strength straight through meant
	// anything above 32 was refused and the operation kept its own default of
	// 4, so the documented default of 50 did nothing at all.
	iterations := min(max(p.Float("strength", 50), 0), 100) * 32 / 100

	if err := runGeglFilter(drawable, "gegl:noise-reduction", map[string]float64{
		"iterations": float64(int(iterations + 0.5)),
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

	out, err := gimpbridge.Run("gimp-drawable-histogram", gimpbridge.Args{
		"drawable":    drawable,
		"channel":     p.String("channel", "value"),
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
