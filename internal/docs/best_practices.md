# GIMP MCP Best Practices & Recipes

This server drives GIMP 3 through a plug-in built against libgimp. There is no
Python interpreter inside GIMP: you call tools, and `call_api` runs procedures
from GIMP's Procedural Database (PDB) by name. Nothing here evaluates source
code.

## Start here

Call `check_server` first. If it reports `connected: false`, GIMP is not
running the plug-in — open GIMP and run **Tools > MCP > Start MCP Server**.

Then establish what you are working with:

```
check_server()
list_images()              # what is open, and the index of each
get_image_metadata()       # dimensions, colour mode, layer stack
```

Most tools take `image_index` (0 is the most recently touched image) and
`layer_name` or `layer_index`. Omit the layer and the active one is used.

## Prefer the dedicated tools

There is a tool for nearly everything. Reach for `call_api` only when none
fits. The dedicated tools set up GIMP's context correctly, flush the display
and return structured results.

```
fill_ellipse(x=100, y=100, width=80, height=60, color="#3366cc")
```

is better than assembling the same thing from three `call_api` calls.

## Filling shapes

Fill in one step with the fill tools. They select the shape, fill it and clear
the selection so nothing is left selected behind you.

```
fill_rectangle(x=20, y=20, width=120, height=80, color="#ff8800")
fill_ellipse(x=180, y=40, width=100, height=100, color="#22aa55")
```

`draw_rectangle` and `draw_ellipse` stroke an outline instead; use
`line_width` to set its weight. Do not try to fill a shape by stroking it
repeatedly — the result has seams and soft edges.

## Curves

Anything curved is an SVG path. Give the path tools a `d` string in image
pixels and GIMP does the geometry; the temporary path is removed afterwards.

```
draw_path(d="M 100 300 C 150 100 350 100 400 300", width=8, color="#0044cc")
fill_path(d="M 200 200 Q 300 50 400 200 Z", color="#dd5500")
```

To outline a curved shape, select it and fill twice:

```
select_path(d="M 60 360 C 120 250 220 250 280 360 Z")
modify_selection(operation="grow", amount=6)
fill_selection(color="#222222")
modify_selection(operation="shrink", amount=6)
fill_selection(color="#88cc88")
select_none()
```

`keep_path=True` leaves the path in the Paths dockable and returns its
`path_id`; `list_paths` and `path_to_selection` work with paths drawn in the
GUI too.

## Colors

Every colour argument takes a CSS string:

```
"white"  "black"  "transparent"
"#ff5733"
"rgb(100, 200, 50)"
"rgba(100, 200, 50, 0.5)"
```

`set_colors` changes the foreground and background that later drawing
operations inherit.

## Selections

Selections persist until you clear them, and every drawing operation is
clipped to the active selection. This is the single most common cause of
"my edit did nothing".

```
select_rectangle(x=50, y=50, width=100, height=100)
fill_selection(color="#ff00ff")
select_none()                      # always clean up
```

Keep edges sharp unless you specifically want a soft transition. Feathering
is off by default; turn it on deliberately with `feather=true` and
`feather_radius`.

## Layers

Plan the stack before you draw, and keep each element on its own layer so you
can fix one thing without repainting the rest.

```
create_layer(name="background", fill="#87ceeb")
create_layer(name="subject", fill="transparent")
create_layer(name="details", fill="transparent")
```

Then target each one explicitly:

```
fill_ellipse(layer_name="subject", x=100, y=100, width=200, height=150,
             color="#8b4513")
```

`list_layers` shows the current stack. Layers are listed top first, which is
the order `layer_index` uses.

## Verify as you go

`get_state_snapshot` returns a PNG of the current canvas without writing a
file. Use it after each meaningful step rather than at the end.

```
fill_ellipse(...)
get_state_snapshot(max_size=512)                            # look at it
get_state_snapshot(region={"x": 180, "y": 40,
                           "width": 120, "height": 120})    # zoom in
```

If something is wrong, fix the cause — undo it or repaint that layer. Painting
over a mistake leaves the original underneath and compounds.

## Exporting

`export_image` writes a raster file; `save_xcf` preserves the layer stack and
is what you want if the work will be edited again.

```
save_xcf(file_path="/abs/path/work.xcf")
export_image(file_path="/abs/path/out.png")
export_image(file_path="/abs/path/out.jpg", format="jpeg", quality=85)
```

Paths are resolved by GIMP, so they must be absolute and on the machine GIMP
is running on.

## Using call_api

`call_api` runs one PDB procedure. GIMP matches argument names exactly and
rejects unknown ones, so look them up first:

```
describe_procedure(api_path="gimp-drawable-get-pixel")
# -> drawable (GimpDrawable), x-coord (gint), y-coord (gint)

call_api(api_path="gimp-drawable-get-pixel",
         kwargs={"drawable": 2, "x-coord": 10, "y-coord": 10})
```

Conventions:

- Images, layers, drawables and fonts are integer ids — the same ids the other
  tools return.
- Pass `-1` for an object argument that should be NULL, such as a layer's
  `parent`.
- Procedure names use dashes: `gimp-image-get-layers`, not
  `gimp_image_get_layers`.

If you know GIMP's Python-Fu console, the translation is direct:

| Python-Fu                  | call_api                                            |
| -------------------------- | --------------------------------------------------- |
| `Gimp.get_images()`        | `api_path="gimp-get-images"`                          |
| `image.get_layers()`       | `api_path="gimp-image-get-layers", kwargs={"image":1}`|
| `Gimp.displays_flush()`    | `api_path="gimp-displays-flush"`                      |

Full procedure reference: https://developer.gimp.org/api/3.0/libgimp/

## Things that will trip you up

- **A leftover selection** silently clips everything. Call `select_none` when
  you are done with one.
- **Wrong layer.** If an edit seems to vanish, check `list_layers` — you may be
  painting on a hidden layer or under another one.
- **Relative paths.** GIMP resolves paths itself; always pass absolute ones.
- **Filling a layer with no alpha.** `create_layer` gives you alpha; a
  flattened background may not have it, so "transparent" will not take.
- **Assuming context.** The user can change the foreground colour or brush in
  the GIMP UI at any time. `get_context_state` tells you what is actually set.
