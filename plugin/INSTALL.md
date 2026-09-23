# Installing the GIMP MCP plug-in

This archive contains a `gimp-mcp-plugin` folder. GIMP loads a plug-in from a
directory named after its executable, so the folder goes in as-is.

## Where it goes

GIMP's per-user plug-ins directory is named after its **major.minor** version,
and a new one is created on each minor upgrade — so this path moves when GIMP
is updated. The active path is shown in
**Edit > Preferences > Folders > Plug-ins**.

| Platform | Directory |
| --- | --- |
| Linux | `~/.config/GIMP/<VER>/plug-ins/` |
| Linux (Snap) | `~/snap/gimp/current/.config/GIMP/<VER>/plug-ins/` |
| macOS | `~/Library/Application Support/GIMP/<VER>/plug-ins/` |
| Windows | `%APPDATA%\GIMP\<VER>\plug-ins\` |

## Install

Linux and macOS:

```bash
tar -xzf gimp-mcp-plugin_*.tar.gz

# macOS; on Linux use ~/.config/GIMP
DEST="$HOME/Library/Application Support/GIMP/3.0/plug-ins"

mkdir -p "$DEST"
cp -R gimp-mcp-plugin "$DEST/"
chmod +x "$DEST/gimp-mcp-plugin/gimp-mcp-plugin"
```

Windows (PowerShell):

```powershell
tar -xzf gimp-mcp-plugin_Windows_x86_64.tar.gz
Copy-Item -Recurse gimp-mcp-plugin "$env:APPDATA\GIMP\3.0\plug-ins\"
```

Replace `3.0` with the version directory GIMP actually reports.

The executable bit matters on Linux and macOS: GIMP silently ignores a
plug-in it cannot execute.

## Run it

Restart GIMP, then click **Tools > MCP > Start MCP Server** in GIMP's menu
bar. A dialog offers the address to bind, defaulting to `127.0.0.1:9877` or to
whatever `GIMP_MCP_BIND_HOST` and `GIMP_MCP_BIND_PORT` were set to when GIMP
was launched. GIMP then confirms the address it is listening on.

<img src="https://raw.githubusercontent.com/ryancurrah/mcp-gimp/main/docs/images/tools-menu.png" alt="GIMP's Tools menu with the MCP submenu open, showing Start MCP Server and Stop MCP Server" width="560">
<img src="https://raw.githubusercontent.com/ryancurrah/mcp-gimp/main/docs/images/start-options.png" alt="The Start MCP Server dialog: Host 127.0.0.1, Port 9877, with Cancel and Start buttons" width="444">

**Tools > MCP > Stop MCP Server** closes it again.

Point the `mcp-gimp` MCP server at it and call `check_server` to confirm.

## macOS: unsigned binary

The download is unsigned, so Gatekeeper will block it on first run. Clear the
quarantine flag after copying:

```bash
xattr -dr com.apple.quarantine "$DEST/gimp-mcp-plugin"
```

## Troubleshooting

The menu entry is missing:

- The folder name and the executable name must match exactly
  (`gimp-mcp-plugin/gimp-mcp-plugin`).
- Check the executable bit.
- Confirm you used the plug-ins directory from Preferences.
- Start GIMP from a terminal; it reports plug-ins it failed to load.

This plug-in is built against GIMP 3 and will not load in GIMP 2.10.
