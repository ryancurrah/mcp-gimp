# mcp-gimp

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Drive [GIMP 3](https://www.gimp.org/) from an MCP client. 83 tools covering
canvases, layers, selections, drawing, text, colour adjustments, GEGL filters
and export, plus a typed escape hatch onto GIMP's full procedural database.

The in-GIMP half is a native plug-in written in Go and linked against
**libgimp**, so there is no scripting runtime in the loop.

## How it fits together

```
MCP client  ──stdio──>  mcp-gimp  ──TCP 9877──>  gimp-mcp-plugin  ──libgimp──>  GIMP
            (JSON-RPC)  (server)   (JSON)        (plug-in, in GIMP)
```

Two processes, because each end is launched by something different: your MCP
client spawns the server, and GIMP loads the plug-in.

| | `mcp-gimp` | `gimp-mcp-plugin` |
| --- | --- | --- |
| Role | MCP server | GIMP 3 plug-in |
| Links | nothing (`CGO_ENABLED=0`) | `libgimp-3.0` via cgo |
| Runs | wherever your client is | inside GIMP |
| Ships as | archives + Docker image | per-platform archives |

## Install

### Homebrew

```bash
brew tap ryancurrah/tap

brew install --cask mcp-gimp          # the server
brew install --cask gimp-mcp-plugin   # the plug-in (macOS only)
```

Restart GIMP, then run **Tools > MCP > Start MCP Server**.

The plug-in cask symlinks the executable into GIMP's per-user plug-ins
directory. That directory is named after GIMP's major.minor version, so it
**moves when GIMP is upgraded** and the link has to be remade:

```bash
brew reinstall --cask gimp-mcp-plugin
```

The plug-in cask is macOS-only — Homebrew has no libgimp to build against, so
it can only ship the prebuilt binary, which is linked against
`/Applications/GIMP.app`. On Linux and Windows, install the plug-in by hand.

### The plug-in, by hand

Download the archive for your platform from the
[releases page](../../releases):

| Platform | Archive |
| --- | --- |
| Linux x86_64 | `gimp-mcp-plugin_Linux_x86_64.tar.gz` |
| Linux arm64 | `gimp-mcp-plugin_Linux_arm64.tar.gz` |
| macOS Intel | `gimp-mcp-plugin_Darwin_x86_64.tar.gz` |
| macOS Apple Silicon | `gimp-mcp-plugin_Darwin_arm64.tar.gz` |
| Windows x86_64 | `gimp-mcp-plugin_Windows_x86_64.tar.gz` |

Each unpacks to a `gimp-mcp-plugin/` folder, which is the layout GIMP wants —
drop it straight into the plug-ins directory:

```bash
tar -xzf gimp-mcp-plugin_Darwin_arm64.tar.gz
cp -R gimp-mcp-plugin "$(./scripts/gimp-plugin-dir.sh)/"
```

That directory is named after GIMP's major.minor version and therefore
**moves when GIMP is upgraded**; the active path is shown in
**Edit > Preferences > Folders > Plug-ins**. Full notes, including the macOS
Gatekeeper step, are in [plugin/INSTALL.md](plugin/INSTALL.md).

Restart GIMP, then run **Tools > MCP > Start MCP Server**.

On macOS the archive is unsigned and has an rpath pointing at
`/Applications/GIMP.app`. If GIMP is installed elsewhere, build it yourself
instead.

#### Building it yourself

Needs GIMP 3 development files, and it cannot be cross-compiled, so build on
the machine that runs GIMP:

```bash
# Debian/Ubuntu:  apt install libgimp-3.0-dev pkg-config
# macOS:          install GIMP.app; headers ship inside the bundle
# Windows:        MSYS2 UCRT64, pacman -S mingw-w64-ucrt-x86_64-gimp
make install-plugin
```

### The server, by hand

```bash
go install github.com/ryancurrah/mcp-gimp/cmd/mcp-gimp@latest
```

or take an archive from the [releases page](../../releases), or run the
[Docker image](#docker).

## Configure your client

```json
{
  "mcpServers": {
    "gimp": {
      "command": "mcp-gimp"
    }
  }
}
```

Options, all with `GIMP_`-prefixed environment equivalents:

| Flag | Env | Default | Meaning |
| --- | --- | --- | --- |
| `-gimp-host` | `GIMP_HOST` | `localhost` | where the plug-in listens |
| `-gimp-port` | `GIMP_PORT` | `9877` | its port |
| `-dial-timeout` | `GIMP_DIAL_TIMEOUT` | `10s` | connect timeout |
| `-call-timeout` | `GIMP_CALL_TIMEOUT` | `120s` | per-operation timeout |

The plug-in's own bind address is set with `GIMP_MCP_BIND_HOST` and
`GIMP_MCP_BIND_PORT`, read from GIMP's environment.

## Docker

The image contains the server only — the plug-in must run inside your GIMP.
So the container has to be able to reach GIMP's socket, and that is the whole
story of whether it works:

```bash
# Linux: the container shares the host's network namespace.
docker run --rm -i --network=host ryancurrah/mcp-gimp
```

On **Docker Desktop (macOS/Windows)** `host.docker.internal` resolves to the
host, but the plug-in binds loopback by default, so the connection is refused.
Either bind it somewhere reachable:

```bash
GIMP_MCP_BIND_HOST=0.0.0.0 gimp    # only on a network you trust
```

or forward the port from the host. File paths are unaffected: `open_image` and
`export_image` are resolved by GIMP on the host, so no volumes are needed.

Running the server natively is simpler; the image is for clients that
orchestrate MCP servers as containers.

## Usage

Start with `check_server`. If it reports `connected: false`, GIMP is not
running the plug-in.

```
check_server()
new_canvas(width=1024, height=768, fill="white")
fill_ellipse(x=100, y=100, width=200, height=150, color="#3366cc")
get_state_snapshot(max_size=512)       # look at the canvas, no file written
export_image(file_path="/abs/path/out.png")
```

Two prompts ship with the server: `gimp_best_practices` and
`gimp_iterative_workflow`.

### call_api

Anything without a dedicated tool goes through `call_api`, which runs one PDB
procedure by name:

```
describe_procedure(api_path="gimp-drawable-get-pixel")
# -> drawable (GimpDrawable), x-coord (gint), y-coord (gint)

call_api(api_path="gimp-drawable-get-pixel",
         kwargs={"drawable": 2, "x-coord": 10, "y-coord": 10})
```

Images, layers and fonts are passed as integer ids; `-1` means NULL. GIMP
matches argument names exactly, so use `describe_procedure` rather than
guessing. Reference: <https://developer.gimp.org/api/3.0/libgimp/>

> **`call_api` does not evaluate source code.** The plug-in links libgimp,
> so there is no interpreter inside GIMP to run a snippet in. Anything you
> would write in GIMP's Python-Fu console is expressed as the procedure it
> calls: `Gimp.get_images()` becomes `api_path="gimp-get-images"`.

## Development

```bash
make build            # the MCP server
make build-plugin     # the GIMP plug-in (needs libgimp)
make test             # both modules
make lint             # golangci-lint over both
make generate         # regenerate the tool definitions
make snapshot         # local goreleaser build of the server
make plugin-snapshot  # local plug-in archive for this host
```

Two modules: the root is the server, `plugin/` is the plug-in, kept apart so
the server never pulls in cgo. A `go.work` ties them together.

### Adding a tool

Tool definitions are data. Add an entry to
[`internal/gen/tools.json`](internal/gen/tools.json) and run `make generate`
to regenerate the argument struct, its defaults, the JSON schema and the
registration. Then implement the matching command in
[`plugin/internal/commands/`](plugin/internal/commands/).

The argument names in `tools.json` are the wire contract — the plug-in reads
those exact keys.

### Releasing

Pushing a `v*` tag runs three jobs. The server is cross-compiled and released
normally, and GoReleaser generates its Homebrew cask from the same archives. The plug-in cannot be: cgo against libgimp has to build on the
target platform, so a matrix of five runners each build one target and append
their archive to the same release, selected by `PLUGIN_TARGET` against
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

A third job then renders the plug-in's cask. GoReleaser cannot: each plug-in
target releases from its own runner, so no single run holds both macOS
archives, and a cask needs both checksums at once. The job waits for the
matrix, reads the checksums off the finished release and fills in
[`packaging/gimp-mcp-plugin.rb.tmpl`](packaging/gimp-mcp-plugin.rb.tmpl).

Both casks are pushed to [ryancurrah/homebrew-tap](https://github.com/ryancurrah/homebrew-tap),
which needs `HOMEBREW_TAP_TOKEN` in this repository's Actions secrets — a PAT
with `contents: write` on the tap. The job's own `GITHUB_TOKEN` cannot write to
another repository. Prereleases (a `-` in the tag) are skipped.

### How the plug-in reaches GIMP

Rather than binding libgimp function by function,
[`plugin/internal/gimpbridge`](plugin/internal/gimpbridge) goes through the
PDB: look a procedure up by name, bind its arguments onto a
`GimpProcedureConfig` by name and declared type, run it, convert the results.
That covers GIMP's whole API through one bridge. GEGL filters take a separate
path, since GIMP 3 configures them through an object the PDB does not expose.

libgimp is not thread safe and expects to be used from the thread running the
GLib main loop, so the socket server accepts connections on a goroutine but
every command is marshalled onto the main thread via `g_idle_add`.

## Licence

MIT — see [LICENSE](LICENSE).

GIMP itself is GPL-3.0, but the plug-in links only `libgimp`, which is
LGPL-3.0 and permits linking from differently licensed code.
