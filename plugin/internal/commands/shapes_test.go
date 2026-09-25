package commands

import (
	"errors"
	"strings"
	"testing"
)

func TestObjectsReadsAListOfObjects(t *testing.T) {
	p := decode(t, `{"shapes":[{"type":"ellipse","width":40},{"type":"path"}],"none":null,"number":3}`)

	shapes := p.Objects("shapes")
	if len(shapes) != 2 {
		t.Fatalf("Objects(shapes) has %d entries, want 2", len(shapes))
	}

	if got := shapes[0].String("type", ""); got != "ellipse" {
		t.Errorf("shapes[0].type = %q, want ellipse", got)
	}

	if got := shapes[0].Float("width", 0); got != 40 {
		t.Errorf("shapes[0].width = %v, want 40", got)
	}

	for _, key := range []string{"none", "number", "missing"} {
		if got := p.Objects(key); got != nil {
			t.Errorf("Objects(%s) = %v, want nil", key, got)
		}
	}
}

func TestPaintSpecDefaultsTheOutline(t *testing.T) {
	spec, err := paintSpecFrom(decode(t, `{"color":"red","stroke_color":"black"}`))
	if err != nil {
		t.Fatalf("paintSpecFrom: %v", err)
	}

	if spec.Fill != "red" || spec.Stroke != "black" || spec.StrokeWidth != 2 || spec.Join != "round" {
		t.Errorf("spec = %+v, want red fill, black 2px round outline", spec)
	}
}

func TestPaintSpecRefusesNothingToPaint(t *testing.T) {
	if _, err := paintSpecFrom(decode(t, `{"stroke_width":4}`)); err == nil {
		t.Error("a shape with neither color nor stroke_color was accepted")
	}
}

func TestPaintSpecRefusesATransparentOutline(t *testing.T) {
	// A transparent outline would be read as a colour by GIMP and paint
	// nothing visible, which looks like success.
	for _, stroke := range []string{"transparent", "none"} {
		_, err := paintSpecFrom(decode(t, `{"color":"red","stroke_color":"`+stroke+`"}`))
		if err == nil || !strings.Contains(err.Error(), `color="transparent"`) {
			t.Errorf("stroke_color %q: error = %v, want a pointer to color=\"transparent\"", stroke, err)
		}
	}
}

func TestOnlyTransparentClearsAShape(t *testing.T) {
	for fill, want := range map[string]bool{
		"transparent": true, "Transparent": true, "none": false, "white": false,
	} {
		if got := clearsToAlpha(fill); got != want {
			t.Errorf("clearsToAlpha(%q) = %v, want %v", fill, got, want)
		}
	}
}

func TestCheckShapeAcceptsEachType(t *testing.T) {
	for _, body := range []string{
		`{"type":"rectangle","width":10,"height":10,"color":"red"}`,
		`{"type":"rounded_rectangle","width":10,"height":10,"radius":3,"color":"red"}`,
		`{"type":"ellipse","width":10,"height":10,"stroke_color":"black"}`,
		`{"type":"path","d":"M 0 0 L 10 10","stroke_color":"black"}`,
	} {
		if _, err := checkShape(decode(t, body)); err != nil {
			t.Errorf("checkShape(%s): %v", body, err)
		}
	}
}

func TestCheckShapeRefusesBadShapes(t *testing.T) {
	for body, want := range map[string]string{
		`{"type":"star","color":"red"}`:                           "unknown type",
		`{"color":"red"}`:                                         "unknown type",
		`{"type":"ellipse","width":10,"color":"red"}`:             "width and height",
		`{"type":"path","color":"red"}`:                           "d is required",
		`{"type":"rectangle","width":10,"height":10}`:             "give color",
		`{"type":"path","d":"M 0 0 L 9 9","stroke_color":"none"}`: "cannot erase",
	} {
		_, err := checkShape(decode(t, body))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("checkShape(%s) error = %v, want it to mention %q", body, err, want)
		}
	}
}

func TestShapeErrorNamesIndexAndType(t *testing.T) {
	err := shapeError(2, decode(t, `{"type":"path"}`), errors.New("boom"))
	if got := err.Error(); got != "shape 2 (path): boom" {
		t.Errorf("shapeError = %q, want %q", got, "shape 2 (path): boom")
	}
}

func TestPaintedPathResultReportsWhatPaintingFound(t *testing.T) {
	plain := paintedPathResult(7, false, false, nil)
	if _, ok := plain["bounds"]; ok {
		t.Errorf("result without bounds = %v, want no bounds key", plain)
	}

	if _, ok := plain["alpha_added"]; ok {
		t.Errorf("result without a clear = %v, want no alpha_added key", plain)
	}

	bounds := map[string]any{"x": 1, "y": 2, "width": 3, "height": 4}

	got := paintedPathResult(7, true, true, bounds)
	if got["alpha_added"] != true || got["path_id"] != 7 || got["bounds"] == nil {
		t.Errorf("result = %v, want alpha_added, path_id 7 and bounds", got)
	}
}

func TestDrawShapesReplacesUndo(t *testing.T) {
	if _, ok := Lookup("draw_shapes"); !ok {
		t.Error("command draw_shapes is not registered")
	}

	// GIMP 3 has no procedure that steps the undo stack; the commands only
	// ever failed, so they are gone rather than advertised.
	for _, name := range []string{"undo", "redo"} {
		if _, ok := Lookup(name); ok {
			t.Errorf("command %s is registered, but GIMP 3 cannot step the undo stack", name)
		}
	}
}
