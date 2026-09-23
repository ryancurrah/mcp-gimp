// Command introspect records what the running GIMP accepts into
// internal/snapshot/gimp.json.
//
// It asks the plug-in to describe every PDB procedure and GEGL operation the
// plug-in's source names, so the snapshot always matches the GIMP that is
// actually installed. Nothing published carries this: GIMP's API reference
// documents the procedures in prose, and neither it nor GEGL's reference lists
// the operation properties GIMP's filter configuration exposes.
//
// Run it with `make introspect` while GIMP is running with the MCP plug-in
// started, then review the diff.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/ryancurrah/mcp-gimp/internal/snapshot"
	"github.com/ryancurrah/mcp-gimp/internal/gimp"
)

func main() {
	out := flag.String("out", "internal/snapshot/gimp.json", "snapshot file to write")
	src := flag.String("plugin", "plugin/internal/commands", "plug-in command source to scan")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	snap, err := introspect(ctx, gimp.New(), *src)

	cancel()

	if err != nil {
		log.Fatalf("introspect: %v", err)
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		log.Fatalf("introspect: %v", err)
	}

	if err := os.WriteFile(*out, append(data, '\n'), 0o644); err != nil { //nolint:gosec // committed data file
		log.Fatalf("introspect: write %s: %v", *out, err)
	}

	fmt.Printf("introspect: GIMP %s, %d procedures, %d operations -> %s\n",
		snap.GIMP, len(snap.Procedures), len(snap.Operations), *out)
}

// introspect builds a snapshot from the running GIMP.
func introspect(ctx context.Context, c *gimp.Client, src string) (*snapshot.Snapshot, error) {
	procs, ops, err := snapshot.References(src)
	if err != nil {
		return nil, err
	}

	version, err := gimpVersion(ctx, c)
	if err != nil {
		return nil, err
	}

	snap := &snapshot.Snapshot{
		GIMP:       version,
		Procedures: map[string][]snapshot.Property{},
		Operations: map[string][]snapshot.Property{},
	}

	var missing []error

	for _, name := range procs {
		var res struct {
			Arguments []snapshot.Property `json:"arguments"`
		}

		if err := call(ctx, c, "describe_procedure", map[string]any{"procedure": name}, &res); err != nil {
			// A procedure the plug-in names but GIMP lacks is exactly what an
			// upgrade must surface, so it is collected rather than skipped.
			missing = append(missing, err)

			continue
		}

		snap.Procedures[name] = res.Arguments
	}

	// Operations are described through a filter on a real drawable, so a
	// scratch image is opened for the purpose and closed afterwards.
	index, closeImage, err := scratchImage(ctx, c)
	if err != nil {
		return nil, err
	}
	defer closeImage()

	for _, name := range ops {
		var res struct {
			Properties []snapshot.Property `json:"properties"`
		}

		params := map[string]any{"operation": name, "image_index": index}
		if err := call(ctx, c, "describe_operation", params, &res); err != nil {
			missing = append(missing, err)

			continue
		}

		snap.Operations[name] = res.Properties
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("the plug-in names things this GIMP does not have:\n%w", errors.Join(missing...))
	}

	return snap, nil
}

// versionNumber pulls "3.2.6" out of however gimp-version phrases it.
var versionNumber = regexp.MustCompile(`\d+\.\d+\.\d+`)

func gimpVersion(ctx context.Context, c *gimp.Client) (string, error) {
	var info struct {
		Version struct {
			Method any `json:"version_method"`
		} `json:"version"`
	}

	if err := call(ctx, c, "get_gimp_info", nil, &info); err != nil {
		return "", err
	}

	v := versionNumber.FindString(fmt.Sprint(info.Version.Method))
	if v == "" {
		return "", fmt.Errorf("cannot read a version from %v", info.Version.Method)
	}

	return v, nil
}

// scratchImage opens a small image and returns its index and a closer.
func scratchImage(ctx context.Context, c *gimp.Client) (int, func(), error) {
	var created struct {
		ImageID int `json:"image_id"`
	}

	if err := call(ctx, c, "new_canvas", map[string]any{
		"width": 8, "height": 8, "name": "gimp-mcp introspect",
	}, &created); err != nil {
		return 0, nil, err
	}

	var list struct {
		Images []struct {
			Index   int `json:"index"`
			ImageID int `json:"image_id"`
		} `json:"images"`
	}

	if err := call(ctx, c, "list_images", nil, &list); err != nil {
		return 0, nil, err
	}

	for _, img := range list.Images {
		if img.ImageID == created.ImageID {
			return img.Index, func() {
				_ = call(ctx, c, "close_image", map[string]any{"image_index": img.Index, "force": true}, nil)
			}, nil
		}
	}

	return 0, nil, fmt.Errorf("scratch image %d is not in the image list", created.ImageID)
}

func call(ctx context.Context, c *gimp.Client, command string, params, out any) error {
	raw, err := c.Call(ctx, command, params)
	if err != nil {
		return fmt.Errorf("%s: %w", command, err)
	}

	if out == nil {
		return nil
	}

	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s: decode: %w", command, err)
	}

	return nil
}
