// Package socket serves the plug-in's command protocol over TCP.
//
// The listener runs on its own goroutine, but every command it accepts is
// executed on the GIMP main thread, because libgimp is not thread safe.
package socket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/commands"
	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
	"github.com/ryancurrah/mcp-gimp/plugin/internal/protocol"
)

// DefaultHost and DefaultPort match what the MCP server dials by default.
const (
	DefaultHost = "127.0.0.1"
	DefaultPort = 9877
)

// Executor runs a command body somewhere it is safe to call libgimp.
type Executor func(func())

// Server accepts command connections.
type Server struct {
	host string
	port int
	// exec marshals work onto the GIMP main thread. It is a field so tests
	// can substitute a direct call.
	exec Executor

	mu       sync.Mutex
	listener net.Listener
}

// New builds a server that dispatches commands onto the GIMP main thread.
func New(host string, port int) *Server {
	return &Server{host: host, port: port, exec: gimpbridge.Do}
}

// NewWithExecutor builds a server with a custom dispatcher, for tests.
func NewWithExecutor(host string, port int, exec Executor) *Server {
	return &Server{host: host, port: port, exec: exec}
}

// Addr reports the address the server listens on.
func (s *Server) Addr() string { return net.JoinHostPort(s.host, fmt.Sprint(s.port)) }

// BoundAddr reports the address actually bound, which differs from Addr when
// port 0 was requested.
func (s *Server) BoundAddr() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener == nil {
		return ""
	}

	return s.listener.Addr().String()
}

// Start begins listening and serves connections on a background goroutine.
//
// It returns as soon as the socket is bound so the caller can hand control
// back to the GLib main loop, which is what executes the queued commands.
func (s *Server) Start() error {
	var lc net.ListenConfig

	ln, err := lc.Listen(context.Background(), "tcp", s.Addr())
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.Addr(), err)
	}

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	log.Printf("listening on %s with %d commands", ln.Addr(), len(commands.Names()))

	go s.acceptLoop(ln)

	return nil
}

// Stop closes the listener.
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener != nil {
		_ = s.listener.Close()
		s.listener = nil
	}
}

func (s *Server) Dispatch(req protocol.Request) protocol.Response {
	handler, ok := commands.Lookup(req.Type)
	if !ok {
		return protocol.Failure(fmt.Errorf("unknown command %q", req.Type))
	}

	params, err := commands.Decode(req.Params)
	if err != nil {
		return protocol.Failure(err)
	}

	var (
		results any
		cmdErr  error
	)

	// libgimp must only be touched from the main thread.
	s.exec(func() {
		defer func() {
			// A panic inside a command would otherwise unwind through cgo
			// into the GLib main loop and take GIMP down with it.
			if r := recover(); r != nil {
				cmdErr = fmt.Errorf("command %s panicked: %v", req.Type, r)
			}
		}()

		results, cmdErr = handler(params)
	})

	if cmdErr != nil {
		return protocol.Failure(cmdErr)
	}

	return protocol.Success(results)
}

// acceptLoop serves until the listener is closed.
func (s *Server) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}

		go s.handle(conn)
	}
}

// handle serves one request and closes the connection, matching the
// one-request-per-connection protocol the MCP server expects.
func (s *Server) handle(conn net.Conn) {
	defer conn.Close() //nolint:errcheck // response already written

	var req protocol.Request

	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		if !errors.Is(err, io.EOF) {
			writeResponse(conn, protocol.Failure(fmt.Errorf("malformed request: %w", err)))
		}

		return
	}

	writeResponse(conn, s.Dispatch(req))
}

// Dispatch runs a command on the GIMP main thread and returns its reply.

// writeResponse serialises a reply.
//
// A command's results are an arbitrary value, so encoding can fail. The
// fallback carries only a string, but is spelled out literally in case even
// that cannot be encoded, so the client always receives a valid response.
func writeResponse(conn net.Conn, resp protocol.Response) {
	body, err := json.Marshal(resp)
	if err != nil {
		body = fmt.Appendf(nil, `{"status":"error","error":%q}`,
			fmt.Sprintf("cannot encode response: %v", err))
	}

	if _, err := conn.Write(body); err != nil {
		log.Printf("write response: %v", err)
	}
}
