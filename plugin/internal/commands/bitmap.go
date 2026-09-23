package commands

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	// StripMetadata leaves Exif, IPTC, XMP, comments and the embedded
	// thumbnail out of the file.
	StripMetadata bool
}

// metadataArgs are the export arguments that embed metadata. GIMP 3 has no
// procedure that clears an image's metadata; what reaches the file is decided
// per export by these flags instead.
var metadataArgs = map[string][]string{ //nolint:gochecknoglobals // fixed table
	"file-jpeg-export": {"include-exif", "include-iptc", "include-xmp", "include-comment", "include-thumbnail"},
	"file-png-export":  {"include-exif", "include-iptc", "include-xmp", "include-comment", "include-thumbnail"},
	"file-webp-export": {"include-exif", "include-iptc", "include-xmp", "include-thumbnail"},
}

// saveImageWith writes an image to path, routing through the format-specific
// export procedure when a quality, compression or metadata setting was
// supplied.
//
// GIMP 3 names these file-<format>-export and they each take their own
// options; gimp-file-save picks a handler by extension but cannot carry them.
func saveImageWith(image gimpbridge.ObjectID, path string, s exportSettings) error {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	args := gimpbridge.Args{}

	var proc string

	switch ext {
	case "jpg", "jpeg":
		proc = "file-jpeg-export"

		if s.Quality > 0 {
			// JPEG takes its quality as a fraction.
			args["quality"] = min(max(s.Quality, 1), 100) / 100
		}
	case "webp":
		proc = "file-webp-export"

		if s.Quality > 0 {
			// WebP, unlike JPEG, takes its quality on 0-100.
			args["quality"] = min(max(s.Quality, 1), 100)
		}
	case "png":
		proc = "file-png-export"

		if s.PNGCompression >= 0 {
			args["compression"] = min(s.PNGCompression, 9)
		}
	}

	if s.StripMetadata {
		if proc == "" {
			return fmt.Errorf("strip_metadata is supported for png, jpeg and webp, not %q", ext)
		}

		for _, name := range metadataArgs[proc] {
			args[name] = false
		}
	}

	if len(args) == 0 {
		return saveImage(image, path)
	}

	args["run-mode"] = runNonInteractive
	args["image"] = image
	args["file"] = path

	return run(proc, args)
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
		"run-mode": runNonInteractive,
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

	format, err := exportFormat(path, p.String("format", ""))
	if err != nil {
		return nil, err
	}

	source, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	// The export works on a copy, so flattening never touches the open image.
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
		"format":          format,
		"file_size_bytes": info.Size(),
	}, nil
}

// exportFormats maps each export format to the file extensions GIMP writes it
// for.
var exportFormats = map[string][]string{ //nolint:gochecknoglobals // fixed table
	"png":  {"png"},
	"jpeg": {"jpg", "jpeg"},
	"webp": {"webp"},
	"tiff": {"tif", "tiff"},
}

// exportFormat names the format GIMP will write path in, which its extension
// decides. A format the caller names must agree with the extension, since
// GIMP would otherwise write a different format than the one asked for.
func exportFormat(path, want string) (string, error) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))

	for format, exts := range exportFormats {
		if !slices.Contains(exts, ext) {
			continue
		}

		if want != "" && want != format {
			return "", fmt.Errorf("format %q does not match the file extension %q, "+
				"which GIMP writes as %s; change one to match the other", want, "."+ext, format)
		}

		return format, nil
	}

	return "", fmt.Errorf("file_path must end in .png, .jpg, .jpeg, .webp, .tif or .tiff, not %q", filepath.Base(path))
}

// saveXCF saves the image in GIMP's native format, keeping layers.
func saveXCF(p Params) (any, error) {
	path := p.String("file_path", "")
	if path == "" {
		return nil, fmt.Errorf("file_path is required")
	}

	// GIMP picks the format from the extension, so any other one would write
	// something other than XCF.
	if !strings.EqualFold(filepath.Ext(path), ".xcf") {
		return nil, fmt.Errorf("file_path must end in .xcf, not %q", filepath.Base(path))
	}

	image, err := imageAt(p.Int("image_index", 0))
	if err != nil {
		return nil, err
	}

	if err := saveAsXCF(image, path); err != nil {
		return nil, err
	}

	return map[string]any{"status": "success", "file_path": path}, nil
}

// saveAsXCF saves an image to path as File > Save does: the image records the
// file and is marked clean. gimp-xcf-save writes the same file but does
// neither, so the image would still look unsaved to close_image.
func saveAsXCF(image gimpbridge.ObjectID, path string) error {
	return run("gimp-file-save", gimpbridge.Args{
		"image": image, "file": path, "run-mode": runNonInteractive,
	})
}
