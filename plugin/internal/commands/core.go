package commands

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
)

//nolint:gochecknoinits // the command table is assembled from several files
func init() {
	register("get_gimp_info", getGimpInfo)
	register("list_images", listImages)
	register("get_image_metadata", getImageMetadata)
	register("new_canvas", newCanvas)
	register("open_image", openImage)
	register("list_layers", listLayers)
	register("call_api", callAPI)
	register("describe_procedure", describeProcedure)
}

// getGimpInfo reports the GIMP build and environment.
func getGimpInfo(_ Params) (any, error) {
	version, err := run1("gimp-version", nil)
	if err != nil {
		return nil, err
	}

	info := map[string]any{
		"version": map[string]any{
			"version_method": version,
		},
		"host": map[string]any{
			"os":   runtime.GOOS,
			"arch": runtime.GOARCH,
		},
		"plugin": map[string]any{
			"implementation": "libgimp (Go)",
		},
	}

	if dir, err := run1("gimp-directory", nil); err == nil {
		info["directories"] = map[string]any{"user": dir}
	}

	return info, nil
}

// listImages summarises every open image.
func listImages(_ Params) (any, error) {
	images, err := openImages()
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, len(images))

	for i, image := range images {
		width, height, err := imageSize(image)
		if err != nil {
			return nil, err
		}

		entry := map[string]any{
			"index":    i,
			"image_id": int(image),
			"width":    width,
			"height":   height,
		}

		if name, err := run1("gimp-image-get-name", gimpbridge.Args{"image": image}); err == nil {
			entry["name"] = name
		}

		if file, err := run1("gimp-image-get-file", gimpbridge.Args{"image": image}); err == nil {
			entry["file"] = file
		}

		out = append(out, entry)
	}

	return map[string]any{"images": out, "count": len(out)}, nil
}

// getImageMetadata describes the active image without exporting pixels.
func getImageMetadata(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	width, height, err := imageSize(image)
	if err != nil {
		return nil, err
	}

	layers, err := layersOf(image)
	if err != nil {
		return nil, err
	}

	described := make([]map[string]any, 0, len(layers))

	for _, layer := range layers {
		name, err := itemName(layer)
		if err != nil {
			return nil, err
		}

		entry := map[string]any{"layer_id": int(layer), "name": name}

		if v, err := run1("gimp-item-get-visible", gimpbridge.Args{"item": layer}); err == nil {
			entry["visible"] = v
		}

		if v, err := run1("gimp-layer-get-opacity", gimpbridge.Args{"layer": layer}); err == nil {
			entry["opacity"] = v
		}

		described = append(described, entry)
	}

	meta := map[string]any{
		"image_id":   int(image),
		"width":      width,
		"height":     height,
		"num_layers": len(layers),
		"layers":     described,
	}

	if base, err := run1("gimp-image-get-base-type", gimpbridge.Args{"image": image}); err == nil {
		meta["color_mode"] = baseTypeName(base)
	}

	return meta, nil
}

// baseTypeName renders GimpImageBaseType as the strings the protocol uses.
func baseTypeName(v gimpbridge.Value) string {
	n, _ := v.(int64)

	switch n {
	case 0:
		return "RGB"
	case 1:
		return "GRAY"
	case 2:
		return "INDEXED"
	default:
		return "UNKNOWN"
	}
}

// baseTypeFor maps a colour mode name onto GimpImageBaseType, reporting
// whether the mode implies an alpha channel.
func baseTypeFor(mode string) (base int, alpha bool, err error) {
	switch mode {
	case "RGB":
		return 0, false, nil
	case "RGBA":
		return 0, true, nil
	case "GRAY":
		return 1, false, nil
	case "GRAYA":
		return 1, true, nil
	case "INDEXED":
		return 2, false, nil
	default:
		return 0, false, fmt.Errorf("unsupported color_mode %q", mode)
	}
}

// newCanvas creates a blank image and shows it.
func newCanvas(p Params) (any, error) {
	width := p.Int("width", 1024)
	height := p.Int("height", 1024)
	mode := p.String("color_mode", "RGB")
	fill := p.String("fill", "white")
	name := p.String("name", "Untitled")

	base, alpha, err := baseTypeFor(mode)
	if err != nil {
		return nil, err
	}

	v, err := run1("gimp-image-new", gimpbridge.Args{
		"width": width, "height": height, "type": base,
	})
	if err != nil {
		return nil, err
	}

	image, err := objectID(v)
	if err != nil {
		return nil, err
	}

	if res := p.Int("resolution", 72); res > 0 {
		if err := run("gimp-image-set-resolution", gimpbridge.Args{
			"image": image, "xresolution": float64(res), "yresolution": float64(res),
		}); err != nil {
			return nil, err
		}
	}

	// Layer type follows the image base type, plus alpha when asked for or
	// when the fill is transparent.
	transparent := isTransparentFill(fill)
	layerType := base * 2

	if alpha || transparent {
		layerType++
	}

	lv, err := run1("gimp-layer-new", gimpbridge.Args{
		"image": image, "name": name, "width": width, "height": height,
		"type": layerType, "opacity": 100.0, "mode": layerModeNormal,
	})
	if err != nil {
		return nil, err
	}

	layer, err := objectID(lv)
	if err != nil {
		return nil, err
	}

	if err := run("gimp-image-insert-layer", gimpbridge.Args{
		"image": image, "layer": layer, "parent": gimpbridge.ObjectID(-1), "position": 0,
	}); err != nil {
		return nil, err
	}

	if err := fillDrawable(layer, fill, transparent); err != nil {
		return nil, err
	}

	displayOpened := true
	if _, err := gimpbridge.Run("gimp-display-new", gimpbridge.Args{"image": image}); err != nil {
		displayOpened = false
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{
		"image_id":       int(image),
		"width":          width,
		"height":         height,
		"color_mode":     mode,
		"display_opened": displayOpened,
	}, nil
}

// GimpFillType, as GIMP 3 numbers it.
//
// GIMP 3 inserted CIELAB_MIDDLE_GRAY at 2, which pushed white, transparent and
// pattern one place up from the values GIMP 2 used. Getting this wrong is
// silent rather than loud: asking for transparent under the old numbering
// paints opaque white, and the layer still reports an alpha channel. This
// server targets GIMP 3 only, so these are the only correct values.
const (
	fillForeground = iota
	fillBackground
	fillCIELabMiddleGray
	fillWhite
	fillTransparent
	fillPattern
)

// fillTypes maps the protocol's fill names onto GimpFillType.
var fillTypes = map[string]int{
	"foreground":  fillForeground,
	"background":  fillBackground,
	"white":       fillWhite,
	"transparent": fillTransparent,
	"none":        fillTransparent,
	"pattern":     fillPattern,
}

// fillTypeFor resolves a fill type name, defaulting to the foreground.
func fillTypeFor(name string) (int, error) {
	if name == "" {
		return fillForeground, nil
	}

	fill, ok := fillTypes[strings.ToLower(name)]
	if !ok {
		return 0, fmt.Errorf("unknown fill type %q", name)
	}

	return fill, nil
}

// isTransparentFill reports whether a fill name asks for transparency rather
// than a colour.
func isTransparentFill(fill string) bool {
	switch strings.ToLower(fill) {
	case "transparent", "none":
		return true
	default:
		return false
	}
}

// fillDrawable fills a drawable with a CSS colour, or clears it when the fill
// is transparent.
func fillDrawable(drawable gimpbridge.ObjectID, fill string, transparent bool) error {
	if transparent {
		return run("gimp-drawable-fill",
			gimpbridge.Args{"drawable": drawable, "fill-type": fillTransparent})
	}

	if err := run("gimp-context-set-foreground",
		gimpbridge.Args{"foreground": gimpbridge.Color(fill)}); err != nil {
		return err
	}

	return run("gimp-drawable-fill",
		gimpbridge.Args{"drawable": drawable, "fill-type": fillForeground})
}

// openImage loads a file and displays it.
func openImage(p Params) (any, error) {
	path := p.String("file_path", "")
	if path == "" {
		return nil, fmt.Errorf("file_path is required")
	}

	v, err := run1("gimp-file-load", gimpbridge.Args{"file": path, "run-mode": 1})
	if err != nil {
		return nil, err
	}

	image, err := objectID(v)
	if err != nil {
		return nil, err
	}

	width, height, err := imageSize(image)
	if err != nil {
		return nil, err
	}

	layers, err := layersOf(image)
	if err != nil {
		return nil, err
	}

	displayOpened := true
	if _, err := gimpbridge.Run("gimp-display-new", gimpbridge.Args{"image": image}); err != nil {
		displayOpened = false
	}

	if err := flush(); err != nil {
		return nil, err
	}

	result := map[string]any{
		"image_id":       int(image),
		"width":          width,
		"height":         height,
		"num_layers":     len(layers),
		"display_opened": displayOpened,
	}

	if base, err := run1("gimp-image-get-base-type", gimpbridge.Args{"image": image}); err == nil {
		result["color_mode"] = baseTypeName(base)
	}

	return result, nil
}

// listLayers describes an image's layer stack.
func listLayers(p Params) (any, error) {
	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	layers, err := layersOf(image)
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, len(layers))

	for i, layer := range layers {
		name, err := itemName(layer)
		if err != nil {
			return nil, err
		}

		entry := map[string]any{"index": i, "layer_id": int(layer), "name": name}

		for key, proc := range map[string]string{
			"visible": "gimp-item-get-visible",
			"opacity": "gimp-layer-get-opacity",
		} {
			arg := "item"
			if proc == "gimp-layer-get-opacity" {
				arg = "layer"
			}

			if v, err := run1(proc, gimpbridge.Args{arg: layer}); err == nil {
				entry[key] = v
			}
		}

		out = append(out, entry)
	}

	return map[string]any{"layers": out, "count": len(out)}, nil
}

// callAPI invokes a PDB procedure directly.
//
// This is the escape hatch for anything the dedicated commands do not cover.
// libgimp has no interpreter, so callers name a procedure and pass its
// arguments by name rather than sending source code.
func callAPI(p Params) (any, error) {
	proc := p.String("api_path", "")
	if proc == "" || proc == "exec" {
		return nil, fmt.Errorf(
			"api_path must name a PDB procedure, for example \"gimp-image-get-layers\"; " +
				"this plug-in runs procedures, not source code")
	}

	// Arguments come either as a kwargs object keyed by PDB argument name, or
	// as a positional args list matched against the procedure's signature.
	args := gimpbridge.Args{}

	if positional, ok := p.Any("args").([]any); ok && len(positional) > 0 {
		spec, err := gimpbridge.Describe(proc)
		if err != nil {
			return nil, err
		}

		if len(positional) > len(spec) {
			return nil, fmt.Errorf("%s takes %d arguments, got %d",
				proc, len(spec), len(positional))
		}

		for i, value := range positional {
			args[spec[i].Name] = normaliseArg(value)
		}
	}

	for key, value := range p.Object("kwargs") {
		var decoded any
		if err := jsonUnmarshal(value, &decoded); err != nil {
			return nil, fmt.Errorf("argument %q: %w", key, err)
		}

		args[key] = normaliseArg(decoded)
	}

	out, err := gimpbridge.Run(proc, args)
	if err != nil {
		return nil, err
	}

	return map[string]any{"procedure": proc, "values": renderValues(out)}, nil
}

// describeProcedure lists the arguments a PDB procedure takes, so callers can
// discover the exact argument names call_api expects.
func describeProcedure(p Params) (any, error) {
	proc := p.String("api_path", p.String("procedure", ""))
	if proc == "" {
		return nil, fmt.Errorf("procedure is required")
	}

	args, err := gimpbridge.Describe(proc)
	if err != nil {
		return nil, err
	}

	return map[string]any{"procedure": proc, "arguments": args}, nil
}

// normaliseArg converts a decoded JSON value into something the bridge binds.
func normaliseArg(v any) gimpbridge.Value {
	switch t := v.(type) {
	case float64:
		// JSON has one number type; integral values are bound as integers so
		// they can satisfy id and enum arguments.
		if t == float64(int64(t)) {
			return int64(t)
		}

		return t

	case []any:
		floats := make([]float64, 0, len(t))

		for _, item := range t {
			f, ok := item.(float64)
			if !ok {
				return v
			}

			floats = append(floats, f)
		}

		return gimpbridge.Doubles(floats)

	default:
		return v
	}
}

// renderValues converts PDB return values into JSON-friendly output.
func renderValues(values []gimpbridge.Value) []any {
	out := make([]any, 0, len(values))

	for _, v := range values {
		switch t := v.(type) {
		case gimpbridge.ObjectID:
			out = append(out, int(t))

		case []gimpbridge.ObjectID:
			ids := make([]int, len(t))
			for i, id := range t {
				ids[i] = int(id)
			}

			out = append(out, ids)

		default:
			out = append(out, v)
		}
	}

	return out
}
