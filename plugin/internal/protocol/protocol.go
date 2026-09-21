// Package protocol defines the JSON envelope exchanged between the MCP server
// and this plug-in.
//
// One request per connection: the client writes a JSON object, the plug-in
// writes a JSON object back and closes.
package protocol

import "encoding/json"

// Request is a command invocation.
type Request struct {
	Type   string          `json:"type"`
	Params json.RawMessage `json:"params"`
}

// Response is the reply. Status is "success" or "error".
type Response struct {
	Status  string `json:"status"`
	Results any    `json:"results,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Success builds a successful response.
func Success(results any) Response {
	if results == nil {
		results = map[string]any{}
	}

	return Response{Status: "success", Results: results}
}

// Failure builds an error response.
func Failure(err error) Response {
	return Response{Status: "error", Error: err.Error()}
}
