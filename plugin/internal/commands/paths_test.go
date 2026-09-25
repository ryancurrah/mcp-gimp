package commands

import (
	"strings"
	"testing"
)

func TestSVGDocumentWrapsPathData(t *testing.T) {
	// GIMP wants a complete SVG document, so the path data goes into a
	// one-element document as-is.
	d := "M 100 300 C 150 100 350 100 400 300"

	got, err := svgDocument(d)
	if err != nil {
		t.Fatalf("svgDocument(%q): %v", d, err)
	}

	want := `<svg xmlns="http://www.w3.org/2000/svg"><path d="` + d + `"/></svg>`
	if got != want {
		t.Errorf("svgDocument(%q) = %q, want %q", d, got, want)
	}
}

func TestSVGDocumentAcceptsRelativeCommandsAndArcs(t *testing.T) {
	for _, d := range []string{
		"m 10 10 l 50 0 l 0 50 z",
		"M 50 50 L 150 50 A 50 50 0 0 1 150 150 Z",
		"M200,200 Q300,50 400,200Z",
	} {
		if _, err := svgDocument(d); err != nil {
			t.Errorf("svgDocument(%q): %v", d, err)
		}
	}
}

func TestSVGDocumentRejectsEmptyPathData(t *testing.T) {
	for _, d := range []string{"", "   ", "\n"} {
		if _, err := svgDocument(d); err == nil {
			t.Errorf("svgDocument(%q) succeeded, want an error", d)
		}
	}
}

func TestSVGDocumentRejectsMarkup(t *testing.T) {
	// These would need escaping inside the attribute and are never valid path
	// data, so a string carrying one is reported rather than rewritten.
	for _, d := range []string{
		`M 10 10 <b>`,
		`M 10 10" onload="x`,
		`M 10 10 &amp; 20 20`,
		`M 10 10 > 20 20`,
	} {
		_, err := svgDocument(d)
		if err == nil {
			t.Errorf("svgDocument(%q) succeeded, want an error", d)

			continue
		}

		if !strings.Contains(err.Error(), "not valid in SVG path data") {
			t.Errorf("svgDocument(%q) error = %q, want it to name the bad character", d, err)
		}
	}
}

func TestPathResultNamesTheKeptPath(t *testing.T) {
	if got := pathResult(203, true); got["path_id"] != 203 || got["status"] != "success" {
		t.Errorf("pathResult(203, true) = %v, want status and path_id 203", got)
	}

	if got := pathResult(203, false); got["path_id"] != nil {
		t.Errorf("pathResult(203, false) = %v, want no path_id", got)
	}
}

func TestPathCommandsAreRegistered(t *testing.T) {
	for _, name := range []string{
		"draw_path", "fill_path", "select_path", "list_paths", "path_to_selection",
	} {
		if _, ok := Lookup(name); !ok {
			t.Errorf("command %s is not registered", name)
		}
	}
}
