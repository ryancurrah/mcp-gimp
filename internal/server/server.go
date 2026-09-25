// Package server exposes the GIMP plug-in's command set as MCP tools.
//
// Almost every tool is a straight pass-through: the tool's arguments are
// marshalled into the plug-in's params object and the plug-in's results object
// is handed back unchanged. Those tools are defined in the tools_*.go files,
// grouped as the plug-in's commands are. The handful that reshape arguments or
// return something other than an object are in handwritten.go.
package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ryancurrah/mcp-gimp/internal/docs"
	"github.com/ryancurrah/mcp-gimp/internal/gimp"
)

// Name is the server name advertised to MCP clients.
const Name = "GimpMCP"

// defaulter is implemented by input types that carry non-zero
// defaults. Defaults are applied before the arguments go on the wire so the
// plug-in sees the documented value rather than a Go zero value.
type defaulter interface {
	SetDefaults()
}

// toolDef describes a pass-through tool.
type toolDef struct {
	// Name is the MCP tool name.
	Name string
	// Command is the plug-in command the tool forwards to.
	Command string
	// Required lists the arguments a caller must supply. Everything else is
	// optional and defaulted.
	Required []string
	// Description is the tool documentation shown to the model.
	Description string
}

// registrar collects tools onto a server, deferring schema errors so that
// registration reads as a flat list.
type registrar struct {
	srv    *mcp.Server
	client *gimp.Client
	// tools records every tool's arguments as registered, for the tests
	// that hold them to what GIMP accepts.
	tools []registeredTool
	errs  []error
}

// registeredTool is one tool and what its arguments declare.
type registeredTool struct {
	name string
	args []argSpec
}

// schemaFor infers the argument schema for In, narrows the required set and
// adds what the field tags declare; see schema.go.
//
// Inference marks every field without "omitempty" as required, but the wire
// format wants each key present (the plug-in distinguishes an explicit null
// from an absent key in a few places), so the required list is set explicitly
// from the tool definition instead.
func schemaFor[In any](r *registrar, tool string, required []string) (*jsonschema.Schema, error) {
	s, err := jsonschema.For[In](nil)
	if err != nil {
		return nil, err
	}

	s.Required = required

	args, err := argSpecs(reflect.TypeFor[In]())
	if err != nil {
		return nil, err
	}

	if err := applySpecs(s, args); err != nil {
		return nil, err
	}

	r.tools = append(r.tools, registeredTool{name: tool, args: args})

	return s, nil
}

// prepare applies defaults to decoded arguments.
func prepare[In any](in *In) {
	if d, ok := any(in).(defaulter); ok {
		d.SetDefaults()
	}
}

// addObject registers a tool that forwards its arguments unchanged and returns
// the plug-in's results object.
func addObject[In any](r *registrar, d toolDef) {
	schema, err := schemaFor[In](r, d.Name, d.Required)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("schema for %s: %w", d.Name, err))

		return
	}

	client := r.client

	mcp.AddTool(r.srv,
		&mcp.Tool{Name: d.Name, Description: d.Description, InputSchema: schema},
		func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
			prepare(&in)

			raw, err := client.Call(ctx, d.Command, in)
			if err != nil {
				return nil, nil, err
			}

			return nil, decodeResults(raw), nil
		})
}

// addImage registers a tool that returns the plug-in's exported PNG as image
// content rather than an object.
func addImage[In any](r *registrar, d toolDef) {
	schema, err := schemaFor[In](r, d.Name, d.Required)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("schema for %s: %w", d.Name, err))

		return
	}

	client := r.client

	mcp.AddTool(r.srv,
		&mcp.Tool{Name: d.Name, Description: d.Description, InputSchema: schema},
		func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
			prepare(&in)

			raw, err := client.Call(ctx, d.Command, in)
			if err != nil {
				return nil, nil, err
			}

			return imageResult(raw)
		})
}

// bitmapResults is the subset of a get_image_bitmap response the server reads.
type bitmapResults struct {
	ImageData string `json:"image_data"`
}

// imageResult converts an exported bitmap payload into MCP image content.
func imageResult(raw json.RawMessage) (*mcp.CallToolResult, any, error) {
	var res bitmapResults
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, nil, fmt.Errorf("decode bitmap response: %w", err)
	}

	if res.ImageData == "" {
		return nil, nil, errors.New("GIMP returned no image data")
	}

	png, err := base64.StdEncoding.DecodeString(res.ImageData)
	if err != nil {
		return nil, nil, fmt.Errorf("decode bitmap payload: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.ImageContent{Data: png, MIMEType: "image/png"}},
	}, nil, nil
}

// decodeResults turns the plug-in's results payload into a value the SDK can
// marshal back out. The plug-in returns objects for most commands but a bare
// string or list for a few, so the payload is decoded loosely.
func decodeResults(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}

	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return map[string]any{"raw": string(raw)}
	}

	return v
}

// New builds the MCP server with every GIMP tool and prompt registered.
func New(client *gimp.Client, version string) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    Name,
		Version: version,
		Title:   "GIMP",
	}, &mcp.ServerOptions{
		Instructions: "Controls a running GIMP instance through the GIMP MCP plug-in. " +
			"Call check_server first to confirm the plug-in is reachable. " +
			"Prefer the dedicated tools (delete_layer, set_layer_offsets, rotate_layer and the rest) " +
			"over call_api; call_api is the escape hatch for procedures no dedicated tool covers.",
	})

	r := &registrar{srv: srv, client: client}

	registerTools(r)
	registerPrompts(srv)

	if len(r.errs) > 0 {
		return nil, errors.Join(r.errs...)
	}

	return srv, nil
}

// registerTools adds every tool.
func registerTools(r *registrar) {
	registerCoreTools(r)
	registerBitmapTools(r)
	registerDrawingTools(r)
	registerPathTools(r)
	registerAdjustTools(r)
	registerEffectsTools(r)
	registerTransformTools(r)
	registerSelectionTools(r)
	registerLayersTools(r)
	registerTextTools(r)
	registerCompositeTools(r)
	registerHandwritten(r)
}

// registerPrompts exposes the guidance documents as MCP prompts.
func registerPrompts(srv *mcp.Server) {
	for _, p := range []struct {
		name, description, body string
	}{
		{
			name: "gimp_best_practices",
			description: "GIMP MCP best practices for common operations - filling shapes, " +
				"bezier paths, and variable persistence",
			body: docs.BestPractices,
		},
		{
			name: "gimp_iterative_workflow",
			description: "Iterative workflow guidance for building complex images with proper " +
				"validation and layer management",
			body: docs.IterativeWorkflow,
		},
	} {
		srv.AddPrompt(
			&mcp.Prompt{Name: p.name, Description: p.description},
			func(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				return &mcp.GetPromptResult{
					Description: p.description,
					Messages: []*mcp.PromptMessage{{
						Role:    "user",
						Content: &mcp.TextContent{Text: p.body},
					}},
				}, nil
			})
	}
}

// ptr returns a pointer to v. SetDefaults uses it to fill optional arguments
// that have a non-zero default.
func ptr[T any](v T) *T { return new(v) }
