# mcp-gimp

An MCP server (Go, repo root) that drives GIMP through a plug-in
(`plugin/`, cgo against libgimp). CONTRIBUTING.md covers building and
releasing; this file covers keeping the tool definitions correct.

Push directly to `main`; no feature branches or PRs.

## Keeping the tool definitions correct

Tools are hand-written Go in `internal/server/tools_*.go`, grouped the way
the plug-in's commands in `plugin/internal/commands/` are. Each tool has a
description constant, an argument struct and, if any argument has a non-zero
default, a `SetDefaults` method. `internal/server/schema.go` documents every
struct tag.

### Where facts come from

- **A range, default or permitted value comes from GIMP, never from prose.**
  Don't take it from the API reference, a tool's description or memory. Ranges
  curated from descriptions and by hand have been wrong several times here.
  Ask the installed GIMP: `describe_procedure` and `describe_operation`, or
  read `internal/snapshot/gimp.json`.
- **Every `enum`, `minimum` and `maximum` names its source,** or the tests
  fail:
  - `gimp:"owner.name/scale"`: the procedure argument or GEGL operation
    property the value is passed to (operations are the names with a colon),
    and what the plug-in divides it by on the way. Separate several with
    spaces.
  - `project:"reason"`: a constraint this server chose rather than GIMP.
  - `sentinel:"-1"`: a value outside GIMP's range that the plug-in reads as
    "use GIMP's default".

  To find which procedure or property an argument reaches, and with what
  arithmetic, read the plug-in command.
- **A tool may accept less than GIMP does, never more.** GObject discards an
  out-of-range value and runs with its default, so the call looks like it
  worked. Where GIMP bounds a value, the tool must declare that bound too.
- **Enum values are GIMP's own names.** The plug-in passes them straight
  through and the bridge resolves them. Never map names to GIMP's enum
  numbers in Go; that is how the GIMP 2→3 fill-type renumbering bug happened.
- **Defaults live only in `SetDefaults`.** The schema reads them from there.

### Checks

```bash
make verify-tools   # tool definitions vs internal/snapshot/gimp.json (no GIMP needed; part of make test)
make test lint      # both modules
make introspect     # refresh the snapshot from the running GIMP
```

- **Plug-in calls a new procedure or operation:** run `make introspect` and
  commit the snapshot. `schema_test.go` fails when the plug-in and the snapshot
  disagree.
- **A test fails:** fix the definition or the plug-in. Never delete a `gimp`
  tag, switch it to `project`, or drop a constraint to make a test pass. If
  the fix would change what a tool means (units, behaviour, a narrower range
  chosen for usability), ask the user.
- **GIMP was upgraded:** follow the `gimp-upgrade` skill
  (`.claude/skills/gimp-upgrade/SKILL.md`). It covers the snapshot diff,
  reconciling the definitions, and the deprecation check against the API
  reference tarball.

### Silent failures to watch for

- `Params.String/Float/Int/Bool` in the plug-in return their fallback when the
  key is missing *or* has the wrong JSON type. A tool that declares an
  argument's type differently from how the plug-in reads it is silently
  ignored.
- The bridge refuses out-of-range numbers and unknown names for both PDB
  arguments and GEGL properties. Keep it that way: don't add clamping or
  fallbacks that hide a bad value.
- A plug-in error that is discarded (`_ = run(...)`, or `if err == nil` with no
  else) hides missing procedures. Three such calls survived the GIMP 3 port
  unnoticed.
- `gimp_choice_list_nicks` returns a list GIMP owns. Freeing it crashes the
  plug-in on the next describe. Check ownership in the API reference before
  freeing anything libgimp returns.

### Trying a change live

Unit tests cannot reach GIMP, so check each changed tool against a running
GIMP, including one value it should refuse.

- `restart_server` restarts only the socket server, not the plug-in binary.
  After `make install-plugin`, restart GIMP itself.
- Kill the old plug-in process too (`pkill -f plug-ins/gimp-mcp-plugin`),
  because it keeps port 9877 open after GIMP exits and answers with EOF.
- Headless: `gimp-console-3.2 -i --batch-interpreter plug-in-script-fu-eval -b
  '(plug-in-mcp-server RUN-NONINTERACTIVE)'`.
- If the user has the GIMP GUI open, ask before restarting it.
