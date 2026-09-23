package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerHandwritten wires up the tools that reshape their arguments or
// return something other than the plug-in's results object.
func registerHandwritten(r *registrar) {
	addCheckServer(r)
	addRestartServer(r)
	addStateSnapshot(r)
	addCallAPI(r)
}

// serverStatus is the reply shared by check_server and restart_server.
type serverStatus struct {
	Connected   bool   `json:"connected"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	GimpVersion string `json:"gimp_version,omitempty"`
	Error       string `json:"error,omitempty"`
}

// gimpInfoResults is the slice of get_gimp_info the status probe reads.
type gimpInfoResults struct {
	Version struct {
		VersionMethod string `json:"version_method"`
	} `json:"version"`
}

// probe asks the plug-in for its version. Unlike every other tool a failure is
// reported in the reply rather than as a tool error, because "is it running?"
// is exactly the question being asked.
func (r *registrar) probe(ctx context.Context) serverStatus {
	status := serverStatus{Host: r.client.Host(), Port: r.client.Port()}

	raw, err := r.client.Call(ctx, "get_gimp_info", nil)
	if err != nil {
		status.Error = err.Error()

		return status
	}

	status.Connected = true
	status.GimpVersion = "unknown"

	var info gimpInfoResults
	if err := json.Unmarshal(raw, &info); err == nil && info.Version.VersionMethod != "" {
		status.GimpVersion = info.Version.VersionMethod
	}

	return status
}

// addCheckServer registers the reachability probe.
func addCheckServer(r *registrar) {
	schema, err := schemaFor[CheckServerInput](r, "check_server", nil)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("schema for check_server: %w", err))

		return
	}

	mcp.AddTool(r.srv,
		&mcp.Tool{Name: "check_server", Description: checkServerDesc, InputSchema: schema},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ CheckServerInput) (*mcp.CallToolResult, any, error) {
			return nil, r.probe(ctx), nil
		})
}

// addRestartServer registers the reconnect tool.
//
// The client opens a fresh connection per command, matching the plug-in's
// one-request-per-connection handling, so there is no stale socket to discard.
// The tool remains because it answers the question callers actually ask after
// restarting GIMP: is the plug-in reachable again?
func addRestartServer(r *registrar) {
	schema, err := schemaFor[RestartServerInput](r, "restart_server", nil)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("schema for restart_server: %w", err))

		return
	}

	mcp.AddTool(r.srv,
		&mcp.Tool{Name: "restart_server", Description: restartServerDesc, InputSchema: schema},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ RestartServerInput) (*mcp.CallToolResult, any, error) {
			return nil, r.probe(ctx), nil
		})
}

// snapshotParams is the wire shape get_state_snapshot builds for the
// plug-in's get_image_bitmap command.
type snapshotParams struct {
	ImageIndex int           `json:"image_index"`
	MaxWidth   *int          `json:"max_width,omitempty"`
	MaxHeight  *int          `json:"max_height,omitempty"`
	Region     *bitmapRegion `json:"region,omitempty"`
}

// bitmapRegion is the plug-in's region rectangle.
type bitmapRegion struct {
	OriginX int `json:"origin_x"`
	OriginY int `json:"origin_y"`
	Width   int `json:"width"`
	Height  int `json:"height"`
}

// addStateSnapshot registers the preview tool. It translates the caller's
// {x, y, width, height} region into the plug-in's origin-based rectangle and
// fans max_size out to both bitmap dimensions.
func addStateSnapshot(r *registrar) {
	schema, err := schemaFor[GetStateSnapshotInput](r, "get_state_snapshot", nil)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("schema for get_state_snapshot: %w", err))

		return
	}

	client := r.client

	mcp.AddTool(r.srv,
		&mcp.Tool{Name: "get_state_snapshot", Description: getStateSnapshotDesc, InputSchema: schema},
		func(ctx context.Context, _ *mcp.CallToolRequest, in GetStateSnapshotInput) (*mcp.CallToolResult, any, error) {
			prepare(&in)

			params := snapshotParams{ImageIndex: in.ImageIndex}

			maxSize := 0
			if in.MaxSize != nil {
				maxSize = *in.MaxSize
			}

			// A max_size of zero means "no cap", matching the reference
			// implementation's truthiness check.
			if maxSize != 0 {
				params.MaxWidth = new(maxSize)
				params.MaxHeight = new(maxSize)
			}

			if in.Region != nil {
				params.Region = &bitmapRegion{
					OriginX: valueOr(in.Region.X, 0),
					OriginY: valueOr(in.Region.Y, 0),
					Width:   valueOr(in.Region.Width, maxSize),
					Height:  valueOr(in.Region.Height, maxSize),
				}
			}

			raw, err := client.Call(ctx, "get_image_bitmap", params)
			if err != nil {
				return nil, nil, err
			}

			return imageResult(raw)
		})
}

// valueOr dereferences an optional argument, or returns fallback.
func valueOr(v *int, fallback int) int {
	if v == nil {
		return fallback
	}

	return *v
}

// addCallAPI registers the escape hatch onto GIMP's procedural database.
//
// It returns its result as a JSON string, and reports plug-in failures in that
// string rather than as a tool error, because callers iterate on snippets and
// need to read the traceback.
func addCallAPI(r *registrar) {
	schema, err := schemaFor[CallAPIInput](r, "call_api", []string{"api_path"})
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("schema for call_api: %w", err))

		return
	}

	client := r.client

	mcp.AddTool(r.srv,
		&mcp.Tool{Name: "call_api", Description: callAPIDesc, InputSchema: schema},
		func(ctx context.Context, _ *mcp.CallToolRequest, in CallAPIInput) (*mcp.CallToolResult, any, error) {
			prepare(&in)

			resp, err := client.Send(ctx, "call_api", in)
			if err != nil {
				return textResult("Error: %s", err), nil, nil
			}

			if resp.Status != "success" {
				return textResult("Error: %s", describeRaw(resp.Error)), nil, nil
			}

			return textResult("%s", resp.Results), nil, nil
		})
}

// textResult wraps formatted text as a tool result. call_api hands back the
// raw JSON GIMP produced, so it is emitted as text rather than re-encoded.
func textResult(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
	}
}

// describeRaw renders a raw JSON error payload for inclusion in a message.
func describeRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "unknown error"
	}

	return string(raw)
}
