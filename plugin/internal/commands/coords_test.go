package commands

import "testing"

func TestGradientLineTakesImageCoordinates(t *testing.T) {
	// A layer at (200, 100) in a 400-pixel-wide image: a gradient from image
	// x=200 to x=300 runs from the layer's left edge to its middle.
	x1, y1, x2, y2 := gradientLine(decode(t, `{"x1":200,"y1":150,"x2":300,"y2":150}`), 400, 200, 100)
	if x1 != 0 || y1 != 50 || x2 != 100 || y2 != 50 {
		t.Errorf("line = (%g, %g) to (%g, %g), want (0, 50) to (100, 50)", x1, y1, x2, y2)
	}

	// The default runs from the image's top-left to its top-right corner,
	// wherever the layer is.
	x1, y1, x2, y2 = gradientLine(decode(t, `{"x1":0,"y1":0,"x2":null,"y2":null}`), 400, 200, 100)
	if x1 != -200 || y1 != -100 || x2 != 200 || y2 != -100 {
		t.Errorf("default line = (%g, %g) to (%g, %g), want (-200, -100) to (200, -100)", x1, y1, x2, y2)
	}
}

func TestInRect(t *testing.T) {
	for _, tc := range []struct {
		x, y int
		want bool
	}{
		{200, 100, true}, {299, 199, true}, {300, 150, false}, {250, 200, false}, {199, 150, false},
	} {
		if got := inRect(tc.x, tc.y, 200, 100, 100, 100); got != tc.want {
			t.Errorf("inRect(%d, %d) in (200, 100) 100x100 = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
}
