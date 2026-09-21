package commands

import "encoding/json"

// jsonUnmarshal is a thin alias so callers in this package do not need to
// import encoding/json directly.
func jsonUnmarshal(data json.RawMessage, v any) error {
	return json.Unmarshal(data, v)
}
