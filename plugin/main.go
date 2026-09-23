// Command gimp-mcp-plugin is a GIMP 3 plug-in that serves this GIMP instance
// over a local socket so the mcp-gimp MCP server can drive it.
//
// It is built against libgimp and is started from GIMP's
// Tools > MCP > Start MCP Server menu entry.
package main

import (
	"log"
	"os"

	"github.com/ryancurrah/mcp-gimp/plugin/internal/gimpbridge"
	"github.com/ryancurrah/mcp-gimp/plugin/internal/socket"

	// Registers the command set.
	_ "github.com/ryancurrah/mcp-gimp/plugin/internal/commands"
)

func main() {
	log.SetPrefix("gimp-mcp-plugin: ")
	log.SetFlags(0)

	// The bind address comes from the menu entry now: the plug-in's
	// registration reads GIMP_MCP_BIND_HOST/PORT for the defaults and the
	// dialog lets the user override them per run.
	gimpbridge.OnStart = func(host string, port int) error {
		return socket.New(host, port).Start()
	}

	gimpbridge.OnStop = socket.Shutdown

	os.Exit(gimpbridge.Main(os.Args))
}
