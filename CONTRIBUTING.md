# Contributing

Two Go modules: the root is the MCP server, `plugin/` is the GIMP plug-in.
They are kept apart so the server never pulls in cgo. A `go.work` ties them
together.

```bash
make build            # the MCP server
make build-plugin     # the GIMP plug-in (needs libgimp)
make install-plugin   # build it and install it into GIMP
make test             # both modules
make lint             # golangci-lint over both
make introspect       # snapshot what the running GIMP accepts
make verify-tools     # check the tool definitions against that snapshot
make snapshot         # local goreleaser build of the server
make plugin-snapshot  # local plug-in archive for this host
```

Building the plug-in needs GIMP 3 development files, and it cannot be
cross-compiled, so build on the machine that runs GIMP:

```bash
# Debian/Ubuntu:  apt install libgimp-3.0-dev pkg-config
# macOS:          install GIMP.app; headers ship inside the bundle
# Windows:        MSYS2 UCRT64, pacman -S mingw-w64-ucrt-x86_64-gimp
```

## Adding a tool

Tools are defined in [`internal/server/tools_*.go`](internal/server/), one
file per group of plug-in commands. A tool is three things side by side: a
description constant, an argument struct, and — when an argument has a
non-zero default — a `SetDefaults` method. Add the `addObject` (or
`addImage`) call to the file's `register…Tools` function, then implement the
command in [`plugin/internal/commands/`](plugin/internal/commands/).

```go
type ApplyVignetteInput struct {
	Radius *float64 `json:"radius" jsonschema:"Size of the clear area, 0-3" minimum:"0" maximum:"3" gimp:"gegl:vignette.radius"`
	Shape  *string  `json:"shape" jsonschema:"Vignette shape" enum:"circle,square,diamond,horizontal,vertical" gimp:"gegl:vignette.shape"`
}
```

The JSON names are the wire contract; the plug-in reads those exact keys. The
schema is inferred from the struct: `jsonschema` is the description, `enum`,
`minimum` and `maximum` constrain the value, and the schema's default is
whatever `SetDefaults` assigns, so it is written once.
[`schema.go`](internal/server/schema.go) documents every tag.

## Where each constraint comes from

Every `enum`, `minimum` and `maximum` must say where it came from, or
`make verify-tools` fails. It is one of two things:

- **Something GIMP declares.** The `gimp` tag names the procedure argument or
  operation property the value is passed to, as `owner.name`, and what the
  plug-in divides it by on the way. `gimp:"gimp:hue-saturation.hue/180"`
  says the tool's -180..180 reaches GIMP as -1..1.

  `make verify-tools` checks the argument's enum, bounds and default against
  [`internal/snapshot/gimp.json`](internal/snapshot/gimp.json), a snapshot of
  what the installed GIMP accepts for every procedure and operation the
  plug-in calls. A tool may accept less than GIMP does, never more: GObject
  discards an out-of-range value and runs with its own default, so the call
  would look like it worked. Where GIMP bounds a value, the tool has to as
  well. A `sentinel` tag names a value outside GIMP's range that the plug-in
  reads as "use GIMP's default".

- **This project's choice.** Units, composite tools and the vocabulary of
  tools that pick between procedures are authored here and cannot be derived.
  The `project` tag says why.

The snapshot is read off a running GIMP by `make introspect` rather than out
of documentation, because nothing published carries it: GIMP's reference
documents the procedures in prose, and neither it nor GEGL's lists the
operation properties GIMP's filter configuration exposes. Descriptions may be
written from the reference; constraints never are.

Enum values are GIMP's own names. The plug-in passes them straight through and
the bridge resolves them, so nothing depends on GIMP's numbering and a name
GIMP does not know is refused with the list it does.

## When GIMP is upgraded

Run the `gimp-upgrade` skill in [`.claude/skills/`](.claude/skills/gimp-upgrade/SKILL.md),
or follow it by hand. In short: install the plug-in against the new GIMP,
start it, and run `make introspect`. The diff to `gimp.json` is exactly what
GIMP changed in the parts this server uses; `make verify-tools` then lists
every tool definition the new GIMP no longer honours, and the definitions are
edited to match. Deprecations are not in the snapshot, so the skill also
checks the API reference tarball from <https://download.gimp.org/gimp/> for
the installed version.

## How the plug-in reaches GIMP

Rather than binding libgimp function by function,
[`plugin/internal/gimpbridge`](plugin/internal/gimpbridge) goes through the
PDB: look a procedure up by name, bind its arguments onto a
`GimpProcedureConfig` by name and declared type, run it, convert the results.
That covers GIMP's whole API through one bridge. GEGL filters take a separate
path, since GIMP 3 configures them through an object the PDB does not expose.

libgimp is not thread safe and expects to be used from the thread running the
GLib main loop, so the socket server accepts connections on a goroutine but
every command is marshalled onto the main thread via `g_idle_add`.

## The two menu entries

The plug-in registers two PDB procedures, both under `<Image>/Tools/MCP`:
`plug-in-mcp-server` and `plug-in-mcp-server-stop`.

GIMP runs every procedure in its own process, so the stop entry is not the
process holding the socket and cannot reach its main loop in memory. It
connects to the server and sends `stop_server`, which
[`socket.Server.handle`](plugin/internal/socket/socket.go) answers itself,
ahead of the command registry, so the reply is written before the listener
goes away. The serving process then closes the listener and calls
`g_main_loop_quit`, which returns it to GIMP.

`host` and `port` are ordinary procedure arguments, so GIMP builds the start
dialog from their specs — there is no widget code. That is also why the
plug-in links `gimpui-3.0` and therefore GTK.

## Releasing

Pushing a `v*` tag runs three jobs.

The **server** is cross-compiled and released normally, and GoReleaser
generates its Homebrew cask from the same archives.

The **plug-in** cannot be cross-compiled: cgo against libgimp has to build on
the target platform, so a matrix of five runners each build one target and
append their archive to the same release, selected by `PLUGIN_TARGET` against
[`.goreleaser.plugin.yml`](.goreleaser.plugin.yml).

| Target | Runner | libgimp from |
| --- | --- | --- |
| `linux_amd64` | `ubuntu-latest` + `debian:trixie` | `libgimp-3.0-dev` |
| `linux_arm64` | `ubuntu-24.04-arm` + `debian:trixie` | `libgimp-3.0-dev` |
| `darwin_amd64` | `macos-15-intel` | GIMP.app (Homebrew cask) |
| `darwin_arm64` | `macos-15` | GIMP.app (Homebrew cask) |
| `windows_amd64` | `windows-latest` | MSYS2 UCRT64 |

Reproduce any one locally with
`PLUGIN_TARGET=<target> make plugin-snapshot` (the target must match the
host).

A third job renders the **plug-in's cask**. GoReleaser cannot: each plug-in
target releases from its own runner, so no single run holds both macOS
archives, and a cask needs both checksums at once. The job waits for the
matrix, reads the checksums off the finished release and fills in
[`packaging/gimp-mcp-plugin.rb.tmpl`](packaging/gimp-mcp-plugin.rb.tmpl).

Both casks are pushed to
[ryancurrah/homebrew-tap](https://github.com/ryancurrah/homebrew-tap). That
needs a `HOMEBREW_TAP_TOKEN` repository secret — a PAT with `contents: write`
on the tap, because a job's own `GITHUB_TOKEN` cannot write to another
repository. Prereleases (a `-` in the tag) are skipped by both casks.
