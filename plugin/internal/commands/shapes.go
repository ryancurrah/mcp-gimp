package commands

import (
	"fmt"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
)

//nolint:gochecknoinits // the command table is assembled from several files
func init() {
	register("draw_shapes", drawShapes)
}

// shapeSelectProcs names the selection procedure for each draw_shapes type
// other than path.
var shapeSelectProcs = map[string]string{
	"rectangle":         "gimp-image-select-rectangle",
	"rounded_rectangle": selectRoundRectangleProc,
	"ellipse":           "gimp-image-select-ellipse",
}

// drawShapes draws a list of shapes in order on one layer, as one undo step.
//
// Every shape is checked before the first is drawn, so a mistake late in the
// list is reported without leaving the earlier shapes on the canvas.
func drawShapes(p Params) (any, error) {
	image, drawable, err := target(p)
	if err != nil {
		return nil, err
	}

	shapes := p.Objects("shapes")
	if len(shapes) == 0 {
		return nil, fmt.Errorf("shapes must be a non-empty list of shape objects")
	}

	specs := make([]paintSpec, len(shapes))
	for i, s := range shapes {
		if specs[i], err = checkShape(s); err == nil {
			err = specs[i].checkColors()
		}

		if err != nil {
			return nil, shapeError(i, s, err)
		}
	}

	return withUndoGroup(image, func() (any, error) {
		alphaAdded := false

		for i, s := range shapes {
			added, err := drawShape(image, drawable, s, specs[i])
			if err != nil {
				err = shapeError(i, s, err)
				if i > 0 {
					err = fmt.Errorf("%w; the %d shape(s) before it were drawn", err, i)
				}

				return nil, err
			}

			alphaAdded = alphaAdded || added
		}

		if err := flush(); err != nil {
			return nil, err
		}

		result := paintResult(alphaAdded)
		result["shapes_drawn"] = len(shapes)

		return result, nil
	})
}

// checkShape validates one draw_shapes entry and reads what it paints.
func checkShape(s Params) (paintSpec, error) {
	kind := s.String("type", "")

	switch {
	case kind == "path":
		if _, err := svgDocument(s.String("d", "")); err != nil {
			return paintSpec{}, err
		}
	case shapeSelectProcs[kind] != "":
		if s.Float("width", 0) <= 0 || s.Float("height", 0) <= 0 {
			return paintSpec{}, fmt.Errorf("a %s needs a width and height above 0", kind)
		}
	default:
		return paintSpec{}, fmt.Errorf(
			"unknown type %q; want rectangle, rounded_rectangle, ellipse or path", kind)
	}

	return paintSpecFrom(s)
}

// shapeError names the shape an error belongs to by its index in the list
// and its type.
func shapeError(i int, s Params, err error) error {
	return fmt.Errorf("shape %d (%s): %w", i, s.String("type", "no type"), err)
}

// drawShape paints one checked shape on drawable.
func drawShape(image, drawable gimpbridge.ObjectID, s Params, spec paintSpec) (bool, error) {
	kind := s.String("type", "")
	if kind != "path" {
		return paintShape(image, drawable, s, shapeSelectProcs[kind], spec)
	}

	var alphaAdded bool

	_, err := withPath(image, s.String("d", ""), false, func(path gimpbridge.ObjectID) error {
		var err error

		alphaAdded, _, err = paintPath(image, drawable, path, spec)

		return err
	})

	return alphaAdded, err
}
