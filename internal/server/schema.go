package server

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// A tool's argument schema is inferred from its input struct. jsonschema.For
// takes each argument's name and type from the field and its description from
// the jsonschema tag, and nothing else, so the rest is declared in tags of
// our own:
//
//	enum:"a,b,c"      the values the argument accepts
//	minimum:"0"       the least value it accepts
//	maximum:"100"     the greatest value it accepts
//
// A constrained argument also says where its constraint comes from, which the
// tests check (see schema_test.go and CONTRIBUTING.md):
//
//	gimp:"gimp:hue-saturation.hue/180"
//	    the GIMP operation property or procedure argument the value is passed
//	    to, as owner.name, and what the plug-in divides it by on the way.
//	    Several are separated by spaces. The constraint is checked against the
//	    snapshot of what GIMP accepts.
//	project:"reason"
//	    a constraint this server chose rather than one GIMP imposes.
//	sentinel:"-1"
//	    a value outside GIMP's range that the plug-in reads as "use GIMP's
//	    default" instead of passing it on.
//
// The schema's default for an argument is whatever SetDefaults assigns it, so
// a default is written once and the schema cannot disagree with it.

// argSpec is what one tool argument declares.
type argSpec struct {
	Name     string
	Enum     []string
	Minimum  *float64
	Maximum  *float64
	Sentinel *float64
	GIMP     []gimpRef
	Project  string
	// Default is the argument's value after SetDefaults, or nil when it has
	// none.
	Default json.RawMessage
	// Nullable is set for pointer arguments, whose schema also admits null.
	Nullable bool
}

// gimpRef names one thing in GIMP an argument is passed to.
type gimpRef struct {
	// Owner is a procedure such as gimp-layer-set-opacity, or an operation
	// such as gegl:vignette. Operations are the names with a colon.
	Owner string
	// Name is the procedure argument or operation property.
	Name string
	// Scale is what the plug-in divides the argument by before passing it on.
	Scale float64
}

// IsOperation reports whether the reference names a GEGL operation rather
// than a PDB procedure.
func (r gimpRef) IsOperation() bool { return strings.Contains(r.Owner, ":") }

func (r gimpRef) String() string {
	s := r.Owner + "." + r.Name
	if r.Scale != 1 {
		s += "/" + strconv.FormatFloat(r.Scale, 'f', -1, 64)
	}

	return s
}

// argSpecs reads the declarations off an input struct, and off the structs
// its arguments hold, so a constraint on a nested field is advertised and
// tested like any other. A nested argument is named by its path: region.x
// for a field of an object, shapes[].type for a field of each list element.
func argSpecs(t reflect.Type) ([]argSpec, error) {
	return argSpecsAt(t, "")
}

func argSpecsAt(t reflect.Type, prefix string) ([]argSpec, error) {
	defaults, err := defaultsOf(t)
	if err != nil {
		return nil, err
	}

	specs := make([]argSpec, 0, t.NumField())

	for f := range t.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}

		spec, err := parseField(f)
		if err != nil {
			return nil, fmt.Errorf("argument %s%s: %w", prefix, name, err)
		}

		spec.Name = prefix + name
		spec.Default = defaults[name]
		specs = append(specs, spec)

		if elem, sep, ok := nestedStruct(f.Type); ok {
			nested, err := argSpecsAt(elem, spec.Name+sep)
			if err != nil {
				return nil, err
			}

			specs = append(specs, nested...)
		}
	}

	return specs, nil
}

// nestedStruct reports the struct an argument holds, directly, through a
// pointer or as the elements of a list, and the separator that joins the
// argument's name to its fields' names.
func nestedStruct(t reflect.Type) (reflect.Type, string, bool) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch {
	case t.Kind() == reflect.Struct:
		return t, ".", true
	case t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Struct:
		return t.Elem(), "[].", true
	default:
		return nil, "", false
	}
}

// property finds the schema of an argument named the way argSpecs names it,
// walking into nested objects and list items.
func property(s *jsonschema.Schema, name string) (*jsonschema.Schema, bool) {
	for {
		head, rest, nested := strings.Cut(name, ".")

		list := strings.HasSuffix(head, "[]")

		prop, ok := s.Properties[strings.TrimSuffix(head, "[]")]
		if !ok {
			return nil, false
		}

		if list {
			if prop = prop.Items; prop == nil {
				return nil, false
			}
		}

		if !nested {
			return prop, true
		}

		s, name = prop, rest
	}
}

func parseField(f reflect.StructField) (argSpec, error) {
	spec := argSpec{
		Project:  f.Tag.Get("project"),
		Nullable: f.Type.Kind() == reflect.Pointer,
	}

	if enum := f.Tag.Get("enum"); enum != "" {
		spec.Enum = strings.Split(enum, ",")
	}

	for _, n := range []struct {
		tag string
		dst **float64
	}{
		{"minimum", &spec.Minimum},
		{"maximum", &spec.Maximum},
		{"sentinel", &spec.Sentinel},
	} {
		raw, ok := f.Tag.Lookup(n.tag)
		if !ok {
			continue
		}

		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return argSpec{}, fmt.Errorf("%s tag %q is not a number", n.tag, raw)
		}

		*n.dst = &v
	}

	for raw := range strings.FieldsSeq(f.Tag.Get("gimp")) {
		ref, err := parseRef(raw)
		if err != nil {
			return argSpec{}, err
		}

		spec.GIMP = append(spec.GIMP, ref)
	}

	return spec, nil
}

// parseRef reads owner.name or owner.name/scale.
func parseRef(raw string) (gimpRef, error) {
	ref := gimpRef{Scale: 1}

	target, scale, scaled := strings.Cut(raw, "/")
	if scaled {
		v, err := strconv.ParseFloat(scale, 64)
		if err != nil || v == 0 {
			return gimpRef{}, fmt.Errorf("gimp tag %q: scale must be a non-zero number", raw)
		}

		ref.Scale = v
	}

	dot := strings.LastIndex(target, ".")
	if dot <= 0 || dot == len(target)-1 {
		return gimpRef{}, fmt.Errorf("gimp tag %q: want owner.name, such as gegl:vignette.radius", raw)
	}

	ref.Owner, ref.Name = target[:dot], target[dot+1:]

	return ref, nil
}

// defaultsOf runs SetDefaults on a zero input and reports each argument it
// changed.
func defaultsOf(t reflect.Type) (map[string]json.RawMessage, error) {
	v := reflect.New(t)

	d, ok := reflect.TypeAssert[defaulter](v)
	if !ok {
		return map[string]json.RawMessage{}, nil
	}

	zero, err := fieldsOf(v.Interface())
	if err != nil {
		return nil, err
	}

	d.SetDefaults()

	set, err := fieldsOf(v.Interface())
	if err != nil {
		return nil, err
	}

	out := map[string]json.RawMessage{}

	for name, raw := range set {
		if string(raw) != string(zero[name]) {
			out[name] = raw
		}
	}

	return out, nil
}

// fieldsOf encodes a value and splits it into its fields.
func fieldsOf(v any) (map[string]json.RawMessage, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode defaults: %w", err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("decode defaults: %w", err)
	}

	return fields, nil
}

// applySpecs puts what the tags declare onto the inferred schema.
func applySpecs(s *jsonschema.Schema, specs []argSpec) error {
	for _, spec := range specs {
		prop, ok := property(s, spec.Name)
		if !ok {
			return fmt.Errorf("no schema property for argument %q", spec.Name)
		}

		prop.Default = spec.Default
		prop.Minimum = spec.Minimum
		prop.Maximum = spec.Maximum

		if len(spec.Enum) > 0 {
			// The SDK panics on a default outside the enum, so it is refused
			// here with a message that names the argument.
			var d string
			if json.Unmarshal(spec.Default, &d) == nil && !slices.Contains(spec.Enum, d) {
				return fmt.Errorf("argument %s: default %q is not in its enum %s",
					spec.Name, d, strings.Join(spec.Enum, ","))
			}

			prop.Enum = make([]any, 0, len(spec.Enum)+1)
			for _, v := range spec.Enum {
				prop.Enum = append(prop.Enum, v)
			}

			// A pointer argument is typed ["null", ...], so null has to stay a
			// legal value or an explicit null would contradict the type.
			if spec.Nullable {
				prop.Enum = append(prop.Enum, nil)
			}
		}
	}

	return nil
}
