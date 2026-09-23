---
name: gimp-upgrade
description: Update the mcp-gimp tool definitions after GIMP is upgraded, or when adding or changing a tool argument's enum, bounds or default in internal/server/tools_*.go. Takes a fresh snapshot of what the installed GIMP accepts, reviews the diff, reconciles the tool definitions with it, and checks the API reference for deprecations.
---

# Reconcile mcp-gimp with the installed GIMP

The facts in the tool definitions (`internal/server/tools_*.go`) come from
three places, and each is handled differently:

| Kind of fact | Source of truth | How it is checked |
|---|---|---|
| Argument ranges, defaults and permitted names that GIMP declares | The installed GIMP, via `make introspect` → `internal/snapshot/gimp.json` | `make verify-tools` |
| Deprecations and prose | The API reference tarball for the installed version | This skill, step 5 |
| Units, composite tools, argument names, narrower ranges | This project's design (a `project` tag) | Code review; ask the user |

Never take a range, default or permitted value from prose — not from the API
reference, not from a tool's description, not from memory. Three
export bounds taken from descriptions were wrong because the prose omitted
sentinel values, and two ranges curated by hand were wrong until GIMP was
asked. Ask GIMP.

## 1. Build and start the plug-in against the new GIMP

```bash
make install-plugin
```

Then start GIMP with the plug-in serving. Either use **Tools > MCP > Start MCP
Server** in the GUI, or run headless:

```bash
/Applications/GIMP.app/Contents/MacOS/gimp-console-3.2 -i \
  --batch-interpreter plug-in-script-fu-eval \
  -b '(plug-in-mcp-server RUN-NONINTERACTIVE)' &
```

Adjust the binary name to the new version. Wait until port 9877 answers
(`nc -z 127.0.0.1 9877`). If a previous plug-in process is still running it
will hold the port and answer with EOF; kill it first
(`pkill -f plug-ins/gimp-mcp-plugin`).

## 2. Take the snapshot

```bash
make introspect
```

If it fails with "the plug-in names things this GIMP does not have", GIMP
removed or renamed a procedure or operation the plug-in calls. For each one,
find the replacement in the API reference (step 5), change the plug-in to call
it, rebuild, and repeat from step 1. Do not remove the call without replacing
what it did.

## 3. Review what GIMP changed

```bash
git diff --stat internal/snapshot/gimp.json
git diff internal/snapshot/gimp.json
```

Summarise for the user: new or removed arguments, moved ranges, changed
defaults, added or removed choices. This diff is the authoritative changelog
for the parts of GIMP this server touches.

## 4. Reconcile the tool definitions

```bash
make verify-tools
```

Each failure names the tool argument, the GIMP property it is passed to and
the disagreement. Resolve each one:

- **The tool is wider than GIMP** (range or choices): narrow the tool to what
  GIMP now accepts. If the tool's unit differs from GIMP's (a `/scale` on the
  `gimp` tag), keep the tool's unit and recompute the bound.
- **GIMP bounds something the tool does not**: add the bound the message
  suggests.
- **A choice was renamed**: rename it in the field's `enum` tag and in
  `SetDefaults` — tool enums are GIMP's own names, which the plug-in passes
  through unchanged.
- **A default now falls outside GIMP's range**: pick a new default inside it
  and say why in the commit.
- **The snapshot describes something the plug-in no longer calls, or the
  reverse**: re-run `make introspect`.

Never make a failure go away by deleting a `gimp` tag, by switching it to a
`project` tag, or by dropping a constraint. If a fix would change what a tool
means rather than tracking GIMP — a different unit, a new composite behaviour,
a narrower range chosen for usability — stop and ask the user; those are
design decisions.

When adding or changing an argument with an `enum`, `minimum` or `maximum`,
give its field a source tag:

```go
Hue     float64 `json:"hue" minimum:"-180" maximum:"180" gimp:"gimp:hue-saturation.hue/180"`
Opacity *float64 `json:"opacity" minimum:"0" maximum:"100" gimp:"gimp-layer-set-opacity.opacity"`
Op      string  `json:"operation" enum:"grow,shrink" project:"Picks which gimp-selection-* procedure runs."`
Level   *int    `json:"png_compression" minimum:"-1" maximum:"9" sentinel:"-1" gimp:"file-png-export.compression"`
```

The `gimp` tag is `owner.name/scale`, several separated by spaces; `scale` is
what the plug-in divides by before passing the value on. `sentinel` is a
value outside GIMP's range that the plug-in reads as "use GIMP's default".
Read the plug-in command in `plugin/internal/commands/` to find which
procedure or property an argument reaches and with what arithmetic; the check
then holds the declaration to GIMP. `schema.go` documents every tag.

## 5. Check deprecations and prose in the API reference

Deprecations are not in the snapshot. Download the reference for the
installed version:

```bash
V=3.2.6   # the version recorded in internal/snapshot/gimp.json
curl -LO https://download.gimp.org/gimp/v${V%.*}/api-docs/gimp-api-docs-$V.tar.xz
tar xf gimp-api-docs-$V.tar.xz
```

List the procedures the plug-in calls that the reference marks deprecated. A
PDB name such as `gimp-drawable-curves-spline` is the libgimp function
`gimp_drawable_curves_spline`, and each deprecated function's page carries an
`emblem deprecated` badge and names the function first:

```bash
D=gimp-api-docs-$V/reference/libgimp-3.0
jq -r '.procedures | keys[] | gsub("-"; "_")' internal/snapshot/gimp.json > used.txt
for f in $(grep -l 'emblem deprecated' $D/func.*.html $D/method.*.html); do
  grep -o 'gimp_[a-z0-9_]*' "$f" | head -1
done | grep -xFf used.txt
```

Against GIMP 3.2.6 this prints only `gimp_drawable_curves_spline`, which
`adjust_curves` keeps because its replacement, the `gimp:curves` filter, takes a
GimpCurve object the bridge cannot build yet. Anything else it prints is new:
read that page for the replacement and migrate the call, as the 3.2
adjustments were moved to `gimp:brightness-contrast` and friends. GEGL
operation properties are not in either reference; the snapshot is the only
record of them.

Tool descriptions may be refreshed from the reference's prose, but
constraints never come from it.

## 6. Test and try it

```bash
make test lint
```

Then call each tool whose definition or plug-in code changed against the
running GIMP, including one value the tool should now refuse, and confirm the
refusal names the problem.

## 7. Commit

Commit `gimp.json`, the tool definitions and any plug-in changes together,
and say in the message which GIMP version the snapshot came from and what
changed.
