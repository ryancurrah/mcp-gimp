package server

import (
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ryancurrah/mcp-gimp/internal/gimp"
	"github.com/ryancurrah/mcp-gimp/internal/snapshot"
)

// These tests hold the tool definitions to what GIMP accepts. They read the
// committed snapshot, so they need no running GIMP and run in CI; `make
// introspect` refreshes the snapshot from an installed GIMP. See
// CONTRIBUTING.md.

const (
	snapshotPath = "../snapshot/gimp.json"
	pluginSource = "../../plugin/internal/commands"
)

// unbounded is where a GParamSpec's range stops meaning anything: G_MAXINT,
// G_MAXDOUBLE and their negatives are how GIMP says "no limit".
const unbounded = math.MaxInt32

// registered builds every tool and returns what each declares.
func registered(t *testing.T) []registeredTool {
	t.Helper()

	r := &registrar{srv: mcp.NewServer(&mcp.Implementation{Name: "test"}, nil), client: gimp.New()}
	registerTools(r)

	if len(r.errs) > 0 {
		t.Fatalf("register tools: %v", r.errs)
	}

	return r.tools
}

func loadSnapshot(t *testing.T) *snapshot.Snapshot {
	t.Helper()

	snap, err := snapshot.Load(snapshotPath)
	if err != nil {
		t.Fatalf("%v (run `make introspect` with GIMP running)", err)
	}

	return snap
}

// TestSnapshotMatchesThePlugin fails when the plug-in calls something the
// snapshot does not describe, or the snapshot describes something the plug-in
// no longer calls. Either way the snapshot is stale and must be retaken.
func TestSnapshotMatchesThePlugin(t *testing.T) {
	snap := loadSnapshot(t)

	procs, ops, err := snapshot.References(pluginSource)
	if err != nil {
		t.Fatal(err)
	}

	compare := func(kind string, used []string, described map[string][]snapshot.Property) {
		for _, name := range used {
			if _, ok := described[name]; !ok {
				t.Errorf("the plug-in calls %s %s, which the snapshot of GIMP %s does not describe; "+
					"run `make introspect`, and if GIMP lacks it, stop calling it", kind, name, snap.GIMP)
			}
		}

		for name := range described {
			if !slices.Contains(used, name) {
				t.Errorf("the snapshot describes %s %s, which the plug-in no longer calls; run `make introspect`",
					kind, name)
			}
		}
	}

	compare(snapshot.KindProcedure, procs, snap.Procedures)
	compare(snapshot.KindOperation, ops, snap.Operations)
}

// TestConstraintsHaveASource fails on any enum or bound that does not say
// where it came from. A constraint either mirrors something GIMP declares, and
// names it so it can be checked, or is this server's own choice, and says why.
func TestConstraintsHaveASource(t *testing.T) {
	for _, tool := range registered(t) {
		for _, p := range tool.args {
			constrained := len(p.Enum) > 0 || p.Minimum != nil || p.Maximum != nil
			if constrained && len(p.GIMP) == 0 && p.Project == "" {
				t.Errorf("%s.%s is constrained but names no source: add a gimp tag, "+
					"or a project tag with the reason it is this server's choice", tool.name, p.Name)
			}
		}
	}
}

// TestLayerTargetsTakeALayerID fails on a tool that names its layer only by
// name or position. Every tool that creates a layer reports its layer_id, and
// a text layer's name changes with its text, so a caller holding the id must
// be able to use it.
func TestLayerTargetsTakeALayerID(t *testing.T) {
	for _, tool := range registered(t) {
		var byName, byID bool

		for _, p := range tool.args {
			switch p.Name {
			case "layer_name", "layer_index", "old_name":
				byName = true
			case "layer_id":
				byID = true
			}
		}

		if byName && !byID {
			t.Errorf("%s identifies a layer by name or index but takes no layer_id", tool.name)
		}
	}
}

// TestConstraintsMatchGIMP checks every argument that names what it is passed
// to against what the snapshotted GIMP declares for it.
func TestConstraintsMatchGIMP(t *testing.T) {
	snap := loadSnapshot(t)

	procs, ops, err := snapshot.References(pluginSource)
	if err != nil {
		t.Fatal(err)
	}

	for _, tool := range registered(t) {
		for _, p := range tool.args {
			for _, ref := range p.GIMP {
				where := fmt.Sprintf("%s.%s -> %s", tool.name, p.Name, ref)

				kind := snapshot.KindProcedure
				if ref.IsOperation() {
					kind = snapshot.KindOperation
				}

				// A reference to something the plug-in never calls would be
				// checked against GIMP and prove nothing about the tool.
				if used := slices.Contains(procs, ref.Owner) || slices.Contains(ops, ref.Owner); !used {
					t.Errorf("%s: the plug-in does not call %s", where, ref.Owner)

					continue
				}

				prop, err := snap.Property(kind, ref.Owner, ref.Name)
				if err != nil {
					t.Errorf("%s: %v", where, err)

					continue
				}

				for _, problem := range compareToGIMP(p, ref.Scale, prop) {
					t.Errorf("%s: %s", where, problem)
				}
			}
		}
	}
}

// compareToGIMP lists every way an argument's schema disagrees with the GIMP
// property it is passed to.
//
// A tool may accept less than GIMP does - that is a choice - but never more,
// because GObject discards an out-of-range value and the operation runs with
// its own default instead. Where GIMP bounds a value the tool must say so too,
// or a caller has no way to know.
func compareToGIMP(p argSpec, scale float64, prop snapshot.Property) []string {
	var problems []string

	if len(p.Enum) > 0 {
		if len(prop.Choices) == 0 {
			problems = append(problems, fmt.Sprintf("the tool lists values but GIMP's %s takes none by name", prop.Type))
		}

		for _, v := range p.Enum {
			if len(prop.Choices) > 0 && !slices.Contains(prop.Choices, v) {
				problems = append(problems, fmt.Sprintf("%v is not one of GIMP's choices %s",
					v, strings.Join(prop.Choices, ", ")))
			}
		}
	}

	d := string(p.Default)

	if len(prop.Choices) > 0 {
		if s, err := strconv.Unquote(d); err == nil && !slices.Contains(prop.Choices, s) {
			problems = append(problems, fmt.Sprintf("default %q is not one of GIMP's choices %s",
				s, strings.Join(prop.Choices, ", ")))
		}
	}

	problems = append(problems, compareBound("minimum", p.Minimum, prop.Min, scale, p.Sentinel, -1)...)
	problems = append(problems, compareBound("maximum", p.Maximum, prop.Max, scale, p.Sentinel, 1)...)

	if d != "" {
		if v, err := strconv.ParseFloat(d, 64); err == nil && (p.Sentinel == nil || v != *p.Sentinel) {
			if g := v / scale; (prop.Min != nil && g < *prop.Min-tolerance(*prop.Min)) ||
				(prop.Max != nil && g > *prop.Max+tolerance(*prop.Max)) {
				problems = append(problems, fmt.Sprintf("default %s reaches GIMP as %s, outside %s..%s",
					d, formatFloat(g), bound(prop.Min), bound(prop.Max)))
			}
		}
	}

	return problems
}

// compareBound checks one side of a numeric range. side is -1 for the
// minimum and 1 for the maximum.
func compareBound(label string, tool, gimp *float64, scale float64, sentinel *float64, side float64) []string {
	if gimp == nil || math.Abs(*gimp) >= unbounded {
		return nil
	}

	if tool == nil {
		return []string{fmt.Sprintf("GIMP's %s is %s but the tool declares none; add %s:%q",
			label, formatFloat(*gimp), label, formatFloat(*gimp*scale))}
	}

	// A sentinel sits outside GIMP's range on purpose and never reaches it.
	if sentinel != nil && *tool == *sentinel {
		return nil
	}

	if reaches := *tool / scale; side*(reaches-*gimp) > tolerance(*gimp) {
		return []string{fmt.Sprintf("the tool's %s %s reaches GIMP as %s, beyond GIMP's %s %s",
			label, formatFloat(*tool), formatFloat(reaches), label, formatFloat(*gimp))}
	}

	return nil
}

// formatFloat renders a number without a trailing ".0".
func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// bound renders one side of a range, which GIMP may leave open.
func bound(v *float64) string {
	if v == nil {
		return "any"
	}

	return formatFloat(*v)
}

// tolerance absorbs the rounding in scaled bounds such as 127/127.
func tolerance(v float64) float64 {
	return 1e-9 * math.Max(1, math.Abs(v))
}

// TestCompareToGIMPCatchesKnownDefects replays defects the check exists for,
// each of which reached GIMP silently before it did.
func TestCompareToGIMPCatchesKnownDefects(t *testing.T) {
	f := func(v float64) *float64 { return &v }

	unitRange := snapshot.Property{Type: "gdouble", Min: f(0), Max: f(1), Default: "0.5"}

	for _, tc := range []struct {
		name  string
		p     argSpec
		scale float64
		prop  snapshot.Property
		want  string
	}{
		{
			name:  "drop shadow opacity passed as a percentage",
			p:     argSpec{Default: []byte("60"), Minimum: f(0), Maximum: f(100)},
			scale: 1, prop: unitRange, want: "beyond GIMP's maximum 1",
		},
		{
			name:  "desaturate default that GIMP does not name",
			p:     argSpec{Default: []byte(`"luminosity"`), Enum: []string{"luminosity"}},
			scale: 1,
			prop:  snapshot.Property{Type: "GimpDesaturateMode", Choices: []string{"luminance", "luma"}},
			want:  "not one of GIMP's choices",
		},
		{
			name:  "a bound GIMP declares and the tool omits",
			p:     argSpec{Maximum: f(100)},
			scale: 1,
			prop:  snapshot.Property{Type: "gint", Min: f(1), Max: f(100)},
			want:  `add minimum:"1"`,
		},
		{
			name:  "values listed for a property that takes none by name",
			p:     argSpec{Enum: []string{"a"}},
			scale: 1, prop: unitRange, want: "takes none by name",
		},
	} {
		problems := strings.Join(compareToGIMP(tc.p, tc.scale, tc.prop), "; ")
		if !strings.Contains(problems, tc.want) {
			t.Errorf("%s: got %q, want a problem containing %q", tc.name, problems, tc.want)
		}
	}
}

// TestCompareToGIMPAcceptsDeliberateChoices covers what the check must allow:
// a narrower range, a scaled one, and a sentinel.
func TestCompareToGIMPAcceptsDeliberateChoices(t *testing.T) {
	f := func(v float64) *float64 { return &v }

	for _, tc := range []struct {
		name  string
		p     argSpec
		scale float64
		prop  snapshot.Property
	}{
		{
			name:  "narrower than GIMP",
			p:     argSpec{Minimum: f(0), Maximum: f(3)},
			scale: 1, prop: snapshot.Property{Min: f(0), Max: f(math.MaxFloat64)},
		},
		{
			name:  "brightness in the dialog's units",
			p:     argSpec{Minimum: f(-127), Maximum: f(127)},
			scale: 127, prop: snapshot.Property{Min: f(-1), Max: f(1)},
		},
		{
			name:  "-1 meaning use GIMP's default",
			p:     argSpec{Default: []byte("9"), Minimum: f(-1), Maximum: f(9), Sentinel: f(-1)},
			scale: 1, prop: snapshot.Property{Min: f(0), Max: f(9)},
		},
	} {
		if problems := compareToGIMP(tc.p, tc.scale, tc.prop); len(problems) > 0 {
			t.Errorf("%s: unexpected problems %v", tc.name, problems)
		}
	}
}

// nestedOuter and nestedInner stand in for a tool whose argument is a list of
// objects, like draw_shapes.
type nestedOuter struct {
	Items []nestedInner `json:"items" jsonschema:"the list"`
}

type nestedInner struct {
	Kind  string   `json:"kind" jsonschema:"what it is" enum:"a,b"`
	Width *float64 `json:"width,omitempty" jsonschema:"how wide" minimum:"0" maximum:"10" project:"a test bound"`
}

func (in *nestedInner) SetDefaults() {
	if in.Width == nil {
		in.Width = ptr(3.0)
	}
}

func TestNestedArgumentsAreDeclared(t *testing.T) {
	// A constraint on a list element's field has to reach the specs, or
	// TestConstraintsHaveASource and TestConstraintsMatchGIMP never see it.
	specs, err := argSpecs(reflect.TypeFor[nestedOuter]())
	if err != nil {
		t.Fatalf("argSpecs: %v", err)
	}

	byName := map[string]argSpec{}
	for _, s := range specs {
		byName[s.Name] = s
	}

	kind, ok := byName["items[].kind"]
	if !ok {
		t.Fatalf("no spec for items[].kind; got %v", slices.Collect(maps.Keys(byName)))
	}

	if !slices.Equal(kind.Enum, []string{"a", "b"}) || len(kind.GIMP) != 0 || kind.Project != "" {
		t.Errorf("items[].kind = %+v, want enum a,b with no source, for the source check to catch", kind)
	}

	if width := byName["items[].width"]; string(width.Default) != "3" {
		t.Errorf("items[].width default = %s, want 3 from the element's SetDefaults", width.Default)
	}
}

func TestNestedConstraintsAreAdvertised(t *testing.T) {
	s, err := jsonschema.For[nestedOuter](nil)
	if err != nil {
		t.Fatalf("infer: %v", err)
	}

	specs, err := argSpecs(reflect.TypeFor[nestedOuter]())
	if err != nil {
		t.Fatalf("argSpecs: %v", err)
	}

	if err := applySpecs(s, specs); err != nil {
		t.Fatalf("applySpecs: %v", err)
	}

	width := s.Properties["items"].Items.Properties["width"]
	if width.Maximum == nil || *width.Maximum != 10 || string(width.Default) != "3" {
		t.Errorf("items[].width schema = max %v default %s, want max 10 default 3", width.Maximum, width.Default)
	}

	if kind := s.Properties["items"].Items.Properties["kind"]; len(kind.Enum) != 2 {
		t.Errorf("items[].kind enum = %v, want a,b", kind.Enum)
	}
}
