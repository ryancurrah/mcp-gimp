// Command gimp-mcp-plugin is a GIMP 3 plug-in that serves this GIMP instance
// over a local socket so the mcp-gimp MCP server can drive it.
//
// It is built against libgimp and is started from GIMP's
// Tools > MCP > Start MCP Server menu entry.
package main

import (
	"log"
	"os"
	"strconv"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
	"github.com/ryancurrah/mcp-gimp/plugin/internal/socket"

	// Registers the command set.
	_ "github.com/ryancurrah/mcp-gimp/plugin/internal/commands"
)

func main() {
	log.SetPrefix("gimp-mcp-plugin: ")
	log.SetFlags(0)

	host := envStr("GIMP_MCP_BIND_HOST", socket.DefaultHost)
	port := envInt("GIMP_MCP_BIND_PORT", socket.DefaultPort)

	srv := socket.New(host, port)

	// GIMP calls this once the user runs the menu entry. It must not block;
	// the GLib main loop it returns into is what executes queued commands.
	gimpbridge.OnStart = func() {
		if err := srv.Start(); err != nil {
			log.Printf("cannot start server: %v", err)
		}
	}

	os.Exit(gimpbridge.Main(os.Args))
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
