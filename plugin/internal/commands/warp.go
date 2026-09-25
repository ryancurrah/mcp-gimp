package commands

import (
	"fmt"
	"math"
	"slices"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
)

// warpVector is one push of warp_region: the pixels around (X, Y) move
// towards (X+DX, Y+DY), the centre by Amount of that distance, the rest less
// the further out they are, and nothing from Radius on.
type warpVector struct {
	X, Y, DX, DY, Radius, Amount float64
}

// warpVectorsFrom reads and checks warp_region's vectors.
func warpVectorsFrom(p Params) ([]warpVector, error) {
	objs := p.Objects("vectors")
	if len(objs) == 0 {
		return nil, fmt.Errorf("vectors must be a non-empty list of {x, y, dx, dy} objects")
	}

	vectors := make([]warpVector, len(objs))

	for i, o := range objs {
		for _, key := range []string{"x", "y", "dx", "dy"} {
			if !o.Has(key) {
				return nil, fmt.Errorf("vector %d has no %s", i, key)
			}
		}

		v := warpVector{
			X: o.Float("x", 0), Y: o.Float("y", 0),
			DX: o.Float("dx", 0), DY: o.Float("dy", 0),
			Radius: o.Float("radius", 40), Amount: o.Float("amount", 0.3),
		}

		if v.Radius < 1 {
			return nil, fmt.Errorf("vector %d: radius must be at least 1, got %g", i, v.Radius)
		}

		if v.Amount < 0 || v.Amount > 1 {
			return nil, fmt.Errorf("vector %d: amount must be within 0..1, got %g", i, v.Amount)
		}

		vectors[i] = v
	}

	return vectors, nil
}

// warpArea is the rectangle, in a drawable's own coordinates, that the
// vectors change or sample from, clipped to the drawable. The drawable sits
// at (offX, offY) in the image and is width by height; the vectors are in
// image coordinates. An empty rectangle means no vector reaches the drawable.
func warpArea(vectors []warpVector, offX, offY, width, height int) (x, y, w, h int) {
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)

	for _, v := range vectors {
		// A pixel inside the radius samples from up to the push's length
		// away, plus a pixel for the bilinear neighbours.
		reach := v.Radius + v.Amount*math.Hypot(v.DX, v.DY) + 1
		cx, cy := v.X-float64(offX), v.Y-float64(offY)

		x0, y0 = math.Min(x0, cx-reach), math.Min(y0, cy-reach)
		x1, y1 = math.Max(x1, cx+reach), math.Max(y1, cy+reach)
	}

	left := max(0, int(math.Floor(x0)))
	top := max(0, int(math.Floor(y0)))
	right := min(width, int(math.Ceil(x1)))
	bottom := min(height, int(math.Ceil(y1)))

	return left, top, max(0, right-left), max(0, bottom-top)
}

// warpPixels applies one push to an R'G'B'A float region w by h pixels whose
// top-left pixel is at (originX, originY) in the vector's coordinates, and
// returns the warped copy.
//
// Each pixel inside the radius takes its colour from where the push brings
// it from, sampled bilinearly, so the content at the centre moves by about
// Amount × (DX, DY) and the edge of the radius does not move at all. The
// falloff, (1 - t²)² of the distance t as a fraction of the radius, is smooth
// at both ends, so the push leaves no crease at the centre or the rim.
func warpPixels(px []float32, w, h int, originX, originY float64, v warpVector) []float32 {
	out := slices.Clone(px)
	pushX, pushY := v.DX*v.Amount, v.DY*v.Amount

	if pushX == 0 && pushY == 0 {
		return out
	}

	// Pixel (i, j) covers [i, i+1) × [j, j+1) of the region; its centre is
	// half a pixel in.
	cx, cy := v.X-originX-0.5, v.Y-originY-0.5

	i0 := max(0, int(math.Floor(cx-v.Radius)))
	i1 := min(w-1, int(math.Ceil(cx+v.Radius)))
	j0 := max(0, int(math.Floor(cy-v.Radius)))
	j1 := min(h-1, int(math.Ceil(cy+v.Radius)))

	for j := j0; j <= j1; j++ {
		for i := i0; i <= i1; i++ {
			t := math.Hypot(float64(i)-cx, float64(j)-cy) / v.Radius
			if t >= 1 {
				continue
			}

			f := (1 - t*t) * (1 - t*t)
			k := (j*w + i) * 4
			sampleBilinear(px, w, h, float64(i)-f*pushX, float64(j)-f*pushY, out[k:k+4])
		}
	}

	return out
}

// sampleBilinear reads the colour at (x, y), in pixel-centre coordinates,
// from an R'G'B'A float region, repeating the edge pixels beyond it.
//
// It blends premultiplied colours, so a transparent pixel's leftover colour,
// usually black, does not darken the edge of an opaque one next to it.
func sampleBilinear(px []float32, w, h int, x, y float64, dst []float32) {
	x = math.Min(math.Max(x, 0), float64(w-1))
	y = math.Min(math.Max(y, 0), float64(h-1))

	ix, iy := int(x), int(y)
	fx, fy := x-float64(ix), y-float64(iy)
	nx, ny := min(ix+1, w-1), min(iy+1, h-1)

	var sum [4]float64

	for _, c := range [4]struct {
		i, j   int
		weight float64
	}{
		{ix, iy, (1 - fx) * (1 - fy)},
		{nx, iy, fx * (1 - fy)},
		{ix, ny, (1 - fx) * fy},
		{nx, ny, fx * fy},
	} {
		k := (c.j*w + c.i) * 4
		alpha := float64(px[k+3]) * c.weight

		sum[0] += float64(px[k]) * alpha
		sum[1] += float64(px[k+1]) * alpha
		sum[2] += float64(px[k+2]) * alpha
		sum[3] += alpha
	}

	if sum[3] == 0 {
		dst[0], dst[1], dst[2], dst[3] = 0, 0, 0, 0

		return
	}

	dst[0] = float32(sum[0] / sum[3])
	dst[1] = float32(sum[1] / sum[3])
	dst[2] = float32(sum[2] / sum[3])
	dst[3] = float32(sum[3])
}

// warpRegion pushes the pixels around each vector's point, in order, as one
// undo step.
//
// GIMP 3 has neither the iwarp plug-in nor a PDB warp, and it will not build
// gegl:warp as a drawable filter, so the warp is computed here on the
// layer's pixels and written back through its shadow buffer.
func warpRegion(p Params) (any, error) {
	_, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	vectors, err := warpVectorsFrom(p)
	if err != nil {
		return nil, err
	}

	offX, offY, err := drawableOffsets(drawable)
	if err != nil {
		return nil, err
	}

	width, height, err := drawableSize(drawable)
	if err != nil {
		return nil, err
	}

	x, y, w, h := warpArea(vectors, offX, offY, width, height)
	if w == 0 || h == 0 {
		return nil, fmt.Errorf("no vector reaches the layer, which spans (%d, %d) to (%d, %d)",
			offX, offY, offX+width, offY+height)
	}

	px, err := gimpbridge.ReadPixels(drawable, x, y, w, h)
	if err != nil {
		return nil, err
	}

	for _, v := range vectors {
		px = warpPixels(px, w, h, float64(x+offX), float64(y+offY), v)
	}

	if err := gimpbridge.WritePixels(drawable, x, y, w, h, px); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return map[string]any{
		"status": "success", "warped_vectors": len(vectors),
		"bounds": map[string]any{"x": x + offX, "y": y + offY, "width": w, "height": h},
	}, nil
}
