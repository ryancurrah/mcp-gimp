package protocol_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/protocol"
)

func TestSuccessAlwaysCarriesAnObject(t *testing.T) {
	// The MCP server reads results unconditionally, so a command that
	// returns nothing still has to produce an object.
	body, err := json.Marshal(protocol.Success(nil))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got["status"] != "success" {
		t.Errorf("status = %v, want success", got["status"])
	}

	if _, ok := got["results"].(map[string]any); !ok {
		t.Errorf("results = %#v, want an object", got["results"])
	}
}

func TestFailureCarriesTheMessage(t *testing.T) {
	resp := protocol.Failure(errors.New("no image is open"))

	if resp.Status != "error" {
		t.Errorf("status = %q, want error", resp.Status)
	}

	if resp.Error != "no image is open" {
		t.Errorf("error = %q, want the message", resp.Error)
	}
}

func TestRequestDecodesRawParams(t *testing.T) {
	var req protocol.Request

	if err := json.Unmarshal([]byte(`{"type":"new_canvas","params":{"width":10}}`), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if req.Type != "new_canvas" {
		t.Errorf("type = %q, want new_canvas", req.Type)
	}

	if string(req.Params) != `{"width":10}` {
		t.Errorf("params = %s, want the raw object", req.Params)
	}
}
