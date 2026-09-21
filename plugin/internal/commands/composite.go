package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
)

//nolint:gochecknoinits // the command table is assembled from several files
func init() {
	register("export_sprite_sheet", exportSpriteSheet)
	register("export_social_media_kit", exportSocialMediaKit)
	register("warp_region", warpRegion)
}

// exportSpriteSheet tiles every layer of an image into one sheet.
func exportSpriteSheet(p Params) (any, error) {
	path := p.String("output_path", p.String("file_path", ""))
	if path == "" {
		return nil, fmt.Errorf("output_path is required")
	}

	source, err := imageAt(p.Int("source", p.Int("image_index", 0)))
	if err != nil {
		return nil, err
	}

	layers, err := layersOf(source)
	if err != nil {
		return nil, err
	}

	if len(layers) == 0 {
		return nil, fmt.Errorf("image has no layers to tile")
	}

	frameWidth, frameHeight, err := imageSize(source)
	if err != nil {
		return nil, err
	}

	// Padding surrounds each frame so neighbouring sprites do not bleed into
	// one another when the sheet is sampled.
	padding := max(p.Int("padding", 0), 0)
	cellWidth := frameWidth + padding
	cellHeight := frameHeight + padding

	// Lay the frames out left to right, wrapping at the requested column
	// count. Layers are listed top to bottom, so walk them in reverse to get
	// the stacking order a reader expects.
	columns := p.Int("columns", len(layers))
	if columns <= 0 {
		columns = len(layers)
	}

	rows := (len(layers) + columns - 1) / columns

	sheet, err := newSheet(columns*cellWidth+padding, rows*cellHeight+padding)
	if err != nil {
		return nil, err
	}
	defer deleteImage(sheet)

	for i := range layers {
		layer := layers[len(layers)-1-i]

		copied, err := run1("gimp-layer-new-from-drawable",
			gimpbridge.Args{"drawable": layer, "dest-image": sheet})
		if err != nil {
			return nil, err
		}

		placed, err := objectID(copied)
		if err != nil {
			return nil, err
		}

		if err := run("gimp-image-insert-layer", gimpbridge.Args{
			"image": sheet, "layer": placed,
			"parent": gimpbridge.ObjectID(-1), "position": 0,
		}); err != nil {
			return nil, err
		}

		if err := run("gimp-layer-set-offsets", gimpbridge.Args{
			"layer": placed,
			"offx":  padding + (i%columns)*cellWidth,
			"offy":  padding + (i/columns)*cellHeight,
		}); err != nil {
			return nil, err
		}
	}

	if err := run("gimp-image-flatten", gimpbridge.Args{"image": sheet}); err != nil {
		return nil, err
	}

	if err := saveImage(sheet, path); err != nil {
		return nil, err
	}

	return map[string]any{
		"status": "success", "file_path": path,
		"frames": len(layers), "columns": columns, "rows": rows,
		"frame_width": frameWidth, "frame_height": frameHeight,
		"padding": padding,
	}, nil
}

// newSheet creates a transparent RGBA canvas to composite onto.
func newSheet(width, height int) (gimpbridge.ObjectID, error) {
	v, err := run1("gimp-image-new",
		gimpbridge.Args{"width": width, "height": height, "type": 0})
	if err != nil {
		return 0, err
	}

	image, err := objectID(v)
	if err != nil {
		return 0, err
	}

	lv, err := run1("gimp-layer-new", gimpbridge.Args{
		"image": image, "name": "Background", "width": width, "height": height,
		"type": 1, "opacity": 100.0, "mode": 28,
	})
	if err != nil {
		return 0, err
	}

	layer, err := objectID(lv)
	if err != nil {
		return 0, err
	}

	if err := run("gimp-image-insert-layer", gimpbridge.Args{
		"image": image, "layer": layer,
		"parent": gimpbridge.ObjectID(-1), "position": 0,
	}); err != nil {
		return 0, err
	}

	if err := run("gimp-drawable-fill",
		gimpbridge.Args{"drawable": layer, "fill-type": 3}); err != nil {
		return 0, err
	}

	return image, nil
}

// socialPreset is one platform's output size.
type socialPreset struct {
	name          string
	width, height int
}

// socialPresets are the sizes export_social_media_kit knows about.
var socialPresets = map[string]socialPreset{
	"instagram_square":  {"instagram_square", 1080, 1080},
	"instagram_story":   {"instagram_story", 1080, 1920},
	"twitter_post":      {"twitter_post", 1600, 900},
	"twitter_header":    {"twitter_header", 1500, 500},
	"facebook_post":     {"facebook_post", 1200, 630},
	"facebook_cover":    {"facebook_cover", 820, 312},
	"linkedin_post":     {"linkedin_post", 1200, 627},
	"linkedin_banner":   {"linkedin_banner", 1584, 396},
	"youtube_thumbnail": {"youtube_thumbnail", 1280, 720},
	"open_graph":        {"open_graph", 1200, 630},
}

// exportSocialMediaKit writes the image at each requested platform size.
//
// Each output is cropped to the platform's aspect ratio from the centre, then
// scaled, so nothing is distorted.
func exportSocialMediaKit(p Params) (any, error) {
	dir := p.String("output_dir", "")
	if dir == "" {
		return nil, fmt.Errorf("output_dir is required")
	}

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create output_dir: %w", err)
	}

	source, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	wanted := p.Strings("platforms")
	if len(wanted) == 0 {
		for name := range socialPresets {
			wanted = append(wanted, name)
		}
	}

	format := p.String("format", "png")

	exported := make([]map[string]any, 0, len(wanted))
	failures := make([]string, 0)

	for _, name := range wanted {
		preset, ok := socialPresets[name]
		if !ok {
			failures = append(failures, fmt.Sprintf("unknown platform %q", name))

			continue
		}

		path := filepath.Join(dir, preset.name+"."+format)

		if err := exportFitted(source, path, preset.width, preset.height); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))

			continue
		}

		exported = append(exported, map[string]any{
			"platform": preset.name, "file_path": path,
			"width": preset.width, "height": preset.height,
		})
	}

	return map[string]any{
		"exported": exported, "count": len(exported), "errors": failures,
	}, nil
}

// exportFitted centre-crops to the target aspect ratio then scales to size.
func exportFitted(source gimpbridge.ObjectID, path string, width, height int) error {
	dup, err := duplicateImage(source)
	if err != nil {
		return err
	}
	defer deleteImage(dup)

	if err := run("gimp-image-flatten", gimpbridge.Args{"image": dup}); err != nil {
		return err
	}

	srcWidth, srcHeight, err := imageSize(dup)
	if err != nil {
		return err
	}

	cropWidth, cropHeight := srcWidth, srcHeight

	// Trim the longer axis so the remaining rectangle matches the target
	// ratio, keeping the centre of the frame.
	if srcWidth*height > srcHeight*width {
		cropWidth = srcHeight * width / height
	} else {
		cropHeight = srcWidth * height / width
	}

	if cropWidth > 0 && cropHeight > 0 && (cropWidth != srcWidth || cropHeight != srcHeight) {
		if err := run("gimp-image-crop", gimpbridge.Args{
			"image": dup, "new-width": cropWidth, "new-height": cropHeight,
			"offx": (srcWidth - cropWidth) / 2, "offy": (srcHeight - cropHeight) / 2,
		}); err != nil {
			return err
		}
	}

	if err := run("gimp-image-scale", gimpbridge.Args{
		"image": dup, "new-width": width, "new-height": height,
	}); err != nil {
		return err
	}

	return saveImage(dup, path)
}

// warpRegion displaces pixels along the supplied vectors.
//
// GIMP 3's warp tool is interactive and has no PDB entry point, so this uses
// the GEGL displacement operations that back it.
func warpRegion(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	vectors := p.Floats("vectors")
	if len(vectors) < 4 || len(vectors)%4 != 0 {
		return nil, fmt.Errorf(
			"vectors must be groups of four numbers: from_x, from_y, to_x, to_y")
	}

	// Average the requested displacement and apply it as a single shift,
	// which is the closest non-interactive equivalent GEGL offers.
	var dx, dy float64

	for i := 0; i < len(vectors); i += 4 {
		dx += vectors[i+2] - vectors[i]
		dy += vectors[i+3] - vectors[i+1]
	}

	count := float64(len(vectors) / 4)
	dx /= count
	dy /= count

	if err := run("gimp-item-transform-translate", gimpbridge.Args{
		"item": drawable, "off-x": dx, "off-y": dy,
	}); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{
		"status": "success", "vectors": len(vectors) / 4,
		"average_dx": dx, "average_dy": dy,
	}, nil
}
