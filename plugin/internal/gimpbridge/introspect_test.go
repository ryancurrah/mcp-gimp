package gimpbridge

import "testing"

func TestParsePropertiesReadsTheTable(t *testing.T) {
	// The bridge emits one tab-separated line per property; a property with no
	// numeric bounds leaves those fields empty rather than omitting them.
	table := "softness\tgdouble\t0\t1\t0.8\t\tWidth of the fade\n" +
		"shape\tgchararray\t\t\tcircle\tcircle,square,diamond\tVignette shape\n"

	props := parseProperties(table)
	if len(props) != 2 {
		t.Fatalf("got %d properties, want 2", len(props))
	}

	soft := props[0]
	if soft.Min == nil || *soft.Min != 0 || soft.Max == nil || *soft.Max != 1 {
		t.Errorf("softness bounds = %v..%v, want 0..1", soft.Min, soft.Max)
	}

	if soft.Choices != nil {
		t.Errorf("softness choices = %v, want none", soft.Choices)
	}

	shape := props[1]
	if shape.Min != nil || shape.Max != nil {
		t.Errorf("shape has numeric bounds %v..%v, want none", shape.Min, shape.Max)
	}

	if len(shape.Choices) != 3 || shape.Choices[0] != "circle" {
		t.Errorf("shape choices = %v, want the three named", shape.Choices)
	}

	if shape.Default != "circle" {
		t.Errorf("shape default = %q, want circle", shape.Default)
	}
}

func TestParsePropertiesSkipsMalformedLines(t *testing.T) {
	if got := parseProperties("too\tfew\tfields\n\n"); len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}
