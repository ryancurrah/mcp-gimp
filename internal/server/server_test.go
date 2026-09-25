package server_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"maps"
	"net"
	"reflect"
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
const totalTools = 92

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
		"list_images", "get_histogram", "fill_rounded_rectangle",
		"draw_rounded_rectangle", "select_rounded_rectangle", "rotate_layer",
		"draw_path", "fill_path", "select_path", "list_paths", "path_to_selection",
		"draw_shapes",
	} {
		if !slices.Contains(names, want) {
			t.Errorf("tool %s is missing", want)
		}
	}

	// GIMP 3 has no procedure that steps the undo stack, so these could only
	// ever fail.
	for _, gone := range []string{"undo", "redo"} {
		if slices.Contains(names, gone) {
			t.Errorf("tool %s is registered, but GIMP 3 cannot step the undo stack", gone)
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
		"quality":     float64(90),
		"flatten":     true,
		"image_index": float64(0),
	} {
		if got := args[key]; got != want {
			t.Errorf("params[%q] = %#v, want %#v", key, got, want)
		}
	}

	// The file extension decides the format, so none is assumed.
	if got, ok := args["format"]; !ok || got != nil {
		t.Errorf("params[\"format\"] = %#v, want null", got)
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

// schemaOf returns the advertised input schema for one tool.
func schemaOf(t *testing.T, cs *mcp.ClientSession, name string) map[string]any {
	t.Helper()

	for tool, err := range cs.Tools(t.Context(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}

		if tool.Name != name {
			continue
		}

		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Fatalf("%s input schema is %T, want an object", name, tool.InputSchema)
		}

		return schema
	}

	t.Fatalf("tool %s not found", name)

	return nil
}

// propertyOf returns one argument's schema.
func propertyOf(t *testing.T, schema map[string]any, name string) map[string]any {
	t.Helper()

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema has no properties")
	}

	prop, ok := props[name].(map[string]any)
	if !ok {
		t.Fatalf("no schema for argument %q", name)
	}

	return prop
}

func TestEnumIsAdvertised(t *testing.T) {
	// The permitted values used to appear only in the description prose.
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	prop := propertyOf(t, schemaOf(t, cs, "flip_image"), "direction")

	enum, ok := prop["enum"].([]any)
	if !ok {
		t.Fatalf("direction has no enum: %#v", prop)
	}

	want := []any{"horizontal", "vertical", nil}
	if !slices.Equal(enum, want) {
		t.Errorf("enum = %#v, want %#v", enum, want)
	}

	if got := prop["default"]; got != "horizontal" {
		t.Errorf("default = %#v, want \"horizontal\"", got)
	}
}

func TestEnumRejectsAnUnlistedValue(t *testing.T) {
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "gradient_fill", Arguments: map[string]any{"gradient_type": "diagonal"},
	})
	if err == nil && !res.IsError {
		t.Fatal("gradient_type=diagonal succeeded, want a validation error")
	}
}

func TestNullableEnumStillAcceptsNull(t *testing.T) {
	// The argument is typed ["null", "string"], so an explicit null has to
	// stay legal or the enum would contradict the type.
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	callTool(t, cs, "gradient_fill", map[string]any{"gradient_type": nil})
}

func TestNumericBoundsAreAdvertisedAndEnforced(t *testing.T) {
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	prop := propertyOf(t, schemaOf(t, cs, "apply_drop_shadow"), "opacity")
	if prop["minimum"] != float64(0) || prop["maximum"] != float64(100) {
		t.Errorf("opacity bounds = %#v/%#v, want 0/100", prop["minimum"], prop["maximum"])
	}

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "apply_drop_shadow", Arguments: map[string]any{"opacity": 250},
	})
	if err == nil && !res.IsError {
		t.Fatal("opacity=250 succeeded, want a validation error")
	}
}

func TestRegionShapesAreDistinct(t *testing.T) {
	// get_image_bitmap crops with origin_x/origin_y; get_state_snapshot takes
	// x/y and translates them. Both used to advertise a bare object.
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	for _, tc := range []struct{ tool, key string }{
		{"get_image_bitmap", "origin_x"},
		{"get_state_snapshot", "x"},
	} {
		region := propertyOf(t, schemaOf(t, cs, tc.tool), "region")

		keys, ok := region["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s: region has no properties", tc.tool)
		}

		if _, present := keys[tc.key]; !present {
			t.Errorf("%s: region has no %q key, got %v", tc.tool, tc.key, slices.Sorted(maps.Keys(keys)))
		}
	}
}

func TestConstrainedArgumentsExistAndAgreeWithDefaults(t *testing.T) {
	// A curated constraint that names an argument the struct does not have
	// fails registration outright, so reaching here proves every constrained
	// argument exists. What is checked here is that an enum has not been
	// written to exclude the tool's own default.
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	checked := 0

	for tool, err := range cs.Tools(t.Context(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}

		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			continue
		}

		props, ok := schema["properties"].(map[string]any)
		if !ok {
			continue
		}

		for name, raw := range props {
			prop, ok := raw.(map[string]any)
			if !ok {
				continue
			}

			enum, hasEnum := prop["enum"].([]any)
			def, hasDefault := prop["default"]

			if !hasEnum || !hasDefault {
				continue
			}

			if !slices.Contains(enum, def) {
				t.Errorf("%s.%s: default %#v is not in enum %#v", tool.Name, name, def, enum)
			}

			checked++
		}
	}

	if checked == 0 {
		t.Fatal("no argument carried both an enum and a default; the table is not wired up")
	}

	t.Logf("checked %d enum/default pairs", checked)
}

func TestPathDefaultsAreSentOnTheWire(t *testing.T) {
	// The plug-in reads width with Float and the rest with String and Bool,
	// so each has to arrive with the documented default and JSON type.
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	callTool(t, cs, "draw_path", map[string]any{"d": "M 100 300 C 150 100 350 100 400 300"})

	if got := f.command(); got != "draw_path" {
		t.Errorf("command = %q, want draw_path", got)
	}

	args := f.args()
	for key, want := range map[string]any{
		"d":           "M 100 300 C 150 100 350 100 400 300",
		"width":       float64(2),
		"cap":         "round",
		"join":        "round",
		"antialias":   true,
		"keep_path":   false,
		"image_index": float64(0),
	} {
		if got := args[key]; got != want {
			t.Errorf("params[%q] = %#v, want %#v", key, got, want)
		}
	}

	// No colour means the current foreground, so it goes as null.
	if got, ok := args["color"]; !ok || got != nil {
		t.Errorf("params[\"color\"] = %#v, want null", got)
	}
}

func TestFillPathRequiresColor(t *testing.T) {
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "fill_path", Arguments: map[string]any{"d": "M 200 200 Q 300 50 400 200 Z"},
	})
	if err == nil && !res.IsError {
		t.Fatal("fill_path without color succeeded, want a validation error")
	}
}

func TestSelectionFeatherIsANumber(t *testing.T) {
	// feather is documented as a radius in pixels. The plug-in used to read it
	// as a bool, so the documented value never reached GIMP; it now reads the
	// number, and every select tool sends it the same way.
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	for tool, args := range map[string]map[string]any{
		"select_rectangle":         {"x": 0, "y": 0, "width": 10, "height": 10, "feather": 6},
		"select_ellipse":           {"x": 0, "y": 0, "width": 10, "height": 10, "feather": 6},
		"select_rounded_rectangle": {"x": 0, "y": 0, "width": 10, "height": 10, "feather": 6},
		"select_path":              {"d": "M 0 0 L 10 0 L 10 10 Z", "feather": 6},
		"path_to_selection":        {"path_id": 7, "feather": 6},
	} {
		callTool(t, cs, tool, args)

		if got := f.args()["feather"]; got != float64(6) {
			t.Errorf("%s: params[\"feather\"] = %#v, want 6", tool, got)
		}
	}

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "select_rectangle",
		Arguments: map[string]any{"x": 0, "y": 0, "width": 10, "height": 10, "feather": 5000},
	})
	if err == nil && !res.IsError {
		t.Fatal("feather=5000 succeeded, want a validation error: GIMP's radius stops at 1000")
	}
}

func TestFillOutlineDefaultsAreSentOnTheWire(t *testing.T) {
	// The plug-in sets the line width and join for every outline, so the
	// documented defaults have to arrive with the call, and an unset
	// stroke_color has to arrive as null, meaning no outline.
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"fill_ellipse", map[string]any{"x": 1, "y": 2, "width": 30, "height": 40, "color": "red"}},
		{"fill_rectangle", map[string]any{"x": 1, "y": 2, "width": 30, "height": 40, "color": "red"}},
		{"fill_rounded_rectangle", map[string]any{"x": 1, "y": 2, "width": 30, "height": 40, "color": "red"}},
		{"fill_path", map[string]any{"d": "M 0 0 L 10 0 L 5 9 Z", "color": "red"}},
	} {
		callTool(t, cs, tc.tool, tc.args)

		args := f.args()
		if got, ok := args["stroke_color"]; !ok || got != nil {
			t.Errorf("%s: stroke_color = %#v, want null", tc.tool, got)
		}

		if args["stroke_width"] != float64(2) || args["stroke_join"] != "round" {
			t.Errorf("%s: stroke_width/stroke_join = %#v/%#v, want 2/round",
				tc.tool, args["stroke_width"], args["stroke_join"])
		}
	}
}

func TestDrawPathSendsFill(t *testing.T) {
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	callTool(t, cs, "draw_path", map[string]any{"d": "M 0 0 L 10 0 L 5 9 Z", "fill": "#88cc88"})

	if got := f.args()["fill"]; got != "#88cc88" {
		t.Errorf("fill = %#v, want #88cc88", got)
	}
}

// shapeSchema returns the schema draw_shapes advertises for one shape.
func shapeSchema(t *testing.T, cs *mcp.ClientSession) map[string]any {
	t.Helper()

	items, ok := propertyOf(t, schemaOf(t, cs, "draw_shapes"), "shapes")["items"].(map[string]any)
	if !ok {
		t.Fatal("draw_shapes' shapes has no item schema")
	}

	return items
}

func TestDrawShapesDescribesEachShape(t *testing.T) {
	// The shape's constraints live on a nested struct, which the tag reader
	// used to skip, so the enum and bounds were neither advertised nor
	// checked against GIMP.
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	shape := shapeSchema(t, cs)

	if required, _ := shape["required"].([]any); !slices.Equal(required, []any{"type"}) {
		t.Errorf("shape required = %#v, want only type", shape["required"])
	}

	kinds := propertyOf(t, shape, "type")["enum"]
	if want := []any{"rectangle", "rounded_rectangle", "ellipse", "path"}; !reflect.DeepEqual(kinds, want) {
		t.Errorf("type enum = %#v, want %#v", kinds, want)
	}

	width := propertyOf(t, shape, "stroke_width")
	if width["minimum"] != float64(0) || width["maximum"] != float64(2000) || width["default"] != float64(2) {
		t.Errorf("stroke_width minimum/maximum/default = %#v/%#v/%#v, want 0/2000/2",
			width["minimum"], width["maximum"], width["default"])
	}

	if join := propertyOf(t, shape, "stroke_join"); join["default"] != "round" {
		t.Errorf("stroke_join default = %#v, want round", join["default"])
	}
}

func TestDrawShapesAppliesDefaultsToEachShape(t *testing.T) {
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	callTool(t, cs, "draw_shapes", map[string]any{"shapes": []any{
		map[string]any{"type": "ellipse", "x": 10, "y": 10, "width": 50, "height": 40, "color": "white"},
		map[string]any{"type": "path", "d": "M 0 0 L 9 9", "stroke_color": "black", "stroke_width": 5},
	}})

	shapes, ok := f.args()["shapes"].([]any)
	if !ok || len(shapes) != 2 {
		t.Fatalf("shapes = %#v, want a list of two", f.args()["shapes"])
	}

	first, _ := shapes[0].(map[string]any)
	if first["type"] != "ellipse" || first["stroke_width"] != float64(2) || first["stroke_join"] != "round" {
		t.Errorf("first shape = %#v, want an ellipse with the outline defaults", first)
	}

	second, _ := shapes[1].(map[string]any)
	if second["stroke_width"] != float64(5) {
		t.Errorf("second shape stroke_width = %#v, want the 5 it was given", second["stroke_width"])
	}
}

func TestDrawShapesRefusesBadShapes(t *testing.T) {
	f := &fakeGimp{}
	cs := connect(t, f.start(t))

	for name, shape := range map[string]map[string]any{
		"unknown type":     {"type": "star", "color": "red"},
		"missing type":     {"color": "red"},
		"stroke too wide":  {"type": "path", "d": "M 0 0 L 9 9", "stroke_color": "black", "stroke_width": 3000},
		"unknown join":     {"type": "path", "d": "M 0 0 L 9 9", "stroke_color": "black", "stroke_join": "spiky"},
		"fractional width": {"type": "ellipse", "width": 10.5, "height": 10, "color": "red"},
	} {
		res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
			Name: "draw_shapes", Arguments: map[string]any{"shapes": []any{shape}},
		})
		if err == nil && !res.IsError {
			t.Errorf("%s: draw_shapes succeeded, want a validation error", name)
		}
	}
}
