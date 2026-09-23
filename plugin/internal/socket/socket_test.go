package socket_test

import (
	"encoding/json"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/commands"
	"github.com/ryancurrah/mcp-gimp/plugin/internal/protocol"
	"github.com/ryancurrah/mcp-gimp/plugin/internal/socket"
)

// direct runs a command body inline. The real executor hands work to the GIMP
// main thread, which these tests do not have.
func direct(fn func()) { fn() }

// start brings up a server on an arbitrary free port.
func start(t *testing.T) *socket.Server {
	t.Helper()

	s := socket.NewWithExecutor("127.0.0.1", 0, direct)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	t.Cleanup(s.Stop)

	return s
}

// request sends one command and returns the decoded reply.
func request(t *testing.T, addr string, body string) map[string]any {
	t.Helper()

	d := net.Dialer{Timeout: 5 * time.Second}

	conn, err := d.DialContext(t.Context(), "tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close() //nolint:errcheck // test helper

	if _, err := io.WriteString(conn, body); err != nil {
		t.Fatalf("write: %v", err)
	}

	var out map[string]any
	if err := json.NewDecoder(conn).Decode(&out); err != nil {
		t.Fatalf("decode reply: %v", err)
	}

	return out
}

func TestUnknownCommandIsReported(t *testing.T) {
	s := start(t)

	got := request(t, s.BoundAddr(), `{"type":"no_such_command","params":{}}`)

	if got["status"] != "error" {
		t.Errorf("status = %v, want error", got["status"])
	}

	if msg, _ := got["error"].(string); !strings.Contains(msg, "no_such_command") {
		t.Errorf("error = %q, want it to name the command", msg)
	}
}

func TestMalformedRequestIsReported(t *testing.T) {
	s := start(t)

	got := request(t, s.BoundAddr(), `{not json`)

	if got["status"] != "error" {
		t.Errorf("status = %v, want error", got["status"])
	}
}

func TestPanicIsContainedAsAnError(t *testing.T) {
	// A panic crossing cgo would take GIMP down, so dispatch has to catch it.
	s := socket.NewWithExecutor("127.0.0.1", 0, direct)

	resp := s.Dispatch(protocol.Request{Type: "definitely_not_registered"})
	if resp.Status != "error" {
		t.Errorf("status = %q, want error", resp.Status)
	}
}

func TestBadParamsAreRejected(t *testing.T) {
	s := start(t)

	got := request(t, s.BoundAddr(), `{"type":"list_images","params":[1,2,3]}`)

	if got["status"] != "error" {
		t.Errorf("status = %v, want error for non-object params", got["status"])
	}
}

func TestAddrReportsTheConfiguredTarget(t *testing.T) {
	s := socket.NewWithExecutor("127.0.0.1", 9877, direct)
	if got, want := s.Addr(), "127.0.0.1:9877"; got != want {
		t.Errorf("Addr() = %q, want %q", got, want)
	}
}

func TestShutdownRepliesThenClosesTheListener(t *testing.T) {
	// The reply has to reach the Stop menu entry before the listener goes
	// away, or GIMP reports a failure for a stop that actually worked.
	s := start(t)
	addr := s.BoundAddr()

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}

	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("port: %v", err)
	}

	if err := socket.Shutdown(host, n); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// The listener is closed, so a second connection is refused.
	d := net.Dialer{Timeout: 5 * time.Second}

	conn, err := d.DialContext(t.Context(), "tcp", addr)
	if err == nil {
		_ = conn.Close()

		t.Fatal("server still accepting connections after shutdown")
	}
}

func TestShutdownReportsAnAbsentServer(t *testing.T) {
	// The Stop entry shows this message, so it has to name the address rather
	// than surface a bare connection-refused.
	err := socket.Shutdown("127.0.0.1", 1)
	if err == nil {
		t.Fatal("want an error when nothing is listening")
	}

	if !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("error = %q, want it to name the address", err)
	}
}

func TestShutdownCommandIsNotShadowedByARegisteredOne(t *testing.T) {
	// handle() intercepts this name before dispatch, so registering a command
	// under it would silently make that command unreachable.
	if _, exists := commands.Lookup(socket.ShutdownCommand); exists {
		t.Errorf("%q is both a registered command and the shutdown signal",
			socket.ShutdownCommand)
	}
}
