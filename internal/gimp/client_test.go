package gimp_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/ryancurrah/mcp-gimp/internal/gimp"
)

// fakePlugin stands in for the GIMP plug-in: it accepts one JSON request per
// connection, hands it to reply, writes the response and closes.
type fakePlugin struct {
	t        *testing.T
	listener net.Listener
	requests chan gimp.Request
	reply    func(gimp.Request) string
}

// listen opens a loopback listener bound to the test's context.
func listen(t *testing.T) (net.Listener, error) {
	t.Helper()

	var lc net.ListenConfig

	return lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
}

// startFakePlugin listens on a loopback port and serves until the test ends.
func startFakePlugin(t *testing.T, reply func(gimp.Request) string) *fakePlugin {
	t.Helper()

	ln, err := listen(t)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	p := &fakePlugin{t: t, listener: ln, requests: make(chan gimp.Request, 8), reply: reply}

	go p.serve()

	t.Cleanup(func() { _ = ln.Close() })

	return p
}

// serve accepts connections until the listener is closed.
func (p *fakePlugin) serve() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}

		go p.handle(conn)
	}
}

// handle reads one request and writes the canned reply.
func (p *fakePlugin) handle(conn net.Conn) {
	defer conn.Close() //nolint:errcheck // test helper

	var req gimp.Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		return
	}

	p.requests <- req

	_, _ = io.WriteString(conn, p.reply(req))
}

// client builds a Client pointed at the fake.
func (p *fakePlugin) client() *gimp.Client {
	p.t.Helper()

	addr, ok := p.listener.Addr().(*net.TCPAddr)
	if !ok {
		p.t.Fatalf("unexpected listener address %T", p.listener.Addr())
	}

	return gimp.New(
		gimp.WithHost("127.0.0.1"),
		gimp.WithPort(addr.Port),
	)
}

// nextRequest returns the request the fake most recently received.
func (p *fakePlugin) nextRequest() gimp.Request {
	p.t.Helper()

	select {
	case r := <-p.requests:
		return r
	case <-time.After(5 * time.Second):
		p.t.Fatal("plug-in received no request")

		return gimp.Request{}
	}
}

func TestCallReturnsResults(t *testing.T) {
	p := startFakePlugin(t, func(gimp.Request) string {
		return `{"status":"success","results":{"width":1024,"height":768}}`
	})

	raw, err := p.client().Call(t.Context(), "get_image_metadata", map[string]any{"image_index": 0})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	var got map[string]int
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode results: %v", err)
	}

	if got["width"] != 1024 || got["height"] != 768 {
		t.Errorf("results = %v, want width 1024 height 768", got)
	}

	req := p.nextRequest()
	if req.Type != "get_image_metadata" {
		t.Errorf("command = %q, want get_image_metadata", req.Type)
	}
}

func TestCallSurfacesPluginError(t *testing.T) {
	p := startFakePlugin(t, func(gimp.Request) string {
		return `{"status":"error","error":"no image is open"}`
	})

	_, err := p.client().Call(t.Context(), "auto_levels", nil)
	if err == nil {
		t.Fatal("Call succeeded, want error")
	}

	if !strings.Contains(err.Error(), "no image is open") {
		t.Errorf("error = %q, want it to mention the plug-in message", err)
	}
}

func TestCallSurfacesStructuredError(t *testing.T) {
	p := startFakePlugin(t, func(gimp.Request) string {
		return `{"status":"error","error":{"type":"RuntimeError","message":"boom"}}`
	})

	_, err := p.client().Call(t.Context(), "blur", nil)
	if err == nil {
		t.Fatal("Call succeeded, want error")
	}

	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %q, want it to include the structured payload", err)
	}
}

func TestNilParamsSendEmptyObject(t *testing.T) {
	p := startFakePlugin(t, func(gimp.Request) string {
		return `{"status":"success","results":{}}`
	})

	if _, err := p.client().Call(t.Context(), "get_gimp_info", nil); err != nil {
		t.Fatalf("Call: %v", err)
	}

	// The plug-in always does params.get(...), so params must be an object.
	if got, ok := p.nextRequest().Params.(map[string]any); !ok {
		t.Errorf("params = %#v, want an empty object", got)
	}
}

func TestResponseWithoutCloseIsDecoded(t *testing.T) {
	// The plug-in only closes the connection when auto-disconnect is on.
	// Decoding a single JSON value must not depend on seeing EOF.
	ln, err := listen(t)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}

		var req gimp.Request
		_ = json.NewDecoder(conn).Decode(&req)
		_, _ = io.WriteString(conn, `{"status":"success","results":{"ok":true}}`)
		// Deliberately hold the connection open.
		time.Sleep(2 * time.Second)
		_ = conn.Close()
	}()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address %T", ln.Addr())
	}

	c := gimp.New(gimp.WithHost("127.0.0.1"), gimp.WithPort(addr.Port))

	done := make(chan error, 1)

	go func() {
		_, err := c.Call(t.Context(), "list_images", nil)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Call: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Call blocked waiting for the connection to close")
	}
}

func TestUnreachablePluginIsRecognisable(t *testing.T) {
	// Port 1 on loopback is reserved and not listening.
	c := gimp.New(gimp.WithHost("127.0.0.1"), gimp.WithPort(1),
		gimp.WithDialTimeout(500*time.Millisecond))

	_, err := c.Call(t.Context(), "get_gimp_info", nil)
	if !errors.Is(err, gimp.ErrUnreachable) {
		t.Fatalf("error = %v, want ErrUnreachable", err)
	}

	if !strings.Contains(err.Error(), "Start MCP Server") {
		t.Errorf("error = %q, want it to tell the user how to fix it", err)
	}
}

func TestCallTimeoutIsEnforced(t *testing.T) {
	ln, err := listen(t)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// Accept but never reply, like GIMP wedged mid-render.
		defer conn.Close() //nolint:errcheck // test helper
		time.Sleep(5 * time.Second)
	}()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address %T", ln.Addr())
	}

	c := gimp.New(gimp.WithHost("127.0.0.1"), gimp.WithPort(addr.Port),
		gimp.WithCallTimeout(300*time.Millisecond))

	start := time.Now()

	if _, err := c.Call(context.Background(), "scale_image", nil); err == nil {
		t.Fatal("Call succeeded, want timeout")
	}

	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Call took %s, want it bounded by the call timeout", elapsed)
	}
}

func TestAddrReportsTarget(t *testing.T) {
	c := gimp.New(gimp.WithHost("gimp.local"), gimp.WithPort(9999))
	if got, want := c.Addr(), "gimp.local:9999"; got != want {
		t.Errorf("Addr() = %q, want %q", got, want)
	}
}
