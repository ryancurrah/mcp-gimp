package commands

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
)

//nolint:gochecknoinits // the command table is assembled from several files
func init() {
	register("get_image_bitmap", getImageBitmap)
	register("export_image", exportImage)
	register("save_xcf", saveXCF)
}

// getImageBitmap renders an image to PNG and returns it base64 encoded.
//
// The work happens on a throwaway duplicate so cropping, scaling and
// flattening never touch what the user has open.
func getImageBitmap(p Params) (any, error) {
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

	region := p.Object("region")
	maxWidth := p.Int("max_width", 0)
	maxHeight := p.Int("max_height", 0)

	if region != nil {
		w := region.Int("width", 0)
		h := region.Int("height", 0)

		if w <= 0 || h <= 0 {
			return nil, fmt.Errorf("region width and height must be positive")
		}

		if err := run("gimp-image-crop", gimpbridge.Args{
			"image":      dup,
			"new-width":  w,
			"new-height": h,
			"offx":       region.Int("origin_x", 0),
			"offy":       region.Int("origin_y", 0),
		}); err != nil {
			return nil, err
		}

		// A region may carry its own scaling bounds.
		if v := region.Int("max_width", 0); v > 0 {
			maxWidth = v
		}

		if v := region.Int("max_height", 0); v > 0 {
			maxHeight = v
		}
	}

	if err := scaleWithin(dup, maxWidth, maxHeight); err != nil {
		return nil, err
	}

	width, height, err := imageSize(dup)
	if err != nil {
		return nil, err
	}

	png, err := exportPNG(dup)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"image_data": base64.StdEncoding.EncodeToString(png),
		"width":      width,
		"height":     height,
		"format":     "png",
		"size_bytes": len(png),
	}, nil
}

// duplicateImage copies an image so destructive steps stay off the original.
func duplicateImage(image gimpbridge.ObjectID) (gimpbridge.ObjectID, error) {
	v, err := run1("gimp-image-duplicate", gimpbridge.Args{"image": image})
	if err != nil {
		return 0, err
	}

	return objectID(v)
}

// deleteImage discards a working copy, ignoring failures during cleanup.
func deleteImage(image gimpbridge.ObjectID) {
	_ = run("gimp-image-delete", gimpbridge.Args{"image": image})
}

// scaleWithin shrinks an image to fit the given bounds, preserving aspect
// ratio. Zero bounds mean "leave this dimension alone"; an image already
// inside the bounds is untouched.
func scaleWithin(image gimpbridge.ObjectID, maxWidth, maxHeight int) error {
	if maxWidth <= 0 && maxHeight <= 0 {
		return nil
	}

	width, height, err := imageSize(image)
	if err != nil {
		return err
	}

	if width == 0 || height == 0 {
		return nil
	}

	scale := 1.0

	if maxWidth > 0 && width > maxWidth {
		scale = float64(maxWidth) / float64(width)
	}

	if maxHeight > 0 && height > maxHeight {
		if s := float64(maxHeight) / float64(height); s < scale {
			scale = s
		}
	}

	if scale >= 1.0 {
		return nil
	}

	newWidth := max(int(float64(width)*scale), 1)
	newHeight := max(int(float64(height)*scale), 1)

	return run("gimp-image-scale", gimpbridge.Args{
		"image": image, "new-width": newWidth, "new-height": newHeight,
	})
}

// exportPNG writes an image to a temporary PNG and returns its bytes.
//
// GIMP owns the encoding, so the pixels go out through the same file export
// path a user would use rather than being read out of a buffer.
func exportPNG(image gimpbridge.ObjectID) ([]byte, error) {
	dir, err := os.MkdirTemp("", "gimp-mcp-")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(dir) //nolint:errcheck // best effort cleanup

	path := filepath.Join(dir, "snapshot.png")

	if err := saveImage(image, path); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is ours
	if err != nil {
		return nil, fmt.Errorf("read exported PNG: %w", err)
	}

	return data, nil
}

// exportSettings carry the format-specific knobs an export can take.
type exportSettings struct {
	// Quality is the JPEG/WEBP quality, 1-100. Zero means "use the default".
	Quality float64
	// PNGCompression is the zlib level 0-9, or -1 for the default.
	PNGCompression int
}

// saveImageWith writes an image to path, routing through the format-specific
// export procedure when a quality or compression setting was supplied.
//
// GIMP 3 names these file-<format>-export and they each take their own
// options; gimp-file-save picks a handler by extension but cannot carry them.
func saveImageWith(image gimpbridge.ObjectID, path string, s exportSettings) error {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))

	switch ext {
	case "jpg", "jpeg":
		if s.Quality > 0 {
			return run("file-jpeg-export", gimpbridge.Args{
				"run-mode": 1, "image": image, "file": path,
				"quality": clampQuality(s.Quality),
			})
		}
	case "webp":
		if s.Quality > 0 {
			return run("file-webp-export", gimpbridge.Args{
				"run-mode": 1, "image": image, "file": path,
				"quality": clampQuality(s.Quality),
			})
		}
	case "png":
		if s.PNGCompression >= 0 {
			return run("file-png-export", gimpbridge.Args{
				"run-mode": 1, "image": image, "file": path,
				"compression": min(max(s.PNGCompression, 0), 9),
			})
		}
	}

	return saveImage(image, path)
}

// clampQuality converts a 1-100 quality to the 0-1 range GIMP 3 expects.
func clampQuality(q float64) float64 {
	if q > 1 {
		q /= 100
	}

	return min(max(q, 0), 1)
}

// saveImage writes an image to path, letting GIMP pick the handler from the
// file extension.
func saveImage(image gimpbridge.ObjectID, path string) error {
	layers, err := layersOf(image)
	if err != nil {
		return err
	}

	args := gimpbridge.Args{
		"image":    image,
		"file":     path,
		"run-mode": 1, // NONINTERACTIVE
	}

	// Older save procedures want the drawables explicitly.
	if len(layers) > 0 {
		args["drawables"] = gimpbridge.Items(layers)
	}

	if err := run("gimp-file-save", args); err != nil {
		// Retry without drawables for handlers that do not declare them.
		delete(args, "drawables")

		if err2 := run("gimp-file-save", args); err2 != nil {
			return err
		}
	}

	return nil
}

// exportImage writes the image to a raster file.
func exportImage(p Params) (any, error) {
	path := p.String("file_path", "")
	if path == "" {
		return nil, fmt.Errorf("file_path is required")
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

	if p.Bool("flatten", true) {
		if err := run("gimp-image-flatten", gimpbridge.Args{"image": dup}); err != nil {
			return nil, err
		}
	}

	if err := saveImageWith(dup, path, exportSettings{
		Quality: p.Float("quality", 0), PNGCompression: -1,
	}); err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("exported file is missing: %w", err)
	}

	return map[string]any{
		"status":          "success",
		"file_path":       path,
		"format":          p.String("format", "png"),
		"file_size_bytes": info.Size(),
	}, nil
}

// saveXCF writes the image as GIMP's native format, keeping layers.
func saveXCF(p Params) (any, error) {
	path := p.String("file_path", "")
	if path == "" {
		return nil, fmt.Errorf("file_path is required")
	}

	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	if err := run("gimp-xcf-save", gimpbridge.Args{
		"image": image, "file": path, "run-mode": 1,
	}); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "file_path": path}, nil
}
