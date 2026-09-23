package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
)

//nolint:gochecknoinits // the command table is assembled from several files
func init() {
	register("apply_drop_shadow", applyDropShadow)
	register("apply_gegl_filter", applyGeglFilter)
	register("apply_vignette", applyVignette)
	register("batch_export", batchExport)
	register("batch_resize", batchResize)
	register("export_icon_sizes", exportIconSizes)
	register("export_web_optimized", exportWebOptimized)
	register("describe_operation", describeOperation)
}

// describeOperation reports what a GEGL operation will accept.
//
// This is the only authoritative account of an operation's properties, their
// ranges and their permitted values: the libgimp reference documents the
// procedures rather than the filters, and GEGL's reference documents libgegl
// rather than the operation catalogue. Because it asks the running GIMP, the
// answer is for the version actually installed, and is how the tool
// definitions are checked when GIMP is upgraded.
func describeOperation(p Params) (any, error) {
	operation := p.String("operation", "")
	if operation == "" {
		return nil, fmt.Errorf("operation is required")
	}

	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	props, err := gimpbridge.DescribeOp(drawable, operation)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"operation": operation, "properties": props, "count": len(props),
	}, nil
}

// applyGeglFilter runs an arbitrary GEGL operation on a layer.
//
// The dedicated effect commands cover the operations worth naming; this is the
// escape hatch for the rest of GEGL's catalogue, which call_api cannot reach
// because GIMP 3 applies GEGL through libgimp rather than the PDB. Only
// numeric properties can be set, which is what the filter bridge carries.
func applyGeglFilter(p Params) (any, error) {
	operation := p.String("operation", "")
	if operation == "" {
		return nil, fmt.Errorf("operation is required")
	}

	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	props := p.Object("settings")
	settings := make(map[string]float64, len(props))

	for name := range props {
		settings[name] = props.Float(name, 0)
	}

	if err := runGeglFilter(drawable, operation, settings); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{
		"status": "success", "operation": operation, "settings": settings,
	}, nil
}

// shadowOpacity converts a 0-100 opacity to the 0-1 range gegl:dropshadow
// takes, matching how the other opacity arguments are spelled.
//
// The conversion is not cosmetic. GEGL declares the property with a maximum,
// and g_object_set discards a value above it rather than clamping, leaving
// GEGL's own 0.5 default in place. Passing the protocol's 0-100 through
// unchanged therefore made every value above the maximum behave identically:
// the documented default of 60 rendered exactly like 0.5, and no value above
// 1 could ever be reached.
func shadowOpacity(v float64) float64 {
	return min(max(v/100, 0), 1)
}

// applyDropShadow casts a shadow behind the layer's content.
func applyDropShadow(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	// The shadow colour is a GEGL colour property rather than a number, so
	// it is set on the paint context the operation samples from.
	if color := p.String("color", ""); color != "" {
		if err := run("gimp-context-set-foreground",
			gimpbridge.Args{"foreground": gimpbridge.Color(color)}); err != nil {
			return nil, err
		}
	}

	if err := runGeglFilter(drawable, "gegl:dropshadow", map[string]float64{
		"x":       p.Float("offset_x", 8),
		"y":       p.Float("offset_y", 8),
		"radius":  p.Float("blur_radius", 10),
		"opacity": shadowOpacity(p.Float("opacity", 60)),
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// vignetteShapes are the shapes gegl:vignette accepts, spelled as the
// operation spells them.
var vignetteShapes = []string{"circle", "square", "diamond", "horizontal", "vertical"} //nolint:gochecknoglobals // fixed table

// applyVignette darkens the edges of the frame.
func applyVignette(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	shape := p.String("shape", "circle")
	if !slices.Contains(vignetteShapes, shape) {
		return nil, fmt.Errorf("unknown vignette shape %q, want one of %s",
			shape, strings.Join(vignetteShapes, ", "))
	}

	// softness is bounded at 1 by the operation, so it is clamped rather than
	// refused. radius has no upper bound at all - the 0-3 the tool documents
	// is a useful range, not the operation's - so only the floor is enforced.
	// Both were read from the operation with describe_operation.
	if err := runGeglFilterWithChoices(drawable, "gegl:vignette",
		map[string]float64{
			"radius":   max(p.Float("radius", 1.5), 0),
			"softness": min(max(p.Float("softness", 0.8), 0), 1),
		},
		// shape is a GimpChoice on GIMP 3's filter config, so it is set by
		// name; as a number it never reached the operation at all.
		map[string]string{"shape": shape},
	); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success"}, nil
}

// batchExport writes every open image, or one of them, into a directory.
func batchExport(p Params) (any, error) {
	dir := p.String("output_dir", "")
	if dir == "" {
		return nil, fmt.Errorf("output_dir is required")
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create output_dir: %w", err)
	}

	images, err := openImages()
	if err != nil {
		return nil, err
	}

	if p.Has("image_index") {
		image, err := imageAt(p.Int("image_index", 0))
		if err != nil {
			return nil, err
		}

		images = []gimpbridge.ObjectID{image}
	}

	format := p.String("format", "png")
	pattern := p.String("name_pattern", "{name}")

	exported := make([]map[string]any, 0, len(images))
	failures := make([]string, 0)

	for i, image := range images {
		name := fmt.Sprintf("image_%d", i)

		if v, err := run1("gimp-image-get-name", gimpbridge.Args{"image": image}); err == nil {
			if s, ok := v.(string); ok && s != "" {
				name = strings.TrimSuffix(s, filepath.Ext(s))
			}
		}

		base := strings.NewReplacer(
			"{name}", name,
			"{index}", fmt.Sprint(i),
		).Replace(pattern)

		path := filepath.Join(dir, base+"."+format)

		width, height, err := imageSize(image)
		if err != nil {
			failures = append(failures, err.Error())

			continue
		}

		if err := exportFlattened(image, path, exportSettings{
			Quality: p.Float("quality", 0), PNGCompression: -1,
		}); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", path, err))

			continue
		}

		exported = append(exported, map[string]any{
			"file_path": path, "name": name, "width": width, "height": height,
		})
	}

	return map[string]any{
		"exported": exported, "count": len(exported), "errors": failures,
	}, nil
}

// exportFlattened writes a flattened copy of an image to path.
func exportFlattened(image gimpbridge.ObjectID, path string, s exportSettings) error {
	dup, err := duplicateImage(image)
	if err != nil {
		return err
	}
	defer deleteImage(dup)

	if err := run("gimp-image-flatten", gimpbridge.Args{"image": dup}); err != nil {
		return err
	}

	return saveImageWith(dup, path, s)
}

// batchResize writes the image at several sizes.
func batchResize(p Params) (any, error) {
	dir := p.String("output_dir", "")
	if dir == "" {
		return nil, fmt.Errorf("output_dir is required")
	}

	source, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	srcWidth, srcHeight, err := imageSize(source)
	if err != nil {
		return nil, err
	}

	// A target is given as a list of widths, an explicit width and/or
	// height, or a scale factor.
	targets := make([][2]int, 0, 4)

	for _, s := range p.Floats("sizes") {
		w := int(s)
		targets = append(targets, [2]int{w, scaleOther(w, srcWidth, srcHeight)})
	}

	width, height := p.Int("width", 0), p.Int("height", 0)

	if width > 0 || height > 0 {
		targets = append(targets, fitTarget(width, height,
			srcWidth, srcHeight, p.Bool("maintain_aspect", true)))
	}

	if factor := p.Float("scale_factor", 0); factor > 0 {
		targets = append(targets, [2]int{
			max(int(float64(srcWidth)*factor), 1),
			max(int(float64(srcHeight)*factor), 1),
		})
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("one of sizes, width, height or scale_factor is required")
	}

	return exportTargets(p, source, dir, targets,
		p.String("format", "png"), "{name}_{width}x{height}")
}

// exportIconSizes writes square icons at the conventional sizes.
func exportIconSizes(p Params) (any, error) {
	dir := p.String("output_dir", "")
	if dir == "" {
		return nil, fmt.Errorf("output_dir is required")
	}

	widths := iconSizes(p.String("platform", ""))

	if sizes := p.Floats("sizes"); len(sizes) > 0 {
		widths = make([]int, len(sizes))
		for i, s := range sizes {
			widths[i] = int(s)
		}
	}

	return exportSizes(p, dir, widths, p.String("format", "png"), "icon_{size}x{size}")
}

// fitTarget resolves the requested width and height into a concrete size.
//
// With maintain_aspect a missing dimension is derived from the other, and
// supplying both fits the image inside that box rather than distorting it.
func fitTarget(width, height, srcWidth, srcHeight int, maintainAspect bool) [2]int {
	switch {
	case width > 0 && height <= 0:
		if !maintainAspect {
			return [2]int{width, srcHeight}
		}

		return [2]int{width, scaleOther(width, srcWidth, srcHeight)}

	case height > 0 && width <= 0:
		if !maintainAspect {
			return [2]int{srcWidth, height}
		}

		return [2]int{scaleOther(height, srcHeight, srcWidth), height}

	default:
		if !maintainAspect {
			return [2]int{width, height}
		}

		// Fit inside the box, keeping the original proportions.
		if srcWidth*height > srcHeight*width {
			return [2]int{width, scaleOther(width, srcWidth, srcHeight)}
		}

		return [2]int{scaleOther(height, srcHeight, srcWidth), height}
	}
}

// exportTargets renders the image at each explicit size.
func exportTargets(p Params, source gimpbridge.ObjectID, dir string,
	targets [][2]int, format, pattern string,
) (any, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create output_dir: %w", err)
	}

	name := p.String("base_name", "image")

	exported := make([]map[string]any, 0, len(targets))
	failures := make([]string, 0)

	for _, t := range targets {
		width, height := t[0], t[1]

		if width <= 0 || height <= 0 {
			continue
		}

		base := strings.NewReplacer(
			"{name}", name,
			"{width}", fmt.Sprint(width),
			"{height}", fmt.Sprint(height),
			"{size}", fmt.Sprint(width),
		).Replace(pattern)

		path := filepath.Join(dir, base+"."+format)

		if err := exportResized(source, path, width, height); err != nil {
			failures = append(failures, fmt.Sprintf("%dx%d: %v", width, height, err))

			continue
		}

		exported = append(exported, map[string]any{
			"file_path": path, "width": width, "height": height,
		})
	}

	return map[string]any{
		"exported": exported, "count": len(exported), "errors": failures,
	}, nil
}

// exportResized writes one copy scaled to an exact size.
func exportResized(source gimpbridge.ObjectID, path string, width, height int) error {
	dup, err := duplicateImage(source)
	if err != nil {
		return err
	}
	defer deleteImage(dup)

	if err := run("gimp-image-flatten", gimpbridge.Args{"image": dup}); err != nil {
		return err
	}

	if err := run("gimp-image-scale", gimpbridge.Args{
		"image": dup, "new-width": width, "new-height": height,
	}); err != nil {
		return err
	}

	return saveImage(dup, path)
}

// exportSizes renders the image at each requested width.
func exportSizes(p Params, dir string, widths []int, format, pattern string) (any, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create output_dir: %w", err)
	}

	source, err := imageAt(p.Int("source_image_index", p.Int("image_index", 0)))
	if err != nil {
		return nil, err
	}

	name := p.String("base_name", "image")

	exported := make([]map[string]any, 0, len(widths))
	failures := make([]string, 0)

	for _, width := range widths {
		if width <= 0 {
			continue
		}

		path, err := exportScaled(source, dir, name, format, pattern, width)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%d: %v", width, err))

			continue
		}

		exported = append(exported, map[string]any{"file_path": path, "size": width})
	}

	return map[string]any{
		"exported": exported, "count": len(exported), "errors": failures,
	}, nil
}

// exportScaled writes one scaled copy and returns its path.
func exportScaled(source gimpbridge.ObjectID, dir, name, format, pattern string, width int) (string, error) {
	dup, err := duplicateImage(source)
	if err != nil {
		return "", err
	}
	defer deleteImage(dup)

	if err := run("gimp-image-flatten", gimpbridge.Args{"image": dup}); err != nil {
		return "", err
	}

	srcWidth, srcHeight, err := imageSize(dup)
	if err != nil {
		return "", err
	}

	height := scaleOther(width, srcWidth, srcHeight)

	if err := run("gimp-image-scale", gimpbridge.Args{
		"image": dup, "new-width": width, "new-height": height,
	}); err != nil {
		return "", err
	}

	base := strings.NewReplacer(
		"{name}", name,
		"{size}", fmt.Sprint(width),
	).Replace(pattern)

	path := filepath.Join(dir, base+"."+format)

	if err := saveImage(dup, path); err != nil {
		return "", err
	}

	return path, nil
}

// iconSizes returns the icon sizes a platform conventionally needs.
func iconSizes(platform string) []int {
	switch platform {
	case "ios":
		return []int{20, 29, 40, 58, 60, 76, 80, 87, 120, 152, 167, 180, 1024}
	case "android":
		return []int{36, 48, 72, 96, 144, 192, 512}
	case "windows":
		return []int{16, 24, 32, 48, 64, 128, 256}
	case "macos":
		return []int{16, 32, 64, 128, 256, 512, 1024}
	case "favicon", "web":
		return []int{16, 32, 48, 180, 192, 512}
	default:
		return []int{16, 32, 48, 64, 128, 256, 512}
	}
}

// exportWebOptimized writes a flattened, size-capped copy for the web.
func exportWebOptimized(p Params) (any, error) {
	path := p.String("file_path", "")

	// The tool may name a directory instead of a full path.
	if path == "" {
		if dir := p.String("output_dir", ""); dir != "" {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return nil, fmt.Errorf("create output_dir: %w", err)
			}

			path = filepath.Join(dir, "web-optimized.png")
		}
	}

	if path == "" {
		return nil, fmt.Errorf("file_path or output_dir is required")
	}

	source, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	dup, err := duplicateImage(source)
	if err != nil {
		return nil, err
	}
	defer deleteImage(dup)

	if err := run("gimp-image-flatten", gimpbridge.Args{"image": dup}); err != nil {
		return nil, err
	}

	if err := scaleWithin(dup, p.Int("max_width", 0), p.Int("max_height", 0)); err != nil {
		return nil, err
	}

	if err := saveImageWith(dup, path, exportSettings{
		Quality:        p.Float("jpeg_quality", 0),
		PNGCompression: p.Int("png_compression", -1),
		StripMetadata:  p.Bool("strip_metadata", true),
	}); err != nil {
		return nil, err
	}

	width, height, err := imageSize(dup)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("exported file is missing: %w", err)
	}

	return map[string]any{
		"status": "success", "file_path": path,
		"width": width, "height": height, "file_size_bytes": info.Size(),
	}, nil
}
