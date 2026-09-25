package commands

import (
	"encoding/json"
	"slices"
	"testing"
)

// decode builds Params from a JSON object literal.
func decode(t *testing.T, body string) Params {
	t.Helper()

	p, err := Decode(json.RawMessage(body))
	if err != nil {
		t.Fatalf("Decode(%s): %v", body, err)
	}

	return p
}

func TestNullIsTreatedAsAbsent(t *testing.T) {
	// The MCP server sends every declared parameter, using null for the ones
	// the caller omitted, so null has to mean "use the default".
	p := decode(t, `{"layer_name":null,"quality":null,"flatten":null}`)

	if got := p.String("layer_name", "fallback"); got != "fallback" {
		t.Errorf("String = %q, want the fallback", got)
	}

	if got := p.Int("quality", 90); got != 90 {
		t.Errorf("Int = %d, want the fallback", got)
	}

	if !p.Bool("flatten", true) {
		t.Error("Bool did not fall back to true")
	}

	if p.Has("layer_name") {
		t.Error("Has reported a null parameter as supplied")
	}
}

func TestExplicitZeroIsKept(t *testing.T) {
	// A caller asking for quality 0 must not be given the default instead.
	p := decode(t, `{"quality":0,"flatten":false,"name":""}`)

	if got := p.Int("quality", 90); got != 0 {
		t.Errorf("Int = %d, want 0", got)
	}

	if p.Bool("flatten", true) {
		t.Error("Bool = true, want the explicit false")
	}

	if got := p.String("name", "Layer"); got != "" {
		t.Errorf("String = %q, want the explicit empty string", got)
	}

	if !p.Has("quality") {
		t.Error("Has did not see an explicitly supplied zero")
	}
}

func TestMissingKeysUseFallbacks(t *testing.T) {
	p := decode(t, `{}`)

	if got := p.Float("radius", 5); got != 5 {
		t.Errorf("Float = %v, want 5", got)
	}

	if got := p.Int("image_index", 0); got != 0 {
		t.Errorf("Int = %d, want 0", got)
	}
}

func TestEmptyParamsDecode(t *testing.T) {
	for _, body := range []string{"", "null"} {
		p, err := Decode(json.RawMessage(body))
		if err != nil {
			t.Fatalf("Decode(%q): %v", body, err)
		}

		if len(p) != 0 {
			t.Errorf("Decode(%q) = %v, want empty", body, p)
		}
	}
}

func TestNonObjectParamsAreRejected(t *testing.T) {
	if _, err := Decode(json.RawMessage(`[1,2,3]`)); err == nil {
		t.Fatal("Decode accepted a list, want an error")
	}
}

func TestFloatsFlattensPairs(t *testing.T) {
	// Curve control points arrive as [[x,y],...] but GIMP wants a flat list.
	p := decode(t, `{"points":[[0,0],[0.5,0.6],[1,1]]}`)

	got := p.Floats("points")
	want := []float64{0, 0, 0.5, 0.6, 1, 1}

	if len(got) != len(want) {
		t.Fatalf("Floats = %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Floats = %v, want %v", got, want)
		}
	}
}

func TestFloatsAcceptsFlatList(t *testing.T) {
	p := decode(t, `{"strokes":[0,0,10,10]}`)

	if got := p.Floats("strokes"); len(got) != 4 {
		t.Errorf("Floats = %v, want four values", got)
	}
}

func TestStringFallbackChaining(t *testing.T) {
	// Several tools accept an alias, expressed as a chained fallback.
	p := decode(t, `{"start_color":"#ffffff","color1":null}`)

	if got := p.String("color1", p.String("start_color", "")); got != "#ffffff" {
		t.Errorf("chained fallback = %q, want #ffffff", got)
	}
}

func TestEveryCommandIsRegisteredOnce(t *testing.T) {
	names := Names()
	if len(names) < 70 {
		t.Errorf("only %d commands registered, want the full set", len(names))
	}

	seen := make(map[string]bool, len(names))

	for _, n := range names {
		if seen[n] {
			t.Errorf("command %s registered twice", n)
		}

		seen[n] = true

		if _, ok := Lookup(n); !ok {
			t.Errorf("command %s is listed but cannot be looked up", n)
		}
	}
}

func TestLookupRejectsUnknown(t *testing.T) {
	if _, ok := Lookup("no_such_command"); ok {
		t.Error("Lookup found a command that does not exist")
	}
}

func TestFitTargetPreservesAspect(t *testing.T) {
	tests := []struct {
		name                  string
		width, height         int
		maintain              bool
		wantWidth, wantHeight int
	}{
		{"width only", 200, 0, true, 200, 100},
		{"height only", 0, 100, true, 200, 100},
		{"box fits inside", 200, 200, true, 200, 100},
		{"exact when not maintaining", 200, 200, false, 200, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Source is 400x200, a 2:1 image.
			got := fitTarget(tt.width, tt.height, 400, 200, tt.maintain)
			if got[0] != tt.wantWidth || got[1] != tt.wantHeight {
				t.Errorf("fitTarget = %dx%d, want %dx%d",
					got[0], got[1], tt.wantWidth, tt.wantHeight)
			}
		})
	}
}

func TestAnchorOffsetPlacesTheOldCanvas(t *testing.T) {
	// Growing 100x100 to 200x200 leaves 100px of slack on each axis.
	tests := map[string][2]int{
		"top-left":     {0, 0},
		"center":       {50, 50},
		"bottom-right": {100, 100},
		"top":          {50, 0},
		"left":         {0, 50},
	}

	for anchor, want := range tests {
		t.Run(anchor, func(t *testing.T) {
			x, y := anchorOffset(anchor, 100, 100, 200, 200)
			if x != want[0] || y != want[1] {
				t.Errorf("anchorOffset(%s) = %d,%d want %d,%d", anchor, x, y, want[0], want[1])
			}
		})
	}
}

func TestScaleOtherKeepsProportion(t *testing.T) {
	if got := scaleOther(200, 400, 300); got != 150 {
		t.Errorf("scaleOther = %d, want 150", got)
	}

	// A zero original must not divide by zero.
	if got := scaleOther(200, 0, 300); got != 300 {
		t.Errorf("scaleOther with zero original = %d, want the untouched value", got)
	}
}

func TestIconSizesArePlatformSpecific(t *testing.T) {
	if len(iconSizes("ios")) == len(iconSizes("android")) {
		t.Error("ios and android returned the same number of sizes")
	}

	if len(iconSizes("nonsense")) == 0 {
		t.Error("an unknown platform returned no sizes")
	}
}

func TestBaseTypeForKnowsAlpha(t *testing.T) {
	if _, alpha, err := baseTypeFor("RGBA"); err != nil || !alpha {
		t.Errorf("RGBA: alpha=%v err=%v, want alpha", alpha, err)
	}

	if _, alpha, err := baseTypeFor("RGB"); err != nil || alpha {
		t.Errorf("RGB: alpha=%v err=%v, want no alpha", alpha, err)
	}

	if _, _, err := baseTypeFor("CMYK"); err == nil {
		t.Error("baseTypeFor accepted an unsupported mode")
	}
}

func TestLayerTypeForNamesGimpImageType(t *testing.T) {
	// Layer types are passed to GIMP by name, so these must be the nicks
	// GimpImageType declares rather than GIMP's numbering.
	for _, tc := range []struct {
		base  string
		alpha bool
		want  string
	}{
		{"rgb", false, "rgb-image"},
		{"rgb", true, "rgba-image"},
		{"gray", true, "graya-image"},
		{"indexed", false, "indexed-image"},
	} {
		if got := layerTypeFor(tc.base, tc.alpha); got != tc.want {
			t.Errorf("layerTypeFor(%q, %v) = %q, want %q", tc.base, tc.alpha, got, tc.want)
		}
	}
}

func TestAlignOffset(t *testing.T) {
	// Text is placed after it is rendered, so the alignment arithmetic is what
	// decides where a caption lands on a card.
	const imageWidth, layerWidth = 600, 284

	for _, tc := range []struct {
		align string
		x     int
		want  int
	}{
		{align: "left", x: 25, want: 25},
		{align: "", x: 25, want: 25},
		{align: "center", x: 999, want: 158},
		{align: "centre", x: 0, want: 158},
		{align: "right", x: 0, want: 316},
		{align: "right", x: 25, want: 291},
	} {
		got, err := alignOffset(tc.align, imageWidth, layerWidth, tc.x)
		if err != nil {
			t.Fatalf("alignOffset(%q, %d): %v", tc.align, tc.x, err)
		}

		if got != tc.want {
			t.Errorf("alignOffset(%q, x=%d) = %d, want %d", tc.align, tc.x, got, tc.want)
		}
	}
}

func TestAlignOffsetRejectsUnknownAlignment(t *testing.T) {
	// A misspelled alignment must not silently place the layer at zero.
	if _, err := alignOffset("centered", 600, 284, 0); err == nil {
		t.Error("alignOffset accepted an unknown alignment")
	}
}

func TestIsTransparentFill(t *testing.T) {
	for name, want := range map[string]bool{
		"transparent": true,
		"none":        true,
		"TRANSPARENT": true,
		"None":        true,
		"white":       false,
		"#ffffff":     false,
		"":            false,
	} {
		if got := isTransparentFill(name); got != want {
			t.Errorf("isTransparentFill(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestRoundedRectangleCommandsAreRegistered(t *testing.T) {
	for _, name := range []string{
		"fill_rounded_rectangle",
		"draw_rounded_rectangle",
		"select_rounded_rectangle",
	} {
		if _, ok := Lookup(name); !ok {
			t.Errorf("command %s is not registered", name)
		}
	}
}

func TestShapeSelectArgsAddsCornerRadiiOnlyForRoundedRectangles(t *testing.T) {
	p := decode(t, `{"x":10,"y":20,"width":100,"height":50,"radius":8}`)

	plain := shapeSelectArgs(p, "gimp-image-select-rectangle", 1)
	if _, ok := plain["corner-radius-x"]; ok {
		t.Error("a plain rectangle was given a corner radius")
	}

	round := shapeSelectArgs(p, selectRoundRectangleProc, 1)
	if round["corner-radius-x"] != 8.0 || round["corner-radius-y"] != 8.0 {
		t.Errorf("corner radii = %v/%v, want 8/8",
			round["corner-radius-x"], round["corner-radius-y"])
	}

	// The shared geometry must survive either way.
	if plain["x"] != 10.0 || plain["width"] != 100.0 {
		t.Errorf("geometry = %v,%v want 10,100", plain["x"], plain["width"])
	}
}

func TestShapeSelectArgsPrefersPerAxisRadii(t *testing.T) {
	p := decode(t, `{"radius":8,"radius_x":20,"radius_y":4}`)

	args := shapeSelectArgs(p, selectRoundRectangleProc, 1)
	if args["corner-radius-x"] != 20.0 || args["corner-radius-y"] != 4.0 {
		t.Errorf("corner radii = %v/%v, want 20/4",
			args["corner-radius-x"], args["corner-radius-y"])
	}
}

func TestShadowOpacityConvertsToGeglRange(t *testing.T) {
	// The documented default of 60 used to reach GEGL unchanged, where it was
	// above the property maximum and so discarded in favour of GEGL's own 0.5.
	tests := map[float64]float64{0: 0, 60: 0.6, 100: 1, 250: 1, -5: 0}

	for in, want := range tests {
		if got := shadowOpacity(in); got != want {
			t.Errorf("shadowOpacity(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestVignetteShapesMatchTheOperation(t *testing.T) {
	// The tool's enum and this table have to stay in step; a shape the
	// operation does not know is rejected rather than silently ignored.
	want := []string{"circle", "square", "diamond", "horizontal", "vertical"}

	if !slices.Equal(vignetteShapes, want) {
		t.Errorf("vignetteShapes = %v, want %v", vignetteShapes, want)
	}
}

func TestUnitScaleReachesTheFullRange(t *testing.T) {
	// The documented endpoint has to reach the filter's endpoint; dividing
	// -127..127 by 254 reached only half of it.
	tests := []struct{ v, full, want float64 }{
		{127, 127, 1}, {-127, 127, -1}, {0, 127, 0}, {40, 127, 40.0 / 127.0},
		{180, 180, 1}, {100, 100, 1},
		{500, 127, 1}, {-500, 127, -1}, // clamped, never discarded
	}

	for _, tc := range tests {
		if got := unitScale(tc.v, tc.full); got != tc.want {
			t.Errorf("unitScale(%v, %v) = %v, want %v", tc.v, tc.full, got, tc.want)
		}
	}
}

func TestExportFormatFollowsTheExtension(t *testing.T) {
	for _, tc := range []struct {
		path, want, format string
		fails              bool
	}{
		{path: "/tmp/a.png", format: "png"},
		{path: "/tmp/a.JPG", format: "jpeg"},
		{path: "/tmp/a.jpeg", want: "jpeg", format: "jpeg"},
		{path: "/tmp/a.tif", want: "tiff", format: "tiff"},
		{path: "/tmp/a.webp", format: "webp"},
		// A named format that the extension contradicts would be written as
		// the extension's format, so it is refused.
		{path: "/tmp/a.png", want: "jpeg", fails: true},
		{path: "/tmp/a.bmp", fails: true},
		{path: "/tmp/a", fails: true},
	} {
		got, err := exportFormat(tc.path, tc.want)
		if tc.fails {
			if err == nil {
				t.Errorf("exportFormat(%q, %q) = %q, want an error", tc.path, tc.want, got)
			}

			continue
		}

		if err != nil || got != tc.format {
			t.Errorf("exportFormat(%q, %q) = %q, %v; want %q", tc.path, tc.want, got, err, tc.format)
		}
	}
}

func TestBaseTypeNameUsesTheEnumNick(t *testing.T) {
	// The bridge returns GimpImageBaseType by nick. It used to arrive as a
	// string the number switch could not read, so every image was "RGB".
	for v, want := range map[any]string{
		"rgb": "RGB", "gray": "GRAY", "indexed": "INDEXED", "": "UNKNOWN", int64(1): "UNKNOWN",
	} {
		if got := baseTypeName(v); got != want {
			t.Errorf("baseTypeName(%#v) = %q, want %q", v, got, want)
		}
	}
}
