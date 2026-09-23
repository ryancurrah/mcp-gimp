# mcp-gimp

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Drive [GIMP 3](https://www.gimp.org/) from an MCP client. 88 tools covering
canvases, layers, selections, drawing, text, colour adjustments, GEGL filters
and export, plus a typed escape hatch onto GIMP's full procedural database.

## Quick start

**1. Install the MCP server and the GIMP plug-in.** On macOS:

```bash
brew tap ryancurrah/tap
brew install --cask mcp-gimp gimp-mcp-plugin
```

On Linux and Windows, see [Installing elsewhere](#installing-elsewhere).

**2. Start the plug-in inside GIMP.** Restart GIMP, then, in GIMP's own
menu bar, click **Tools > MCP > Start MCP Server**. A dialog offers the
address to bind — the defaults are fine — and GIMP confirms
`MCP server listening on 127.0.0.1:9877`. Leave GIMP running.

**3. Register the server with your MCP client.** In Claude Code:

```bash
claude mcp add gimp --scope user -- mcp-gimp
```

`--scope user` registers it once for every project. Drop it to add the server
to the current project only.

Any other client takes the same thing as config:

```json
{
  "mcpServers": {
    "gimp": {
      "command": "mcp-gimp"
    }
  }
}
```

**4. Confirm the client can reach GIMP.** Ask it to run `check_server`. If
that returns `connected: true`, you're done — try:

```
new_canvas(width=1024, height=768, fill="white")
fill_ellipse(x=100, y=100, width=200, height=150, color="#3366cc")
export_image(file_path="/abs/path/out.png")
```

## Why there are two pieces

```
MCP client  ──stdio──>  mcp-gimp  ──TCP 9877──>  gimp-mcp-plugin  ──libgimp──>  GIMP
            (JSON-RPC)  (server)   (JSON)        (plug-in, in GIMP)
```

Your MCP client launches the **server**; GIMP loads the **plug-in**. Different
things start each one, so they ship separately and you install both. The
plug-in is native code linked against libgimp — there is no scripting runtime
in the loop.

## Troubleshooting

**`check_server` says `connected: false`.** GIMP isn't running the plug-in.
Start GIMP and click **Tools > MCP > Start MCP Server** in its menu bar.

**Nothing happened when I clicked Start MCP Server.** It reports through
GIMP's own message handling, which **Edit > Preferences > Interface > Message
Handling** may have pointed at the error console rather than a dialog. Either
way, `check_server` is the real test.

**It says the address is already in use.** The server is already running —
there is one per GIMP, not one per click. Use **Tools > MCP > Stop MCP
Server** first, or just leave the running one alone.

**There's no MCP submenu under GIMP's Tools menu.** GIMP didn't load the
plug-in:

- Restart GIMP — plug-ins are only scanned at startup.
- If you upgraded GIMP since installing, its plug-ins directory moved. On
  Homebrew, `brew reinstall --cask gimp-mcp-plugin`; otherwise install it
  again in the new directory.
- Check you used the directory shown in
  **Edit > Preferences > Folders > Plug-ins**.
- This plug-in needs GIMP 3. It will not load in GIMP 2.10.

More detail, including the macOS Gatekeeper step, is in
[plugin/INSTALL.md](plugin/INSTALL.md).

## Installing elsewhere

### The plug-in

Set `ARCHIVE` to your platform, and `DEST` to the path GIMP shows under
**Edit > Preferences > Folders > Plug-ins**:

| Platform | `ARCHIVE` |
| --- | --- |
| Linux x86_64 | `gimp-mcp-plugin_Linux_x86_64.tar.gz` |
| Linux arm64 | `gimp-mcp-plugin_Linux_arm64.tar.gz` |
| macOS Intel | `gimp-mcp-plugin_Darwin_x86_64.tar.gz` |
| macOS Apple Silicon | `gimp-mcp-plugin_Darwin_arm64.tar.gz` |
| Windows x86_64 | `gimp-mcp-plugin_Windows_x86_64.tar.gz` |

```bash
ARCHIVE=gimp-mcp-plugin_Linux_x86_64.tar.gz
DEST=~/.config/GIMP/3.0/plug-ins

mkdir -p "$DEST"
curl -fL "https://github.com/ryancurrah/mcp-gimp/releases/latest/download/$ARCHIVE" \
  | tar -xzf - -C "$DEST"
```

On Windows, in PowerShell:

```powershell
$archive = "gimp-mcp-plugin_Windows_x86_64.tar.gz"
$dest = "$env:APPDATA\GIMP\3.0\plug-ins"

New-Item -ItemType Directory -Force -Path $dest | Out-Null
curl.exe -fL -o $archive "https://github.com/ryancurrah/mcp-gimp/releases/latest/download/$archive"
tar -xzf $archive -C $dest
```

Restart GIMP, then click **Tools > MCP > Start MCP Server** in GIMP's
menu bar.

The macOS binary is unsigned and expects GIMP at `/Applications/GIMP.app`.

### The server

```bash
go install github.com/ryancurrah/mcp-gimp/cmd/mcp-gimp@latest
```

Or grab a binary — same idea, with `mcp-gimp_Linux_x86_64.tar.xz`,
`mcp-gimp_Darwin_arm64.tar.xz` and so on from the
[releases page](../../releases):

```bash
curl -fL "https://github.com/ryancurrah/mcp-gimp/releases/latest/download/mcp-gimp_Linux_x86_64.tar.xz" \
  | tar -xJf - --strip-components=1
sudo install -m 755 mcp-gimp /usr/local/bin/
```

Or run the [Docker image](#docker).

## Settings

You usually need none of these. All have `GIMP_`-prefixed environment
equivalents:

| Flag | Env | Default | Meaning |
| --- | --- | --- | --- |
| `-gimp-host` | `GIMP_HOST` | `localhost` | where the plug-in listens |
| `-gimp-port` | `GIMP_PORT` | `9877` | its port |
| `-dial-timeout` | `GIMP_DIAL_TIMEOUT` | `10s` | connect timeout |
| `-call-timeout` | `GIMP_CALL_TIMEOUT` | `120s` | per-operation timeout |

The plug-in's bind address is chosen in the dialog that **Start MCP Server**
opens. `GIMP_MCP_BIND_HOST` and `GIMP_MCP_BIND_PORT`, read from GIMP's
environment, set what that dialog starts out with — useful for launching GIMP
preconfigured:

```bash
GIMP_MCP_BIND_PORT=9999 gimp
```

Change the port there and the MCP server needs `-gimp-port` to match.

**Tools > MCP > Stop MCP Server** closes the socket. It reaches the running
server over the same port, so if you started on a non-default one, pass it the
same port — from Script-Fu, `(plug-in-mcp-server-stop RUN-NONINTERACTIVE
"127.0.0.1" 9999)`.

## Usage

Start with `check_server`. Then work the canvas:

```
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

## Contributing

Building from source, adding a tool and how the plug-in works internally are
in [CONTRIBUTING.md](CONTRIBUTING.md).

## Licence

MIT — see [LICENSE](LICENSE).

GIMP itself is GPL-3.0, but the plug-in links only `libgimp`, which is
LGPL-3.0 and permits linking from differently licensed code.
