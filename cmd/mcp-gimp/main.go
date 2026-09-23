// Command mcp-gimp serves the GIMP MCP plug-in's command set over MCP stdio.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ryancurrah/mcp-gimp/internal/gimp"
	"github.com/ryancurrah/mcp-gimp/internal/server"
)

// version is overridden at release time via -ldflags.
var version = ""

// config holds the resolved runtime settings.
type config struct {
	host        string
	port        int
	dialTimeout time.Duration
	callTimeout time.Duration
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}

		fmt.Fprintf(os.Stderr, "mcp-gimp: %v\n", err)
		os.Exit(1)
	}
}

// run parses configuration and serves until the context is cancelled.
func run(args []string) error {
	cfg, showVersion, err := parseFlags(args)
	if err != nil {
		return err
	}

	if showVersion {
		fmt.Println(buildVersion())

		return nil
	}

	client := gimp.New(
		gimp.WithHost(cfg.host),
		gimp.WithPort(cfg.port),
		gimp.WithDialTimeout(cfg.dialTimeout),
		gimp.WithCallTimeout(cfg.callTimeout),
	)

	srv, err := server.New(client, buildVersion())
	if err != nil {
		return fmt.Errorf("build server: %w", err)
	}

	// Diagnostics go to stderr; stdout carries the MCP stream.
	fmt.Fprintf(os.Stderr, "mcp-gimp %s serving on stdio, GIMP plug-in at %s\n",
		buildVersion(), client.Addr())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

// parseFlags resolves settings from flags, falling back to environment
// variables so the published container image can be configured without
// rewriting the client's argument list.
func parseFlags(args []string) (config, bool, error) {
	fs := flag.NewFlagSet("mcp-gimp", flag.ContinueOnError)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), `mcp-gimp serves a running GIMP instance over MCP (stdio).

GIMP must be running with the companion plug-in started via
Tools > MCP > Start MCP Server.

Options:
`)
		fs.PrintDefaults()
		_, _ = fmt.Fprintf(fs.Output(), `
Environment:
  GIMP_HOST           same as -gimp-host
  GIMP_PORT           same as -gimp-port
  GIMP_DIAL_TIMEOUT   same as -dial-timeout
  GIMP_CALL_TIMEOUT   same as -call-timeout

Flags take precedence over environment variables.
`)
	}

	var (
		host        = fs.String("gimp-host", envStr("GIMP_HOST", gimp.DefaultHost), "host the GIMP plug-in listens on")
		port        = fs.Int("gimp-port", envInt("GIMP_PORT", gimp.DefaultPort), "port the GIMP plug-in listens on")
		dialTimeout = fs.Duration("dial-timeout", envDur("GIMP_DIAL_TIMEOUT", 10*time.Second), "timeout for connecting to the plug-in")
		callTimeout = fs.Duration("call-timeout", envDur("GIMP_CALL_TIMEOUT", 120*time.Second), "timeout for a single GIMP operation")
		showVersion = fs.Bool("version", false, "print the version and exit")
	)

	if err := fs.Parse(args); err != nil {
		return config{}, false, err
	}

	if *port < 1 || *port > 65535 {
		return config{}, false, fmt.Errorf("invalid port %d", *port)
	}

	return config{
		host:        *host,
		port:        *port,
		dialTimeout: *dialTimeout,
		callTimeout: *callTimeout,
	}, *showVersion, nil
}

// envStr reads a string setting from the environment.
func envStr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}

	return fallback
}

// envInt reads an integer setting from the environment, ignoring junk.
func envInt(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}

	return n
}

// envDur reads a duration setting from the environment, ignoring junk.
func envDur(key string, fallback time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}

	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}

	return d
}

// buildVersion reports the release version, falling back to VCS info from the
// build so `go install`ed binaries still identify themselves.
func buildVersion() string {
	if version != "" {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}

	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}

	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 12 {
			return "dev-" + s.Value[:12]
		}
	}

	return "dev"
}
