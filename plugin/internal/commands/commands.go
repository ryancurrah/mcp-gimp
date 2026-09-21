// Package commands implements the plug-in's command set on top of the PDB
// bridge.
//
// Each command decodes its JSON params, runs one or more PDB procedures on the
// GIMP main thread and returns a JSON-compatible results value. The shapes
// match the protocol the MCP server expects.
package commands

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
)

// Handler runs one command. It is always invoked on the GIMP main thread.
type Handler func(params Params) (any, error)

// registry holds the command table.
var (
	registryMu sync.RWMutex
	registry   = map[string]Handler{}
)

// register adds a command, panicking on a duplicate so collisions surface at
// start-up rather than at call time.
func register(name string, h Handler) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if _, exists := registry[name]; exists {
		panic("duplicate command: " + name)
	}

	registry[name] = h
}

// Lookup finds a command by name.
func Lookup(name string) (Handler, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	h, ok := registry[name]

	return h, ok
}

// Names lists the registered commands in sorted order.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}

	sort.Strings(out)

	return out
}

// Params wraps the raw JSON params with typed, defaulted accessors.
//
// The MCP server always sends every declared parameter, using null for ones
// the caller left out, so every accessor takes the value to use when the key
// is absent or null.
type Params map[string]json.RawMessage

// Decode parses a params object. A missing or null body yields empty Params.
func Decode(raw json.RawMessage) (Params, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return Params{}, nil
	}

	var p Params
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("params must be an object: %w", err)
	}

	return p, nil
}

// Has reports whether key was supplied with a non-null value.
func (p Params) Has(key string) bool {
	_, ok := p.raw(key)

	return ok
}

// Int reads an integer parameter.
func (p Params) Int(key string, fallback int) int {
	v, ok := p.raw(key)
	if !ok {
		return fallback
	}

	var n float64
	if err := json.Unmarshal(v, &n); err != nil {
		return fallback
	}

	return int(n)
}

// Float reads a floating point parameter.
func (p Params) Float(key string, fallback float64) float64 {
	v, ok := p.raw(key)
	if !ok {
		return fallback
	}

	var n float64
	if err := json.Unmarshal(v, &n); err != nil {
		return fallback
	}

	return n
}

// String reads a string parameter.
func (p Params) String(key, fallback string) string {
	v, ok := p.raw(key)
	if !ok {
		return fallback
	}

	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return fallback
	}

	return s
}

// Bool reads a boolean parameter.
func (p Params) Bool(key string, fallback bool) bool {
	v, ok := p.raw(key)
	if !ok {
		return fallback
	}

	var b bool
	if err := json.Unmarshal(v, &b); err != nil {
		return fallback
	}

	return b
}

// Floats reads an array of numbers, flattening nested arrays so that
// [[x, y], [x, y]] and [x, y, x, y] are both accepted.
func (p Params) Floats(key string) []float64 {
	v, ok := p.raw(key)
	if !ok {
		return nil
	}

	var nested [][]float64
	if err := json.Unmarshal(v, &nested); err == nil {
		out := make([]float64, 0, len(nested)*2)
		for _, pair := range nested {
			out = append(out, pair...)
		}

		return out
	}

	var flat []float64
	if err := json.Unmarshal(v, &flat); err != nil {
		return nil
	}

	return flat
}

// Strings reads an array of strings.
func (p Params) Strings(key string) []string {
	v, ok := p.raw(key)
	if !ok {
		return nil
	}

	var out []string
	if err := json.Unmarshal(v, &out); err != nil {
		return nil
	}

	return out
}

// Object reads a nested object parameter.
func (p Params) Object(key string) Params {
	v, ok := p.raw(key)
	if !ok {
		return nil
	}

	var out Params
	if err := json.Unmarshal(v, &out); err != nil {
		return nil
	}

	return out
}

// Any reads a parameter as a free-form value.
func (p Params) Any(key string) any {
	v, ok := p.raw(key)
	if !ok {
		return nil
	}

	var out any
	if err := json.Unmarshal(v, &out); err != nil {
		return nil
	}

	return out
}

// raw returns the value for key when it is present and not null.
func (p Params) raw(key string) (json.RawMessage, bool) {
	v, ok := p[key]
	if !ok || len(v) == 0 || string(v) == "null" {
		return nil, false
	}

	return v, true
}

// run is shorthand for a PDB call that returns no interesting values.
func run(proc string, args gimpbridge.Args) error {
	_, err := gimpbridge.Run(proc, args)

	return err
}

// run1 is shorthand for a PDB call whose first return value is wanted.
func run1(proc string, args gimpbridge.Args) (gimpbridge.Value, error) {
	out, err := gimpbridge.Run(proc, args)
	if err != nil {
		return nil, err
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("%s returned no value", proc)
	}

	return out[0], nil
}

// objectID coerces a PDB return value to an object id.
func objectID(v gimpbridge.Value) (gimpbridge.ObjectID, error) {
	id, ok := v.(gimpbridge.ObjectID)
	if !ok {
		return 0, fmt.Errorf("expected an object, got %T", v)
	}

	return id, nil
}
