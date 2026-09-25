package gimpbridge

/*
#include "bridge.h"
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// ReadPixels copies a rectangle of a drawable, in the drawable's own
// coordinates, as straight R'G'B'A floats in its colour space, four per
// pixel, row by row. Outside the drawable the nearest edge pixel is repeated.
//
// It must be called from the GIMP main thread; see [Do].
func ReadPixels(drawable ObjectID, x, y, width, height int) ([]float32, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("cannot read a %dx%d rectangle", width, height)
	}

	px := make([]float32, width*height*4)

	var cerr *C.char

	if C.mcp_read_pixels(C.gint32(drawable), C.int(x), C.int(y), C.int(width), C.int(height),
		(*C.float)(unsafe.Pointer(&px[0])), &cerr) == 0 { //nolint:gocritic // cgo expansion
		return nil, bridgeError(cerr)
	}

	return px, nil
}

// WritePixels replaces a rectangle of a drawable with pixels in the format
// ReadPixels returns. The change is one undo step, and an active selection
// limits it, as it does a filter.
//
// It must be called from the GIMP main thread; see [Do].
func WritePixels(drawable ObjectID, x, y, width, height int, px []float32) error {
	if width <= 0 || height <= 0 || len(px) != width*height*4 {
		return fmt.Errorf("%d floats do not fill a %dx%d rectangle of R'G'B'A pixels",
			len(px), width, height)
	}

	var cerr *C.char

	if C.mcp_write_pixels(C.gint32(drawable), C.int(x), C.int(y), C.int(width), C.int(height),
		(*C.float)(unsafe.Pointer(&px[0])), &cerr) == 0 { //nolint:gocritic // cgo expansion
		return bridgeError(cerr)
	}

	return nil
}

// bridgeError converts the message a failed bridge call set, which may be
// missing, into an error.
func bridgeError(cerr *C.char) error {
	if err := cError(cerr); err != nil {
		return err
	}

	return errors.New(errUnknown)
}
