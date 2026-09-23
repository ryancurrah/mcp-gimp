package gimpbridge

/*
#include "bridge.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"strconv"
	"strings"
	"unsafe"
)

// Property is what an operation or procedure will accept for one of its
// settings.
//
// It is read from the running GIMP rather than from documentation. Nothing
// published records it: the libgimp reference documents the procedures, and
// GEGL's reference documents libgegl rather than the operation catalogue.
type Property struct {
	Name string `json:"name"`
	// Type is the GType the property holds, such as gdouble or GimpChoice.
	Type string `json:"type"`
	// Min and Max bound a numeric property. A value outside them is refused
	// rather than clamped, because GLib would otherwise discard it and leave
	// the operation's own default in place.
	Min *float64 `json:"minimum,omitempty"`
	Max *float64 `json:"maximum,omitempty"`
	// Default is the operation's own default, as text.
	Default string `json:"default,omitempty"`
	// Choices are the permitted values of a property set by name.
	Choices []string `json:"choices,omitempty"`
	Blurb   string   `json:"blurb,omitempty"`
}

// DescribeOp lists the properties an operation exposes through GIMP's filter
// configuration, which is what ApplyGEGL configures.
//
// It must be called from the GIMP main thread; see [Do].
func DescribeOp(drawable ObjectID, operation string) ([]Property, error) {
	cop := C.CString(operation)
	defer C.free(unsafe.Pointer(cop))

	var cerr *C.char

	// gocritic reads the cgo-expanded call below, not this source line.
	out := C.mcp_describe_op(C.gint32(drawable), cop, &cerr) //nolint:gocritic // cgo expansion
	if out == nil {
		msg := errUnknown

		if cerr != nil {
			msg = C.GoString(cerr)
			C.free(unsafe.Pointer(cerr))
		}

		return nil, fmt.Errorf("describe %s: %s", operation, msg)
	}

	defer C.free(unsafe.Pointer(out))

	return parseProperties(C.GoString(out)), nil
}

// parseProperties reads the tab-separated table the bridge builds.
func parseProperties(table string) []Property {
	lines := strings.Split(strings.TrimRight(table, "\n"), "\n")
	props := make([]Property, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		f := strings.Split(line, "\t")
		if len(f) < 7 {
			continue
		}

		p := Property{Name: f[0], Type: f[1], Default: f[4], Blurb: f[6]}

		if v, err := strconv.ParseFloat(f[2], 64); err == nil {
			p.Min = &v
		}

		if v, err := strconv.ParseFloat(f[3], 64); err == nil {
			p.Max = &v
		}

		if f[5] != "" {
			p.Choices = strings.Split(f[5], ",")
		}

		props = append(props, p)
	}

	return props
}

// UserDirectory is GIMP's per-user configuration directory. It comes from
// libgimpbase rather than the PDB, which has no procedure for it.
func UserDirectory() string {
	return C.GoString(C.gimp_directory())
}
