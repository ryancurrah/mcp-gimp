// Package gimp speaks the line protocol exposed by the GIMP MCP plug-in.
//
// The plug-in listens on a TCP socket inside GIMP, accepts one JSON request per
// connection, writes a single JSON response and then closes the connection.
// There is no framing beyond that, so a response is read by decoding exactly one
// JSON value off the wire.
package gimp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

// DefaultHost and DefaultPort match the plug-in's built-in listen address.
const (
	DefaultHost = "localhost"
	DefaultPort = 9877
)

// Request is the envelope the plug-in expects.
type Request struct {
	Type   string `json:"type"`
	Params any    `json:"params"`
}

// Response is the envelope the plug-in returns. Results is left raw so callers
// can decode it into whatever shape the individual command produces.
type Response struct {
	Status  string          `json:"status"`
	Results json.RawMessage `json:"results"`
	Error   json.RawMessage `json:"error"`
}

// Client dials the GIMP plug-in. A fresh connection is made for every command,
// mirroring the plug-in's one-request-per-connection behaviour.
type Client struct {
	host string
	port int

	// DialTimeout bounds establishing the TCP connection.
	dialTimeout time.Duration
	// CallTimeout bounds a whole request/response exchange. GIMP renders
	// synchronously, so a large export can legitimately take a while.
	callTimeout time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithHost sets the host the plug-in is reachable on.
func WithHost(h string) Option { return func(c *Client) { c.host = h } }

// WithPort sets the port the plug-in listens on.
func WithPort(p int) Option { return func(c *Client) { c.port = p } }

// WithDialTimeout bounds establishing the TCP connection.
func WithDialTimeout(d time.Duration) Option { return func(c *Client) { c.dialTimeout = d } }

// WithCallTimeout bounds a single request/response exchange.
func WithCallTimeout(d time.Duration) Option { return func(c *Client) { c.callTimeout = d } }

// New builds a Client. With no options it targets the plug-in's defaults.
func New(opts ...Option) *Client {
	c := &Client{
		host:        DefaultHost,
		port:        DefaultPort,
		dialTimeout: 10 * time.Second,
		callTimeout: 120 * time.Second,
	}
	for _, o := range opts {
		o(c)
	}

	return c
}

// Addr reports the address the client dials.
func (c *Client) Addr() string { return net.JoinHostPort(c.host, fmt.Sprint(c.port)) }

// Host reports the configured host.
func (c *Client) Host() string { return c.host }

// Port reports the configured port.
func (c *Client) Port() int { return c.port }

// ErrUnreachable is returned when the plug-in socket cannot be dialled. It is
// worth distinguishing because the remedy is always the same: start the plug-in
// from Tools > MCP > Start MCP Server.
var ErrUnreachable = errors.New("cannot reach the GIMP MCP plug-in")

// Send performs one command exchange. params is marshalled as-is; pass nil for
// commands that take none.
func (c *Client) Send(ctx context.Context, command string, params any) (*Response, error) {
	if params == nil {
		// The plug-in reads params.get(...) throughout, so an empty object is
		// always a valid stand-in for "no arguments".
		params = struct{}{}
	}

	body, err := json.Marshal(Request{Type: command, Params: params})
	if err != nil {
		return nil, fmt.Errorf("encode %s request: %w", command, err)
	}

	ctx, cancel := context.WithTimeout(ctx, c.callTimeout)
	defer cancel()

	dialer := net.Dialer{Timeout: c.dialTimeout}

	conn, err := dialer.DialContext(ctx, "tcp", c.Addr())
	if err != nil {
		return nil, fmt.Errorf("%w at %s: %w (is GIMP running with Tools > MCP > Start MCP Server?)",
			ErrUnreachable, c.Addr(), err)
	}
	defer conn.Close() //nolint:errcheck // response already read; close is best effort

	// Propagate cancellation to the blocking socket calls below.
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()

	// The plug-in accumulates until the buffer parses as JSON; the trailing
	// newline is harmless and matches the reference client.
	if _, err := conn.Write(append(body, '\n')); err != nil {
		return nil, fmt.Errorf("send %s to GIMP: %w", command, err)
	}

	// Decode exactly one JSON value. This returns as soon as the response is
	// complete, whether or not the plug-in closes the connection afterwards.
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("%s timed out after %s: %w", command, c.callTimeout, ctxErr)
		}

		return nil, fmt.Errorf("read %s response from GIMP: %w", command, err)
	}

	return &resp, nil
}

// Call performs an exchange and fails when the plug-in reports a non-success
// status, returning the raw results payload on success.
func (c *Client) Call(ctx context.Context, command string, params any) (json.RawMessage, error) {
	resp, err := c.Send(ctx, command, params)
	if err != nil {
		return nil, err
	}

	if resp.Status != "success" {
		return nil, fmt.Errorf("GIMP reported an error for %s: %s", command, describeError(resp.Error))
	}

	return resp.Results, nil
}

// describeError renders the plug-in's error field, which is sometimes a string
// and sometimes a structured object, as readable text.
func describeError(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "unknown error"
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	return string(raw)
}
