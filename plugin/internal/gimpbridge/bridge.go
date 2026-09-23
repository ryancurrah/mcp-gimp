// Package gimpbridge wraps libgimp's procedural database (PDB) so GIMP can be
// driven from Go.
//
// Rather than binding libgimp function by function, everything goes through
// the PDB: a procedure is looked up by name, its arguments are bound onto a
// GimpProcedureConfig by name and type, and the run is dispatched. That covers
// the whole GIMP API surface with one bridge.
//
// libgimp is not thread safe and expects to be used from the thread running
// the GLib main loop. Every call below must therefore be made from inside
// [Do]; calling directly from another goroutine will corrupt GIMP's state.
package gimpbridge

/*
#cgo pkg-config: gimp-3.0 gimpui-3.0
#include "bridge.h"
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime/cgo"
	"unsafe"
)

// errUnknown is what the bridge reports when a C call fails without setting a
// message, which should not happen but must not produce an empty error.
const errUnknown = "unknown error"

// Value is a JSON-compatible value crossing the bridge.
type Value = any

// ObjectID identifies a GIMP image or item. GIMP hands out integer ids that
// stay valid for the lifetime of the object, which is what the wire protocol
// exchanges instead of pointers.
type ObjectID int32

// Args are procedure arguments keyed by the PDB argument name.
type Args map[string]Value

// Color is a CSS colour string such as "white", "#ff5733" or "rgb(1,2,3)".
type Color string

// Items is a list of item ids bound to a core-object-array argument.
type Items []ObjectID

// Doubles is a list bound to a float-array argument, used for coordinates.
type Doubles []float64

// Run executes a PDB procedure and returns its results.
//
// It must be called from the GIMP main thread; see [Do].
func Run(name string, args Args) ([]Value, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	proc := C.mcp_lookup_procedure(cname)
	if proc == nil {
		return nil, fmt.Errorf("no such GIMP procedure: %s", name)
	}

	config := C.mcp_create_config(proc)
	if config == nil {
		return nil, fmt.Errorf("cannot configure %s", name)
	}

	if err := bindArgs(proc, config, name, args); err != nil {
		return nil, err
	}

	var cerr *C.char

	// gocritic reads the cgo-expanded call below, not this source line.
	values := C.mcp_run_config(proc, config, &cerr) //nolint:gocritic // cgo expansion
	if values == nil {
		msg := errUnknown

		if cerr != nil {
			msg = C.GoString(cerr)
			C.free(unsafe.Pointer(cerr))
		}

		return nil, fmt.Errorf("%s: %s", name, msg)
	}
	defer C.mcp_values_free(values)

	return readResults(values), nil
}

// NormalizeColor returns a CSS colour in the form GIMP's colour results are
// reported in, so a requested colour can be compared with one read back.
//
// It must be called from the GIMP main thread; see [Do].
func NormalizeColor(css string) (string, error) {
	ccss := C.CString(css)
	defer C.free(unsafe.Pointer(ccss))

	out := C.mcp_color_css_normalize(ccss)
	if out == nil {
		return "", fmt.Errorf("cannot parse colour %q", css)
	}

	return goStringFree(out), nil
}

// argSpec describes one declared procedure argument.
type argSpec struct {
	name    string
	gtype   string
	blurb   string
	object  bool
	present bool
}

// Describe lists the arguments a PDB procedure declares, with their ranges,
// defaults and permitted values.
//
// It must be called from the GIMP main thread; see [Do].
func Describe(name string) ([]Property, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	proc := C.mcp_lookup_procedure(cname)
	if proc == nil {
		return nil, fmt.Errorf("no such GIMP procedure: %s", name)
	}

	table := C.mcp_describe_procedure(proc)
	defer C.free(unsafe.Pointer(table))

	return parseProperties(C.GoString(table)), nil
}

// bindArgs assigns each supplied argument onto the config, coercing it to the
// type the procedure declared.
func bindArgs(proc *C.GimpProcedure, config *C.GimpProcedureConfig, procName string, args Args) error {
	specs := make(map[string]argSpec, len(args))

	for i := range int(C.mcp_procedure_arg_count(proc)) {
		name := C.GoString(C.mcp_procedure_arg_name(proc, C.int(i)))
		specs[name] = argSpec{
			name:    name,
			gtype:   C.GoString(C.mcp_procedure_arg_type(proc, C.int(i))),
			object:  C.mcp_procedure_arg_is_object(proc, C.int(i)) != 0,
			present: true,
		}
	}

	for key, raw := range args {
		spec, ok := specs[key]
		if !ok {
			return fmt.Errorf("%s has no argument %q", procName, key)
		}

		if err := bindOne(config, spec, raw); err != nil {
			return fmt.Errorf("%s argument %q: %w", procName, key, err)
		}
	}

	return nil
}

// bindOne assigns a single argument, dispatching on the Go value's type and
// falling back to the declared GType for numbers.
func bindOne(config *C.GimpProcedureConfig, spec argSpec, raw Value) error {
	cname := C.CString(spec.name)
	defer C.free(unsafe.Pointer(cname))

	switch v := raw.(type) {
	case nil:
		return nil

	case bool:
		C.mcp_set_bool(config, cname, boolToC(v))

	case string:
		cval := C.CString(v)
		defer C.free(unsafe.Pointer(cval))

		// A string lands in a colour or file argument as CSS or a path
		// rather than as plain text.
		switch spec.gtype {
		case "GeglColor":
			return setColor(config, cname, spec, Color(v))
		case "GFile", "GLocalFile":
			C.mcp_set_file(config, cname, cval)
		default:
			// Enum and GimpChoice arguments are set by name, so a caller
			// never needs GIMP's numbering.
			return cError(C.mcp_set_string(config, cname, cval))
		}

	case Color:
		return setColor(config, cname, spec, v)

	case ObjectID:
		return setObject(config, cname, spec, v)

	case int:
		return setScalar(config, cname, spec, float64(v), int64(v))

	case int32:
		return setScalar(config, cname, spec, float64(v), int64(v))

	case int64:
		return setScalar(config, cname, spec, float64(v), v)

	case float64:
		return setScalar(config, cname, spec, v, int64(v))

	case Items:
		return setItems(config, cname, v)

	case Doubles:
		// JSON has no way to mark a list as ids, so a list of numbers aimed at
		// an object-array argument arrives here and is bound as items.
		if spec.gtype == objectArrayType {
			ids := make(Items, len(v))
			for i, f := range v {
				ids[i] = ObjectID(f)
			}

			return setItems(config, cname, ids)
		}

		vals := make([]C.double, len(v))
		for i, f := range v {
			vals[i] = C.double(f)
		}

		var head *C.double
		if len(vals) > 0 {
			head = &vals[0]
		}

		C.mcp_set_double_array(config, cname, head, C.int(len(vals)))

	default:
		return fmt.Errorf("cannot bind %T", raw)
	}

	return nil
}

// booleanType is the GType name of a boolean argument, which takes no range.
const booleanType = "gboolean"

// objectArrayType is the GType name GIMP declares for arguments that take a
// list of images, items or other objects.
const objectArrayType = "GimpCoreObjectArray"

// setItems binds a list of GIMP objects by id.
func setItems(config *C.GimpProcedureConfig, cname *C.char, items Items) error {
	ids := make([]C.gint32, len(items))
	for i, id := range items {
		ids[i] = C.gint32(id)
	}

	var head *C.gint32
	if len(ids) > 0 {
		head = &ids[0]
	}

	if C.mcp_set_item_array(config, cname, head, C.int(len(ids))) == 0 {
		return fmt.Errorf("one of the items %v does not exist", items)
	}

	return nil
}

// setColor binds a CSS colour string.
func setColor(config *C.GimpProcedureConfig, cname *C.char, spec argSpec, c Color) error {
	cval := C.CString(string(c))
	defer C.free(unsafe.Pointer(cval))

	if C.mcp_set_color(config, cname, cval) == 0 {
		return fmt.Errorf("cannot parse colour %q", string(c))
	}

	return nil
}

// setObject binds an image or item by id, choosing based on the declared type.
//
// A negative id means "no object", which is how GIMP spells an absent parent
// or an optional drawable.
func setObject(config *C.GimpProcedureConfig, cname *C.char, spec argSpec, id ObjectID) error {
	if id < 0 {
		C.mcp_set_object_null(config, cname)

		return nil
	}

	if spec.gtype == "GimpImage" {
		if C.mcp_set_image(config, cname, C.gint32(id)) == 0 {
			return fmt.Errorf("no image with id %d", id)
		}

		return nil
	}

	if C.mcp_set_item(config, cname, C.gint32(id)) == 0 {
		return fmt.Errorf("no %s with id %d", spec.gtype, id)
	}

	return nil
}

// setScalar binds a number, routing it to an object lookup when the argument
// actually takes a GIMP object such as an image or layer.
func setScalar(config *C.GimpProcedureConfig, cname *C.char, spec argSpec, f float64, i int64) error {
	if spec.object {
		return setObject(config, cname, spec, ObjectID(i))
	}

	if spec.gtype != booleanType {
		if err := cError(C.mcp_check_number(config, cname, C.double(f))); err != nil {
			return err
		}
	}

	setNumber(config, cname, spec, f, i)

	return nil
}

// cError converts a message the bridge allocated into an error, freeing it.
func cError(msg *C.char) error {
	if msg == nil {
		return nil
	}

	defer C.free(unsafe.Pointer(msg))

	return errors.New(C.GoString(msg))
}

// setNumber binds a numeric argument, honouring the declared type so an int
// lands in a double argument and vice versa.
func setNumber(config *C.GimpProcedureConfig, cname *C.char, spec argSpec, f float64, i int64) {
	switch spec.gtype {
	case "gdouble", "gfloat":
		C.mcp_set_double(config, cname, C.double(f))
	case booleanType:
		C.mcp_set_bool(config, cname, boolToC(i != 0))
	default:
		C.mcp_set_int(config, cname, C.gint64(i))
	}
}

// readResults converts the procedure's return values, skipping the leading
// status enum that every PDB procedure returns.
func readResults(values *C.GimpValueArray) []Value {
	n := int(C.mcp_values_length(values, 0))
	out := make([]Value, 0, max(n-1, 0))

	for i := 1; i < n; i++ {
		out = append(out, readValue(values, C.int(i)))
	}

	return out
}

// readValue converts one return value based on its GType.
func readValue(values *C.GimpValueArray, i C.int) Value {
	switch t := C.GoString(C.mcp_value_type(values, i)); t {
	case booleanType:
		return C.mcp_value_bool(values, i) != 0

	case "gdouble", "gfloat":
		return float64(C.mcp_value_double(values, i))

	case "gint", "guint", "gint64", "guint64":
		return int64(C.mcp_value_int(values, i))

	case "gchararray":
		return goStringFree(C.mcp_value_string(values, i))

	case "GeglColor":
		return goStringFree(C.mcp_value_color_css(values, i))

	case "GFile", "GLocalFile":
		path := C.mcp_value_file_path(values, i)
		if path == nil {
			return nil
		}

		return goStringFree(path)

	case "GimpCoreObjectArray":
		n := int(C.mcp_value_object_array_len(values, i))
		ids := make([]ObjectID, n)

		for j := range n {
			ids[j] = ObjectID(C.mcp_value_object_array_id(values, i, C.int(j)))
		}

		return ids

	case "GimpDoubleArray":
		n := int(C.mcp_value_double_array_len(values, i))
		out := make([]float64, n)

		for j := range n {
			out[j] = float64(C.mcp_value_double_array_at(values, i, C.int(j)))
		}

		return out

	default:
		// Images, layers, channels and other GObjects come back as ids.
		if id := int32(C.mcp_value_object_id(values, i)); id >= 0 {
			return ObjectID(id)
		}

		return goStringFree(C.mcp_value_string(values, i))
	}
}

// goStringFree converts a C string allocated by GLib and releases it.
func goStringFree(s *C.char) string {
	if s == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(s))

	return C.GoString(s)
}

// boolToC converts a Go bool to the C int the bridge expects.
func boolToC(b bool) C.int {
	if b {
		return 1
	}

	return 0
}

// ApplyGEGL applies a GEGL operation to a drawable and merges it down.
//
// GIMP 3's filters are GEGL operations rather than individual PDB
// procedures, so effects go through here instead of [Run]. Settings are
// numeric; the bridge coerces each one to the type the operation declares.
//
// It must be called from the GIMP main thread; see [Do].
func ApplyGEGL(drawable ObjectID, operation string, settings map[string]float64) error {
	return ApplyGEGLWithChoices(drawable, operation, settings, nil)
}

// ApplyGEGLWithChoices applies a GEGL operation whose configuration includes a
// property named by one of a fixed set of choices.
//
// GIMP 3 re-declares a GEGL enum property as a GimpChoice, whose value is a
// string, so a property such as gegl:vignette's shape cannot be set as a
// number however it is coerced: the bridge finds no numeric property of that
// name and silently leaves the operation's own default in place. Choices go
// through their own map for that reason.
//
// It must be called from the GIMP main thread; see [Do].
func ApplyGEGLWithChoices(drawable ObjectID, operation string,
	settings map[string]float64, choices map[string]string,
) error {
	cop := C.CString(operation)
	defer C.free(unsafe.Pointer(cop))

	textNames := make([]*C.char, 0, len(choices))
	textValues := make([]*C.char, 0, len(choices))

	for name, value := range choices {
		cname := C.CString(name)
		// Freed when the call returns, which is what the GEGL op needs.
		defer C.free(unsafe.Pointer(cname))

		cvalue := C.CString(value)
		defer C.free(unsafe.Pointer(cvalue))

		textNames = append(textNames, cname)
		textValues = append(textValues, cvalue)
	}

	var (
		textNameHead  **C.char
		textValueHead **C.char
	)

	if len(textNames) > 0 {
		textNameHead = (**C.char)(unsafe.Pointer(&textNames[0]))
		textValueHead = (**C.char)(unsafe.Pointer(&textValues[0]))
	}

	names := make([]*C.char, 0, len(settings))
	values := make([]C.double, 0, len(settings))

	for name, value := range settings {
		cname := C.CString(name)
		// Freed when the call returns, which is what the GEGL op needs.
		defer C.free(unsafe.Pointer(cname))

		names = append(names, cname)
		values = append(values, C.double(value))
	}

	var (
		nameHead  **C.char
		valueHead *C.double
	)

	if len(names) > 0 {
		nameHead = (**C.char)(unsafe.Pointer(&names[0]))
		valueHead = &values[0]
	}

	var cerr *C.char

	// gocritic reads the cgo-expanded call below, not this source line.
	if C.mcp_apply_gegl(C.gint32(drawable), cop, nameHead, valueHead,
		C.int(len(names)), textNameHead, textValueHead,
		C.int(len(textNames)), &cerr) == 0 { //nolint:gocritic // cgo expansion
		msg := errUnknown

		if cerr != nil {
			msg = C.GoString(cerr)
			C.free(unsafe.Pointer(cerr))
		}

		// The bridge already names the operation in its message.
		return errors.New(msg)
	}

	return nil
}

// call is a unit of work queued onto the GIMP main thread.
type call struct {
	fn   func()
	done chan struct{}
}

// Do runs fn on the GIMP main thread and waits for it to finish.
//
// Every libgimp call must be wrapped in this, because GIMP's wire protocol is
// dispatched by the GLib main loop and is not safe to touch concurrently.
func Do(fn func()) {
	c := &call{fn: fn, done: make(chan struct{})}

	h := cgo.NewHandle(c)
	defer h.Delete()

	C.mcp_idle_dispatch(C.guintptr(h))
	<-c.done
}

// goMainThreadCall is invoked by the GLib idle handler on the main thread.
//
//export goMainThreadCall
func goMainThreadCall(handle C.guintptr) {
	c, ok := cgo.Handle(handle).Value().(*call)
	if !ok {
		return
	}

	defer close(c.done)

	c.fn()
}

// Quit stops the plug-in's main loop, ending the GIMP procedure.
func Quit() { C.mcp_quit_main_loop() }

// Main hands control to libgimp, which drives plug-in registration and
// invokes the registered procedure. It returns GIMP's exit status.
func Main(args []string) int {
	argv := make([]*C.char, len(args)+1)

	for i, a := range args {
		argv[i] = C.CString(a)
	}

	defer func() {
		for _, p := range argv {
			if p != nil {
				C.free(unsafe.Pointer(p))
			}
		}
	}()

	return int(C.mcp_main(C.int(len(args)), (**C.char)(unsafe.Pointer(&argv[0]))))
}

// OnStart is invoked on the GIMP main thread when the user runs
// Tools > MCP > Start MCP Server. It must not block: the main loop it returns
// into is what dispatches work queued by [Do].
var OnStart func(host string, port int) error

// OnStop is invoked when the user runs Tools > MCP > Stop MCP Server.
//
// GIMP runs each procedure in its own process, so this one is not the process
// serving the socket and cannot reach its main loop directly. It asks over the
// socket instead.
var OnStop func(host string, port int) error

// goStartServer is called by the plug-in's run callback.
//
//export goStartServer
func goStartServer(host *C.char, port C.int) *C.char {
	return callHook(OnStart, host, port)
}

// goStopServer is called by the stop procedure's run callback.
//
//export goStopServer
func goStopServer(host *C.char, port C.int) *C.char {
	return callHook(OnStop, host, port)
}

// callHook runs a hook and hands its error back to C.
//
// A NULL return means success. Anything else is a message allocated with
// malloc that the caller frees, which is what C.CString gives us.
func callHook(fn func(string, int) error, host *C.char, port C.int) *C.char {
	if fn == nil {
		return nil
	}

	if err := fn(C.GoString(host), int(port)); err != nil {
		return C.CString(err.Error())
	}

	return nil
}
