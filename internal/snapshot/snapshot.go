// Package snapshot records what the installed GIMP accepts, so the tool
// definitions can be checked against GIMP itself rather than against prose.
//
// A snapshot is read off a running GIMP by `make introspect` and committed as
// internal/snapshot/gimp.json. It covers every PDB procedure and GEGL operation the
// plug-in calls, with each argument's type, range, default and permitted
// values. Upgrading GIMP and re-running introspect turns every change GIMP
// made into a reviewable diff, and the server's tests fail on any tool
// definition the new GIMP no longer honours.
package snapshot

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Snapshot is what one GIMP build accepts.
type Snapshot struct {
	// GIMP is the version the snapshot was read from.
	GIMP string `json:"gimp"`
	// Procedures maps a PDB procedure name to its arguments.
	Procedures map[string][]Property `json:"procedures"`
	// Operations maps a GEGL operation name to the properties GIMP's filter
	// configuration exposes for it.
	Operations map[string][]Property `json:"operations"`
}

// Property is one argument or property, as GIMP declares it.
type Property struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Min     *float64 `json:"minimum,omitempty"`
	Max     *float64 `json:"maximum,omitempty"`
	Default string   `json:"default,omitempty"`
	Choices []string `json:"choices,omitempty"`
	Blurb   string   `json:"blurb,omitempty"`
}

// Load reads a snapshot file.
func Load(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the committed snapshot
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}

	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse snapshot %s: %w", path, err)
	}

	return &s, nil
}

// Property finds one argument of a procedure or property of an operation.
func (s *Snapshot) Property(kind, owner, name string) (Property, error) {
	table := s.Procedures
	if kind == KindOperation {
		table = s.Operations
	}

	props, ok := table[owner]
	if !ok {
		return Property{}, fmt.Errorf("GIMP %s has no %s %s", s.GIMP, kind, owner)
	}

	for _, p := range props {
		if p.Name == name {
			return p, nil
		}
	}

	return Property{}, fmt.Errorf("GIMP %s: %s %s has no argument %q", s.GIMP, kind, owner, name)
}

// Kinds of GIMP entry a snapshot records.
const (
	KindProcedure = "procedure"
	KindOperation = "operation"
)

var (
	// procedureName matches a PDB procedure name used as a string literal.
	procedureName = regexp.MustCompile(`^(gimp|file|plug-in)(-[a-z0-9]+)+$`)
	// operationName matches a GEGL operation name.
	operationName = regexp.MustCompile(`^(gegl|gimp):[a-z0-9]+(-[a-z0-9]+)*$`)
)

// References lists the procedures and operations the plug-in's source names.
//
// It reads the string literals in dir rather than a hand-kept list, so a
// procedure the plug-in starts calling is covered by the next snapshot without
// anyone having to remember to add it.
func References(dir string) (procedures, operations []string, err error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, nil, fmt.Errorf("list %s: %w", dir, err)
	}

	procs, ops := map[string]bool{}, map[string]bool{}
	fset := token.NewFileSet()

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		f, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, nil, fmt.Errorf("parse %s: %w", file, err)
		}

		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}

			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}

			switch {
			case operationName.MatchString(s):
				ops[s] = true
			case procedureName.MatchString(s):
				procs[s] = true
			}

			return true
		})
	}

	return sorted(procs), sorted(ops), nil
}

func sorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}
