#ifndef MCP_BRIDGE_H
#define MCP_BRIDGE_H

#include <libgimp/gimp.h>

/* Plug-in registration ---------------------------------------------------- */

/* The bind address the menu entries default to, overridden by
 * GIMP_MCP_BIND_HOST and GIMP_MCP_BIND_PORT. These mirror socket.DefaultHost
 * and socket.DefaultPort on the Go side, which the MCP server also dials. */
#define MCP_DEFAULT_HOST "127.0.0.1"
#define MCP_DEFAULT_PORT 9877

GType mcp_plug_in_get_type (void);

/* mcp_main runs the GIMP plug-in protocol. It returns when GIMP is done with
 * the plug-in process. */
int mcp_main (int argc, char **argv);

/* Main-loop dispatch ------------------------------------------------------ */

/* mcp_idle_dispatch schedules handle to run on the thread owning the GLib main
 * loop. libgimp is not thread safe, so every GIMP call has to go through here.
 */
void mcp_idle_dispatch (guintptr handle);

/* mcp_quit_main_loop stops the loop started by the plug-in run callback. */
void mcp_quit_main_loop (void);

/* Procedure introspection ------------------------------------------------- */

/* mcp_lookup_procedure resolves a PDB procedure by name, or NULL. */
GimpProcedure *mcp_lookup_procedure (const char *name);

/* mcp_procedure_arg_count reports how many arguments a procedure takes. */
int mcp_procedure_arg_count (GimpProcedure *procedure);

/* mcp_procedure_arg_name and mcp_procedure_arg_type describe argument i.
 * The returned strings are owned by GLib and must not be freed. */
const char *mcp_procedure_arg_name (GimpProcedure *procedure, int i);
const char *mcp_procedure_arg_type (GimpProcedure *procedure, int i);

/* mcp_procedure_arg_is_object reports whether argument i takes a GObject,
 * which is how GIMP passes images, layers, channels and paths. Such arguments
 * are bound from an integer id rather than a number. */
int mcp_procedure_arg_is_object (GimpProcedure *procedure, int i);

/* Argument binding -------------------------------------------------------- */

GimpProcedureConfig *mcp_create_config (GimpProcedure *procedure);

void mcp_set_int    (GimpProcedureConfig *config, const char *name, gint64 v);
void mcp_set_uint   (GimpProcedureConfig *config, const char *name, guint64 v);
void mcp_set_double (GimpProcedureConfig *config, const char *name, double v);
void mcp_set_bool   (GimpProcedureConfig *config, const char *name, int v);
/* mcp_set_string binds text: a string, the name of one of a GimpChoice's
 * options, or an enum value's nick. It returns a message the caller must free
 * when the argument does not take that name, or NULL. */
char *mcp_set_string (GimpProcedureConfig *config, const char *name, const char *v);
void mcp_set_enum   (GimpProcedureConfig *config, const char *name, int v);

/* mcp_check_number validates a number against the argument's declared range
 * before it is set, returning a message the caller must free, or NULL when
 * the value is acceptable. */
char *mcp_check_number (GimpProcedureConfig *config, const char *name, double v);

/* mcp_set_image and mcp_set_item bind objects by their GIMP id. They return 0
 * when the id does not resolve. */
int mcp_set_image (GimpProcedureConfig *config, const char *name, gint32 id);
int mcp_set_item  (GimpProcedureConfig *config, const char *name, gint32 id);

/* mcp_set_object_null binds NULL to an object argument, which is how GIMP
 * spells "no parent" and similar optional objects. */
void mcp_set_object_null (GimpProcedureConfig *config, const char *name);

/* mcp_set_file binds a filesystem path to a GFile argument. */
void mcp_set_file (GimpProcedureConfig *config, const char *name, const char *path);

/* mcp_set_color parses a CSS colour string and binds it. Returns 0 on a parse
 * failure. */
int mcp_set_color (GimpProcedureConfig *config, const char *name, const char *css);

/* mcp_set_item_array binds a list of drawables/items by id. Returns 0 when any
 * id fails to resolve. */
int mcp_set_item_array (GimpProcedureConfig *config, const char *name,
                        const gint32 *ids, int n);

/* mcp_set_double_array binds a float array argument. */
void mcp_set_double_array (GimpProcedureConfig *config, const char *name,
                           const double *values, int n);

/* Invocation -------------------------------------------------------------- */

/* mcp_run_config executes the procedure. On failure it returns NULL and sets
 * *err to a newly allocated message the caller must free. */
GimpValueArray *mcp_run_config (GimpProcedure       *procedure,
                                GimpProcedureConfig *config,
                                char               **err);

/* GEGL filters ------------------------------------------------------------ */

/* mcp_apply_gegl applies a GEGL operation to a drawable and merges it down.
 *
 * GIMP 3 exposes its filters as GEGL operations configured through a
 * GimpDrawableFilterConfig object, which the PDB does not reach, so the whole
 * build-configure-merge cycle happens here. Numeric property values are
 * supplied as doubles and coerced to whatever type the operation declares.
 *
 * A property that names one of a fixed set of choices is passed through the
 * text arrays instead. GIMP 3 re-declares a GEGL enum property as a
 * GimpChoice, whose value is a string, so such a property cannot be reached
 * as a number at all.
 *
 * Returns 0 on failure and sets *err to a message the caller must free.
 */
int mcp_apply_gegl (gint32        drawable_id,
                    const char   *operation,
                    const char  **names,
                    const double *values,
                    int           n,
                    const char  **text_names,
                    const char  **text_values,
                    int           text_n,
                    char        **err);

/* Pixel access ------------------------------------------------------------ */

/* mcp_read_pixels copies a rectangle of a drawable, in the drawable's own
 * coordinates, into out as straight R'G'B'A floats in the drawable's colour
 * space, row by row. out holds width * height * 4 floats. Outside the
 * drawable the nearest edge pixel is repeated.
 *
 * Returns 0 on failure and sets *err to a message the caller must free. */
int mcp_read_pixels (gint32  drawable_id,
                     int     x,
                     int     y,
                     int     width,
                     int     height,
                     float  *out,
                     char  **err);

/* mcp_write_pixels replaces a rectangle of a drawable with pixels in the
 * format mcp_read_pixels produces. It goes through the drawable's shadow
 * buffer, as a filter does, so the change is one undo step and an active
 * selection limits it.
 *
 * Returns 0 on failure and sets *err to a message the caller must free. */
int mcp_write_pixels (gint32       drawable_id,
                      int          x,
                      int          y,
                      int          width,
                      int          height,
                      const float *in,
                      char       **err);

/* mcp_describe_op lists what an operation will accept, one property per line:
 *
 *   name \t type \t min \t max \t default \t choices \t blurb
 *
 * Read off the GimpDrawableFilterConfig, which is what this plug-in
 * configures, and so reports the version of GIMP actually installed. Nothing
 * published records this: the libgimp reference documents the procedures and
 * GEGL's documents libgegl, neither the operation catalogue.
 *
 * Returns a string the caller must free, or NULL with *err set.
 */
char *mcp_describe_op (gint32       drawable_id,
                       const char  *operation,
                       char       **err);

/* mcp_describe_procedure lists a PDB procedure's arguments in the same table
 * format as mcp_describe_op. Returns a string the caller must free. */
char *mcp_describe_procedure (GimpProcedure *procedure);

/* Result inspection ------------------------------------------------------- */

int         mcp_values_length (GimpValueArray *values, int i_unused);
const char *mcp_value_type    (GimpValueArray *values, int i);
gint64      mcp_value_int     (GimpValueArray *values, int i);
double      mcp_value_double  (GimpValueArray *values, int i);
int         mcp_value_bool    (GimpValueArray *values, int i);
/* mcp_value_string returns a newly allocated string the caller must free. */
char       *mcp_value_string  (GimpValueArray *values, int i);
/* mcp_value_object_id returns the id of an object value, or -1 when the value
 * is not an object or not one of the GIMP types that carry an id. */
gint32      mcp_value_object_id (GimpValueArray *values, int i);
/* mcp_value_enum_nick returns an enum value's nick, GIMP's own name for it,
 * or NULL when the value is not an enum. Caller frees. */
char       *mcp_value_enum_nick (GimpValueArray *values, int i);
/* mcp_value_color_css returns the colour as "rgba(r,g,b,a)" with components in
 * 0-255 / 0-1. Caller frees. */
char       *mcp_value_color_css (GimpValueArray *values, int i);
/* mcp_color_css_normalize parses a CSS colour and returns it in the form
 * mcp_value_color_css uses, or NULL if it does not parse. Caller frees. */
char       *mcp_color_css_normalize (const char *css);
/* mcp_value_file_path returns a GFile result's path, or NULL. Caller frees. */
char       *mcp_value_file_path (GimpValueArray *values, int i);

/* mcp_value_object_array_len and _id read a core object array result. */
int    mcp_value_object_array_len (GimpValueArray *values, int i);
gint32 mcp_value_object_array_id  (GimpValueArray *values, int i, int j);

/* mcp_value_double_array_len and _at read a float array result. */
int    mcp_value_double_array_len (GimpValueArray *values, int i);
double mcp_value_double_array_at  (GimpValueArray *values, int i, int j);

void mcp_values_free (GimpValueArray *values);

#endif /* MCP_BRIDGE_H */
