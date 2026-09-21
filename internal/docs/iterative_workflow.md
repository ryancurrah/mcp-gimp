# Iterative Workflow with GIMP MCP

## The golden rule

Look at your work after every meaningful change. `get_state_snapshot` returns
the canvas as an image without writing a file, so there is no reason to work
blind.

Building a complex image in one long burst of calls and checking at the end
reliably produces something misshapen, because each mistake gets built on.

## Why this matters

Working without validation goes wrong in predictable ways:

- Elements land in the wrong place because you assumed the canvas size.
- Shapes are drawn on the wrong layer and disappear behind another.
- An earlier selection is still active and silently clips everything after it.
- Proportions drift, and by the time you notice, five things depend on them.

Each of these is obvious the moment you look at the canvas, and invisible if
you do not.

## Phase 1 — Plan before drawing

Find out what you are working with, and decide the layer structure up front.

```
check_server()
list_images()
get_image_metadata()      # never assume the dimensions
```

Sketch the stack, bottom to top. For a character illustration:

- `background` — sky, ground
- `body` — torso and limbs
- `head` — head and ears
- `details` — eyes, nose, mouth
- `texture` — overlay passes

Bottom-to-top order matters: anything you draw is painted over what is below.

## Phase 2 — Create the layers first

```
create_layer(name="background", fill="#87ceeb")
create_layer(name="body", fill="transparent")
create_layer(name="head", fill="transparent")
create_layer(name="details", fill="transparent")
```

Then confirm the stack is what you intended:

```
list_layers()
```

Creating layers as you go tends to produce the wrong order, and reordering
later is more work than planning.

## Phase 3 — Draw incrementally, validating each step

Work one element at a time, on its own layer, and look after each.

```
# Step 1 — body
fill_ellipse(layer_name="body", x=150, y=200, width=200, height=160,
             color="#8b4513")
get_state_snapshot(max_size=512)

# Step 2 — head, positioned relative to the body
fill_ellipse(layer_name="head", x=200, y=120, width=110, height=100,
             color="#8b4513")
get_state_snapshot(max_size=512)

# Step 3 — details, zoomed in to check them properly
fill_ellipse(layer_name="details", x=225, y=150, width=14, height=14,
             color="#000000")
fill_ellipse(layer_name="details", x=265, y=150, width=14, height=14,
             color="#000000")
get_state_snapshot(region={"x": 200, "y": 120, "width": 120, "height": 90})
```

Use the `region` argument to inspect small features. A 512px view of the whole
canvas will not show you whether two eyes line up.

## Phase 4 — Critique honestly

After each snapshot, ask:

- Is it in the right place, at the right size, relative to what is already there?
- Are the proportions right, or did they drift?
- Did it land on the layer I intended?
- Is anything hidden behind something else?
- Is the edge quality right — sharp where it should be sharp?

If the answer is no, fix the cause.

## Fix, do not paint over

When something is wrong, undo it or clear and redraw that layer:

```
undo(steps=1)
```

Painting a correction on top leaves the original underneath. It shows through
at the edges, it is still there when you change the layer's opacity, and every
later fix has to work around it. Because each element is on its own layer, you
can almost always redo one piece without touching the rest.

## Phase 5 — Finish

Verify the whole canvas at a decent size, then save.

```
get_state_snapshot(max_size=1024)
save_xcf(file_path="/abs/path/work.xcf")     # keeps the layer stack
export_image(file_path="/abs/path/final.png")
```

Save the XCF whenever the work might be revisited — flattening on export is
irreversible, and the layered file is what makes another round of edits cheap.

## A note on cost

Snapshots are not free, but they are far cheaper than rebuilding an image
whose proportions went wrong twenty calls ago. Use `max_size` to keep them
small during construction and take one large view at the end.
