package commands

import (
	"math"
	"slices"
	"strings"
	"testing"
)

// columnImage is a w by h opaque black region with a white column at x.
func columnImage(w, h, x int) []float32 {
	px := make([]float32, w*h*4)
	for j := range h {
		for i := range w {
			k := (j*w + i) * 4
			if i == x {
				px[k], px[k+1], px[k+2] = 1, 1, 1
			}

			px[k+3] = 1
		}
	}

	return px
}

// brightestInRow is the x of the brightest pixel in row j.
func brightestInRow(px []float32, w, j int) int {
	best, bestX := float32(-1), -1

	for i := range w {
		if v := px[(j*w+i)*4]; v > best {
			best, bestX = v, i
		}
	}

	return bestX
}

func TestWarpPushesTheCentre(t *testing.T) {
	const w, h = 41, 41

	px := columnImage(w, h, 20)
	out := warpPixels(px, w, h, 0, 0, warpVector{X: 20.5, Y: 20.5, DX: 6, DY: 0, Radius: 15, Amount: 1})

	// The column moves right at the centre row, by a little under the push
	// because the falloff has begun by the time it lands.
	if got := brightestInRow(out, w, 20); got < 24 || got > 26 {
		t.Errorf("column at the centre row is at x=%d, want it pushed to about x=25", got)
	}

	// Rows beyond the radius are untouched.
	if got := brightestInRow(out, w, 2); got != 20 {
		t.Errorf("column outside the radius is at x=%d, want it left at 20", got)
	}
}

func TestWarpLeavesPixelsOutsideTheRadiusAlone(t *testing.T) {
	const w, h = 30, 30

	px := columnImage(w, h, 15)
	v := warpVector{X: 15, Y: 15, DX: 5, DY: 5, Radius: 8, Amount: 0.5}
	out := warpPixels(px, w, h, 0, 0, v)

	for j := range h {
		for i := range w {
			if math.Hypot(float64(i)+0.5-v.X, float64(j)+0.5-v.Y) < v.Radius {
				continue
			}

			k := (j*w + i) * 4
			if !slices.Equal(out[k:k+4], px[k:k+4]) {
				t.Fatalf("pixel (%d, %d) outside the radius changed", i, j)
			}
		}
	}
}

func TestWarpWithNoPushChangesNothing(t *testing.T) {
	px := columnImage(20, 20, 10)

	for _, v := range []warpVector{
		{X: 10, Y: 10, DX: 5, DY: 5, Radius: 8, Amount: 0},
		{X: 10, Y: 10, DX: 0, DY: 0, Radius: 8, Amount: 1},
	} {
		if out := warpPixels(px, 20, 20, 0, 0, v); !slices.Equal(out, px) {
			t.Errorf("warp %+v changed the pixels", v)
		}
	}
}

func TestWarpUsesTheRegionOrigin(t *testing.T) {
	// The same push on a region cut out of a larger image must move the same
	// content: vectors are in image coordinates, the region is not.
	const w, h = 41, 41

	full := warpPixels(columnImage(w, h, 20), w, h, 0, 0,
		warpVector{X: 120.5, Y: 220.5, DX: 6, Radius: 15, Amount: 1})
	if !slices.Equal(full, columnImage(w, h, 20)) {
		t.Error("a push far outside the region changed it")
	}

	moved := warpPixels(columnImage(w, h, 20), w, h, 100, 200,
		warpVector{X: 120.5, Y: 220.5, DX: 6, Radius: 15, Amount: 1})
	if got := brightestInRow(moved, w, 20); got < 24 {
		t.Errorf("with the region at (100, 200) the column is at x=%d, want it pushed right", got)
	}
}

func TestSampleBilinearBlendsPremultiplied(t *testing.T) {
	// Opaque red beside transparent black: halfway between them is red at
	// half coverage, not a darker red.
	px := []float32{1, 0, 0, 1, 0, 0, 0, 0}
	dst := make([]float32, 4)

	sampleBilinear(px, 2, 1, 0.5, 0, dst)

	if dst[0] != 1 || dst[1] != 0 || dst[2] != 0 || math.Abs(float64(dst[3])-0.5) > 1e-6 {
		t.Errorf("sample = %v, want red at alpha 0.5", dst)
	}
}

func TestWarpAreaCoversReachAndClips(t *testing.T) {
	v := warpVector{X: 110, Y: 60, DX: 10, DY: 0, Radius: 20, Amount: 0.5}

	// A layer at (100, 50): the push reaches 20 + 5 + 1 pixels around
	// (10, 10) in layer coordinates, so -16..36, clipped at the layer's
	// top-left to 0..36.
	x, y, w, h := warpArea([]warpVector{v}, 100, 50, 200, 200)
	if x != 0 || y != 0 || w != 36 || h != 36 {
		t.Errorf("area = (%d, %d) %dx%d, want (0, 0) 36x36", x, y, w, h)
	}

	if _, _, w, h := warpArea([]warpVector{v}, 1000, 1000, 50, 50); w != 0 || h != 0 {
		t.Errorf("a push nowhere near the layer gave a %dx%d area, want none", w, h)
	}
}

func TestWarpVectorsFromChecksEachVector(t *testing.T) {
	vs, err := warpVectorsFrom(decode(t, `{"vectors":[{"x":1,"y":2,"dx":3,"dy":-4}]}`))
	if err != nil {
		t.Fatalf("warpVectorsFrom: %v", err)
	}

	if want := (warpVector{X: 1, Y: 2, DX: 3, DY: -4, Radius: 40, Amount: 0.3}); vs[0] != want {
		t.Errorf("vector = %+v, want %+v", vs[0], want)
	}

	for body, want := range map[string]string{
		`{"vectors":[]}`:                                       "non-empty",
		`{"vectors":[[1,2,3,4]]}`:                              "non-empty",
		`{"vectors":[{"x":1,"y":2,"dx":3}]}`:                   "vector 0 has no dy",
		`{"vectors":[{"x":1,"y":2,"dx":3,"dy":4,"radius":0}]}`: "radius must be at least 1",
		`{"vectors":[{"x":1,"y":2,"dx":3,"dy":4,"amount":2}]}`: "amount must be within 0..1",
	} {
		_, err := warpVectorsFrom(decode(t, body))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("warpVectorsFrom(%s) error = %v, want it to mention %q", body, err, want)
		}
	}
}
