package server_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ryancurrah/mcp-gimp/internal/gimp"
	"github.com/ryancurrah/mcp-gimp/internal/server"
)

// totalTools is the number of tools the server exposes.
const totalTools = 83

// fakeGimp records the params each command received and replies with a canned
// results payload.
type fakeGimp struct {
	mu       sync.Mutex
	lastType string
	lastArgs map[string]any
	results  string
	status   string
}

// start brings up a fake plug-in on loopback.
func (f *fakeGimp) start(t *testing.T) *gimp.Client {
	t.Helper()

	if f.status == "" {
		f.status = "success"
	}

	if f.results == "" {
		f.results = "{}"
	}

	var lc net.ListenConfig

	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}

			go func() {
				defer conn.Close() //nolint:errcheck // test helper

				var req struct {
					Type   string         `json:"type"`
					Params map[string]any `json:"params"`
				}

				if err := json.NewDecoder(conn).Decode(&req); err != nil {
					return
				}

				f.mu.Lock()
				f.lastType, f.lastArgs = req.Type, req.Params
				body := `{"status":"` + f.status + `","results":` + f.results + `,"error":"boom"}`
				f.mu.Unlock()

				_, _ = io.WriteString(conn, body)
			}()
		}
	}()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address %T", ln.Addr())
	}

	return gimp.New(gimp.WithHost("127.0.0.1"), gimp.WithPort(addr.Port),
		gimp.WithCallTimeout(10*time.Second))
}

// args returns the params the fake last received.
func (f *fakeGimp) args() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.lastArgs
}

// command returns the command the fake last received.
func (f *fakeGimp) command() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.lastType
}

// connect wires an in-memory MCP client to a server backed by client.
func connect(t *testing.T, c *gimp.Client) *mcp.ClientSession {
	t.Helper()

	srv, err := server.New(c, "test")
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	if _, err := srv.Connect(t.Context(), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).
		Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	t.Cleanup(func() { _ = cs.Close() })

	return cs
}

// callTool invokes a tool and fails the test on a protocol error.
func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}

	return res
}

func TestEveryToolIsRegistered(t *testing.T) {
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	var names []string

	for tool, err := range cs.Tools(t.Context(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}

		names = append(names, tool.Name)

		if tool.Description == "" {
			t.Errorf("tool %s has no description", tool.Name)
		}
	}

	if len(names) != totalTools {
		t.Errorf("registered %d tools, want %d", len(names), totalTools)
	}

	// Spot-check one tool from each category of the tool set.
	for _, want := range []string{
		"check_server", "restart_server", "new_canvas", "get_image_bitmap",
		"get_state_snapshot", "call_api", "open_image", "export_image",
		"auto_levels", "scale_image", "select_rectangle", "create_layer",
		"draw_line", "add_text", "apply_drop_shadow", "export_icon_sizes",
		"list_images", "get_histogram",
	} {
		if !slices.Contains(names, want) {
			t.Errorf("tool %s is missing", want)
		}
	}
}

func TestDefaultsAreSentOnTheWire(t *testing.T) {
	// Every parameter goes on the wire, with documented defaults filled in.
	// The plug-in reads several of them, so the defaults have to be applied
	// before the request is sent.
	f := &fakeGimp{results: `{"file_path":"/tmp/out.png"}`}
	cs := connect(t, f.start(t))

	callTool(t, cs, "export_image", map[string]any{"file_path": "/tmp/out.png"})

	args := f.args()
	for key, want := range map[string]any{
		"format":      "png",
		"quality":     float64(90),
		"flatten":     true,
		"image_index": float64(0),
	} {
		if got := args[key]; got != want {
			t.Errorf("params[%q] = %#v, want %#v", key, got, want)
		}
	}
}

func TestExplicitArgumentsOverrideDefaults(t *testing.T) {
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	callTool(t, cs, "export_image", map[string]any{
		"file_path": "/tmp/out.jpg",
		"format":    "jpeg",
		"quality":   40,
		"flatten":   false,
	})

	args := f.args()
	if args["format"] != "jpeg" || args["quality"] != float64(40) || args["flatten"] != false {
		t.Errorf("params = %#v, want the caller's values", args)
	}
}

func TestZeroIsDistinguishedFromUnset(t *testing.T) {
	// quality defaults to 90, so an explicit 0 must not be mistaken for
	// "not supplied" and silently replaced.
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	callTool(t, cs, "export_image", map[string]any{"file_path": "/tmp/o.jpg", "quality": 0})

	if got := f.args()["quality"]; got != float64(0) {
		t.Errorf("quality = %#v, want 0 to survive as 0", got)
	}
}

func TestOptionalArgumentsAreSentAsNull(t *testing.T) {
	// The plug-in reads params.get("color", "white"); the reference client
	// always sent the key, so an unset colour arrives as null, not "white".
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	callTool(t, cs, "fill_selection", map[string]any{})

	args := f.args()
	if _, present := args["color"]; !present {
		t.Error("color key absent; the plug-in would fall back to white")
	}

	if got := args["color"]; got != nil {
		t.Errorf("color = %#v, want null", got)
	}
}

func TestRequiredArgumentsAreEnforced(t *testing.T) {
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "open_image", Arguments: map[string]any{},
	})
	if err == nil && !res.IsError {
		t.Fatal("open_image without file_path succeeded, want a validation error")
	}
}

func TestOptionalArgumentsAreNotRequired(t *testing.T) {
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	for tool, err := range cs.Tools(t.Context(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}

		if tool.Name != "export_image" {
			continue
		}

		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("input schema is %T, want an object", tool.InputSchema)
		}

		required, _ := schema["required"].([]any)
		if len(required) != 1 || required[0] != "file_path" {
			t.Errorf("required = %v, want [file_path]", required)
		}
	}
}

func TestImageToolReturnsPNGContent(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 1, 2, 3}
	f := &fakeGimp{results: `{"image_data":"` + base64.StdEncoding.EncodeToString(png) + `"}`}
	cs := connect(t, f.start(t))

	res := callTool(t, cs, "get_image_bitmap", map[string]any{})

	if len(res.Content) != 1 {
		t.Fatalf("got %d content items, want 1", len(res.Content))
	}

	img, ok := res.Content[0].(*mcp.ImageContent)
	if !ok {
		t.Fatalf("content is %T, want ImageContent", res.Content[0])
	}

	if img.MIMEType != "image/png" {
		t.Errorf("mime = %q, want image/png", img.MIMEType)
	}

	if string(img.Data) != string(png) {
		t.Errorf("image data = %v, want the decoded PNG bytes", img.Data)
	}
}

func TestStateSnapshotReshapesRegion(t *testing.T) {
	f := &fakeGimp{results: `{"image_data":"` + base64.StdEncoding.EncodeToString([]byte("x")) + `"}`}
	cs := connect(t, f.start(t))

	callTool(t, cs, "get_state_snapshot", map[string]any{
		"region": map[string]any{"x": 200, "y": 300, "width": 100, "height": 80},
	})

	if got := f.command(); got != "get_image_bitmap" {
		t.Errorf("command = %q, want get_image_bitmap", got)
	}

	args := f.args()

	// max_size fans out to both bitmap dimensions.
	if args["max_width"] != float64(512) || args["max_height"] != float64(512) {
		t.Errorf("max dimensions = %v/%v, want 512/512", args["max_width"], args["max_height"])
	}

	region, ok := args["region"].(map[string]any)
	if !ok {
		t.Fatalf("region is %T, want an object", args["region"])
	}

	for key, want := range map[string]float64{
		"origin_x": 200, "origin_y": 300, "width": 100, "height": 80,
	} {
		if region[key] != want {
			t.Errorf("region[%q] = %v, want %v", key, region[key], want)
		}
	}
}

func TestStateSnapshotRegionFallsBackToMaxSize(t *testing.T) {
	f := &fakeGimp{results: `{"image_data":"` + base64.StdEncoding.EncodeToString([]byte("x")) + `"}`}
	cs := connect(t, f.start(t))

	callTool(t, cs, "get_state_snapshot", map[string]any{
		"max_size": 256,
		"region":   map[string]any{"x": 10, "y": 20},
	})

	region, ok := f.args()["region"].(map[string]any)
	if !ok {
		t.Fatalf("region is %T, want an object", f.args()["region"])
	}

	if region["width"] != float64(256) || region["height"] != float64(256) {
		t.Errorf("region size = %v/%v, want it to fall back to max_size",
			region["width"], region["height"])
	}
}

func TestCheckServerReportsFailureInsteadOfErroring(t *testing.T) {
	// check_server answers "is the plug-in up?", so an unreachable plug-in is
	// a normal reply, not a tool error.
	c := gimp.New(gimp.WithHost("127.0.0.1"), gimp.WithPort(1),
		gimp.WithDialTimeout(300*time.Millisecond))
	cs := connect(t, c)

	res := callTool(t, cs, "check_server", map[string]any{})
	if res.IsError {
		t.Fatal("check_server reported a tool error, want a status object")
	}

	status, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content is %T, want an object", res.StructuredContent)
	}

	if status["connected"] != false {
		t.Errorf("connected = %v, want false", status["connected"])
	}

	if status["error"] == nil || status["error"] == "" {
		t.Error("no error text; the caller cannot tell what went wrong")
	}
}

func TestCheckServerReportsVersion(t *testing.T) {
	f := &fakeGimp{results: `{"version":{"version_method":"3.2.0"}}`}
	cs := connect(t, f.start(t))

	res := callTool(t, cs, "check_server", map[string]any{})

	status, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content is %T, want an object", res.StructuredContent)
	}

	if status["connected"] != true {
		t.Errorf("connected = %v, want true", status["connected"])
	}

	if status["gimp_version"] != "3.2.0" {
		t.Errorf("gimp_version = %v, want 3.2.0", status["gimp_version"])
	}
}

func TestCallAPIReportsErrorsAsText(t *testing.T) {
	f := &fakeGimp{status: "error", results: "null"}
	cs := connect(t, f.start(t))

	res := callTool(t, cs, "call_api", map[string]any{"api_path": "exec"})
	if res.IsError {
		t.Fatal("call_api raised a tool error, want the failure as text")
	}

	text := resultText(t, res)
	if !strings.HasPrefix(text, "Error:") {
		t.Errorf("result = %q, want it to start with Error:", text)
	}
}

func TestCallAPIDefaultsCollections(t *testing.T) {
	f := &fakeGimp{results: `{"ok":true}`}
	cs := connect(t, f.start(t))

	callTool(t, cs, "call_api", map[string]any{"api_path": "exec"})

	args := f.args()

	if got, ok := args["args"].([]any); !ok || got == nil {
		t.Errorf("args = %#v, want an empty list", args["args"])
	}

	if got, ok := args["kwargs"].(map[string]any); !ok || got == nil {
		t.Errorf("kwargs = %#v, want an empty object", args["kwargs"])
	}
}

func TestToolErrorsSurfaceGimpMessage(t *testing.T) {
	f := &fakeGimp{status: "error", results: "null"}
	cs := connect(t, f.start(t))

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "auto_levels", Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	if !res.IsError {
		t.Fatal("auto_levels succeeded, want the plug-in error surfaced")
	}

	if !strings.Contains(resultText(t, res), "boom") {
		t.Errorf("result = %q, want the plug-in message", resultText(t, res))
	}
}

func TestPromptsAreServed(t *testing.T) {
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	want := map[string]bool{"gimp_best_practices": false, "gimp_iterative_workflow": false}

	for p, err := range cs.Prompts(t.Context(), nil) {
		if err != nil {
			t.Fatalf("list prompts: %v", err)
		}

		if _, ok := want[p.Name]; ok {
			want[p.Name] = true
		}
	}

	for name, found := range want {
		if !found {
			t.Errorf("prompt %s is missing", name)
		}
	}

	res, err := cs.GetPrompt(t.Context(), &mcp.GetPromptParams{Name: "gimp_best_practices"})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}

	if len(res.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(res.Messages))
	}

	text, ok := res.Messages[0].Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("content is %T, want text", res.Messages[0].Content)
	}

	if len(text.Text) < 100 {
		t.Errorf("prompt body is %d bytes, want the embedded document", len(text.Text))
	}
}

// resultText joins the text content of a tool result.
func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()

	var b strings.Builder

	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}

	return b.String()
}
