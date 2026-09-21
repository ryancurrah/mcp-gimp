#include "bridge.h"
#include "_cgo_export.h"

/* Plug-in registration ---------------------------------------------------- */

#define MCP_TYPE_PLUG_IN (mcp_plug_in_get_type ())
#define MCP_PROC_START   "plug-in-mcp-server"

typedef struct _McpPlugIn      { GimpPlugIn      parent_instance; } McpPlugIn;
typedef struct _McpPlugInClass { GimpPlugInClass parent_class; }    McpPlugInClass;

G_DEFINE_TYPE (McpPlugIn, mcp_plug_in, GIMP_TYPE_PLUG_IN)

static GMainLoop *mcp_loop = NULL;

static GimpValueArray *
mcp_run_start (GimpProcedure       *procedure,
               GimpProcedureConfig *config,
               gpointer             run_data)
{
  /* goStartServer brings up the listener on a background goroutine and
   * returns immediately; the GIMP API is only ever touched from this thread,
   * via mcp_idle_dispatch. */
  goStartServer ();

  mcp_loop = g_main_loop_new (NULL, FALSE);
  g_main_loop_run (mcp_loop);
  g_clear_pointer (&mcp_loop, g_main_loop_unref);

  return gimp_procedure_new_return_values (procedure, GIMP_PDB_SUCCESS, NULL);
}

static GList *
mcp_query_procedures (GimpPlugIn *plug_in)
{
  return g_list_append (NULL, g_strdup (MCP_PROC_START));
}

static GimpProcedure *
mcp_create_procedure (GimpPlugIn  *plug_in,
                      const gchar *name)
{
  GimpProcedure *procedure = NULL;

  if (! g_strcmp0 (name, MCP_PROC_START))
    {
      procedure = gimp_procedure_new (plug_in, name,
                                      GIMP_PDB_PROC_TYPE_PLUGIN,
                                      mcp_run_start, NULL, NULL);

      /* The server is useful before anything is open, so keep the menu entry
       * enabled with no image and no selected drawable. */
      gimp_procedure_set_sensitivity_mask (procedure,
                                           GIMP_PROCEDURE_SENSITIVE_NO_IMAGE |
                                           GIMP_PROCEDURE_SENSITIVE_DRAWABLE |
                                           GIMP_PROCEDURE_SENSITIVE_DRAWABLES |
                                           GIMP_PROCEDURE_SENSITIVE_NO_DRAWABLES);

      /* An <Image> menu procedure has to accept the standard run mode. */
      gimp_procedure_add_enum_argument (procedure, "run-mode",
                                        "Run mode", "The run mode",
                                        GIMP_TYPE_RUN_MODE,
                                        GIMP_RUN_NONINTERACTIVE,
                                        G_PARAM_READWRITE);

      gimp_procedure_set_menu_label (procedure, "Start MCP Server");
      gimp_procedure_add_menu_path (procedure, "<Image>/Tools/MCP");
      gimp_procedure_set_documentation
        (procedure,
         "Start the MCP server",
         "Serves this GIMP instance over a local socket so MCP clients can drive it",
         name);
      gimp_procedure_set_attribution (procedure, "ryancurrah", "ryancurrah", "2026");
    }

  return procedure;
}

static void
mcp_plug_in_class_init (McpPlugInClass *klass)
{
  GimpPlugInClass *plug_in_class = GIMP_PLUG_IN_CLASS (klass);

  plug_in_class->query_procedures = mcp_query_procedures;
  plug_in_class->create_procedure = mcp_create_procedure;
}

static void
mcp_plug_in_init (McpPlugIn *self)
{
}

int
mcp_main (int argc, char **argv)
{
  return gimp_main (MCP_TYPE_PLUG_IN, argc, argv);
}

/* Main-loop dispatch ------------------------------------------------------ */

static gboolean
mcp_idle_trampoline (gpointer data)
{
  goMainThreadCall ((guintptr) data);

  return G_SOURCE_REMOVE;
}

void
mcp_idle_dispatch (guintptr handle)
{
  g_idle_add (mcp_idle_trampoline, (gpointer) handle);
}

void
mcp_quit_main_loop (void)
{
  if (mcp_loop != NULL)
    g_main_loop_quit (mcp_loop);
}

/* Procedure introspection ------------------------------------------------- */

GimpProcedure *
mcp_lookup_procedure (const char *name)
{
  return gimp_pdb_lookup_procedure (gimp_get_pdb (), name);
}

int
mcp_procedure_arg_count (GimpProcedure *procedure)
{
  int n = 0;

  (void) gimp_procedure_get_arguments (procedure, &n);

  return n;
}

const char *
mcp_procedure_arg_name (GimpProcedure *procedure, int i)
{
  int            n     = 0;
  GParamSpec   **specs = gimp_procedure_get_arguments (procedure, &n);

  if (i < 0 || i >= n)
    return NULL;

  return g_param_spec_get_name (specs[i]);
}

const char *
mcp_procedure_arg_type (GimpProcedure *procedure, int i)
{
  int          n     = 0;
  GParamSpec **specs = gimp_procedure_get_arguments (procedure, &n);

  if (i < 0 || i >= n)
    return NULL;

  return g_type_name (G_PARAM_SPEC_VALUE_TYPE (specs[i]));
}

const char *
mcp_procedure_arg_blurb (GimpProcedure *procedure, int i)
{
  int          n     = 0;
  GParamSpec **specs = gimp_procedure_get_arguments (procedure, &n);

  if (i < 0 || i >= n)
    return NULL;

  return g_param_spec_get_blurb (specs[i]);
}

int
mcp_procedure_arg_is_object (GimpProcedure *procedure, int i)
{
  int          n     = 0;
  GParamSpec **specs = gimp_procedure_get_arguments (procedure, &n);
  GType        type;

  if (i < 0 || i >= n)
    return 0;

  type = G_PARAM_SPEC_VALUE_TYPE (specs[i]);

  /* GFile is an object too, but it is bound from a path string. */
  if (type == G_TYPE_FILE)
    return 0;

  return G_TYPE_IS_OBJECT (type) ? 1 : 0;
}

/* Argument binding -------------------------------------------------------- */

GimpProcedureConfig *
mcp_create_config (GimpProcedure *procedure)
{
  return gimp_procedure_create_config (procedure);
}

/* set_prop assigns value to the named property and releases it. */
static void
set_prop (GimpProcedureConfig *config, const char *name, GValue *value)
{
  g_object_set_property (G_OBJECT (config), name, value);
  g_value_unset (value);
}

void
mcp_set_int (GimpProcedureConfig *config, const char *name, gint64 v)
{
  GParamSpec *spec = g_object_class_find_property (G_OBJECT_GET_CLASS (config), name);
  GValue      value = G_VALUE_INIT;
  GType       type  = spec ? G_PARAM_SPEC_VALUE_TYPE (spec) : G_TYPE_INT;

  /* GIMP uses int, uint and int64 arguments interchangeably from a caller's
   * point of view, so coerce to whatever the property actually wants. */
  g_value_init (&value, type);

  if (type == G_TYPE_INT)         g_value_set_int (&value, (gint) v);
  else if (type == G_TYPE_UINT)   g_value_set_uint (&value, (guint) v);
  else if (type == G_TYPE_INT64)  g_value_set_int64 (&value, v);
  else if (type == G_TYPE_UINT64) g_value_set_uint64 (&value, (guint64) v);
  else if (type == G_TYPE_DOUBLE) g_value_set_double (&value, (double) v);
  else if (type == G_TYPE_BOOLEAN) g_value_set_boolean (&value, v != 0);
  else if (G_TYPE_IS_ENUM (type)) g_value_set_enum (&value, (gint) v);
  else                            g_value_set_int (&value, (gint) v);

  set_prop (config, name, &value);
}

void
mcp_set_uint (GimpProcedureConfig *config, const char *name, guint64 v)
{
  mcp_set_int (config, name, (gint64) v);
}

void
mcp_set_double (GimpProcedureConfig *config, const char *name, double v)
{
  GParamSpec *spec = g_object_class_find_property (G_OBJECT_GET_CLASS (config), name);
  GValue      value = G_VALUE_INIT;
  GType       type  = spec ? G_PARAM_SPEC_VALUE_TYPE (spec) : G_TYPE_DOUBLE;

  g_value_init (&value, type);

  if (type == G_TYPE_DOUBLE)     g_value_set_double (&value, v);
  else if (type == G_TYPE_FLOAT) g_value_set_float (&value, (float) v);
  else if (type == G_TYPE_INT)   g_value_set_int (&value, (gint) v);
  else if (type == G_TYPE_UINT)  g_value_set_uint (&value, (guint) v);
  else                           g_value_set_double (&value, v);

  set_prop (config, name, &value);
}

void
mcp_set_bool (GimpProcedureConfig *config, const char *name, int v)
{
  GValue value = G_VALUE_INIT;

  g_value_init (&value, G_TYPE_BOOLEAN);
  g_value_set_boolean (&value, v != 0);
  set_prop (config, name, &value);
}

void
mcp_set_string (GimpProcedureConfig *config, const char *name, const char *v)
{
  GValue value = G_VALUE_INIT;

  g_value_init (&value, G_TYPE_STRING);
  g_value_set_string (&value, v);
  set_prop (config, name, &value);
}

void
mcp_set_enum (GimpProcedureConfig *config, const char *name, int v)
{
  GParamSpec *spec = g_object_class_find_property (G_OBJECT_GET_CLASS (config), name);
  GValue      value = G_VALUE_INIT;
  GType       type  = spec ? G_PARAM_SPEC_VALUE_TYPE (spec) : G_TYPE_INT;

  g_value_init (&value, type);

  if (G_TYPE_IS_ENUM (type)) g_value_set_enum (&value, v);
  else                       g_value_set_int (&value, v);

  set_prop (config, name, &value);
}

int
mcp_set_image (GimpProcedureConfig *config, const char *name, gint32 id)
{
  GimpImage *image = gimp_image_get_by_id (id);
  GValue     value = G_VALUE_INIT;

  if (image == NULL)
    return 0;

  g_value_init (&value, GIMP_TYPE_IMAGE);
  g_value_set_object (&value, image);
  set_prop (config, name, &value);

  return 1;
}

int
mcp_set_item (GimpProcedureConfig *config, const char *name, gint32 id)
{
  GParamSpec *spec  = g_object_class_find_property (G_OBJECT_GET_CLASS (config), name);
  GType       type  = spec ? G_PARAM_SPEC_VALUE_TYPE (spec) : GIMP_TYPE_ITEM;
  GObject    *object;
  GValue      value = G_VALUE_INIT;

  /* Resources (fonts, brushes, gradients, palettes) have their own id space
   * separate from items, so pick the lookup that matches the argument. */
  if (g_type_is_a (type, GIMP_TYPE_RESOURCE))
    object = G_OBJECT (gimp_resource_get_by_id (id));
  else
    object = G_OBJECT (gimp_item_get_by_id (id));

  if (object == NULL)
    return 0;

  g_value_init (&value, type);
  g_value_set_object (&value, object);
  set_prop (config, name, &value);

  return 1;
}

void
mcp_set_object_null (GimpProcedureConfig *config, const char *name)
{
  GParamSpec *spec = g_object_class_find_property (G_OBJECT_GET_CLASS (config), name);
  GValue      value = G_VALUE_INIT;

  g_value_init (&value, spec ? G_PARAM_SPEC_VALUE_TYPE (spec) : G_TYPE_OBJECT);
  g_value_set_object (&value, NULL);
  set_prop (config, name, &value);
}

void
mcp_set_file (GimpProcedureConfig *config, const char *name, const char *path)
{
  GFile  *file  = g_file_new_for_path (path);
  GValue  value = G_VALUE_INIT;

  g_value_init (&value, G_TYPE_FILE);
  g_value_set_object (&value, file);
  set_prop (config, name, &value);
  g_object_unref (file);
}

int
mcp_set_color (GimpProcedureConfig *config, const char *name, const char *css)
{
  GeglColor *color = gimp_color_parse_css (css);
  GValue     value = G_VALUE_INIT;

  if (color == NULL)
    return 0;

  g_value_init (&value, GEGL_TYPE_COLOR);
  g_value_set_object (&value, color);
  set_prop (config, name, &value);
  g_object_unref (color);

  return 1;
}

int
mcp_set_item_array (GimpProcedureConfig *config, const char *name,
                    const gint32 *ids, int n)
{
  GimpItem **items = g_new0 (GimpItem *, n > 0 ? n : 1);
  GValue     value = G_VALUE_INIT;
  int        i;

  for (i = 0; i < n; i++)
    {
      items[i] = gimp_item_get_by_id (ids[i]);

      if (items[i] == NULL)
        {
          g_free (items);

          return 0;
        }
    }

  g_value_init (&value, GIMP_TYPE_CORE_OBJECT_ARRAY);
  g_value_set_boxed (&value, items);
  set_prop (config, name, &value);
  g_free (items);

  return 1;
}

void
mcp_set_double_array (GimpProcedureConfig *config, const char *name,
                      const double *values, int n)
{
  GValue    value = G_VALUE_INIT;
  GimpArray array;

  array.data        = (guint8 *) values;
  array.length      = (gsize) n * sizeof (gdouble);
  array.static_data = TRUE;

  g_value_init (&value, GIMP_TYPE_DOUBLE_ARRAY);
  g_value_set_boxed (&value, &array);
  set_prop (config, name, &value);
}

/* Invocation -------------------------------------------------------------- */

GimpValueArray *
mcp_run_config (GimpProcedure       *procedure,
                GimpProcedureConfig *config,
                char               **err)
{
  GimpValueArray *values = gimp_procedure_run_config (procedure, config);
  GimpPDBStatusType status;

  *err = NULL;

  if (values == NULL)
    {
      *err = g_strdup ("procedure returned no values");

      return NULL;
    }

  status = g_value_get_enum (gimp_value_array_index (values, 0));

  if (status != GIMP_PDB_SUCCESS)
    {
      const GValue *message = NULL;

      if (gimp_value_array_length (values) > 1)
        message = gimp_value_array_index (values, 1);

      if (message != NULL && G_VALUE_HOLDS_STRING (message))
        *err = g_strdup (g_value_get_string (message));
      else
        *err = g_strdup_printf ("procedure failed with status %d", (int) status);

      gimp_value_array_unref (values);

      return NULL;
    }

  return values;
}

/* GEGL filters ------------------------------------------------------------ */

/* set_gegl_prop assigns a numeric value to a GEGL operation property,
 * coercing it to the property's declared type. */
static void
set_gegl_prop (GObject *config, const char *name, double v)
{
  GParamSpec *spec = g_object_class_find_property (G_OBJECT_GET_CLASS (config), name);
  GValue      value = G_VALUE_INIT;
  GType       type;

  if (spec == NULL)
    return;

  type = G_PARAM_SPEC_VALUE_TYPE (spec);

  g_value_init (&value, type);

  if (type == G_TYPE_DOUBLE)       g_value_set_double (&value, v);
  else if (type == G_TYPE_FLOAT)   g_value_set_float (&value, (float) v);
  else if (type == G_TYPE_INT)     g_value_set_int (&value, (gint) v);
  else if (type == G_TYPE_UINT)    g_value_set_uint (&value, (guint) v);
  else if (type == G_TYPE_BOOLEAN) g_value_set_boolean (&value, v != 0.0);
  else if (G_TYPE_IS_ENUM (type))  g_value_set_enum (&value, (gint) v);
  else
    {
      g_value_unset (&value);

      return;
    }

  g_object_set_property (config, name, &value);
  g_value_unset (&value);
}

int
mcp_apply_gegl (gint32        drawable_id,
                const char   *operation,
                const char  **names,
                const double *values,
                int           n,
                char        **err)
{
  GimpDrawable       *drawable = GIMP_DRAWABLE (gimp_item_get_by_id (drawable_id));
  GimpDrawableFilter *filter;
  GObject            *config;
  int                 i;

  *err = NULL;

  if (drawable == NULL)
    {
      *err = g_strdup_printf ("no drawable with id %d", drawable_id);

      return 0;
    }

  filter = gimp_drawable_filter_new (drawable, operation, operation);

  if (filter == NULL)
    {
      *err = g_strdup_printf ("GIMP has no GEGL operation named %s", operation);

      return 0;
    }

  config = G_OBJECT (gimp_drawable_filter_get_config (filter));

  for (i = 0; i < n; i++)
    set_gegl_prop (config, names[i], values[i]);

  gimp_drawable_filter_update (filter);
  gimp_drawable_merge_filter (drawable, filter);
  g_object_unref (filter);

  return 1;
}

/* Result inspection ------------------------------------------------------- */

int
mcp_values_length (GimpValueArray *values, int i_unused)
{
  (void) i_unused;

  return gimp_value_array_length (values);
}

const char *
mcp_value_type (GimpValueArray *values, int i)
{
  return G_VALUE_TYPE_NAME (gimp_value_array_index (values, i));
}

gint64
mcp_value_int (GimpValueArray *values, int i)
{
  GValue *v    = gimp_value_array_index (values, i);
  GType   type = G_VALUE_TYPE (v);

  if (type == G_TYPE_INT)    return g_value_get_int (v);
  if (type == G_TYPE_UINT)   return g_value_get_uint (v);
  if (type == G_TYPE_INT64)  return g_value_get_int64 (v);
  if (type == G_TYPE_UINT64) return (gint64) g_value_get_uint64 (v);
  if (G_TYPE_IS_ENUM (type)) return g_value_get_enum (v);
  if (type == G_TYPE_BOOLEAN) return g_value_get_boolean (v);

  return 0;
}

double
mcp_value_double (GimpValueArray *values, int i)
{
  GValue *v    = gimp_value_array_index (values, i);
  GType   type = G_VALUE_TYPE (v);

  if (type == G_TYPE_DOUBLE) return g_value_get_double (v);
  if (type == G_TYPE_FLOAT)  return g_value_get_float (v);

  return (double) mcp_value_int (values, i);
}

int
mcp_value_bool (GimpValueArray *values, int i)
{
  return g_value_get_boolean (gimp_value_array_index (values, i)) ? 1 : 0;
}

char *
mcp_value_string (GimpValueArray *values, int i)
{
  GValue *v = gimp_value_array_index (values, i);

  if (G_VALUE_HOLDS_STRING (v))
    return g_strdup (g_value_get_string (v));

  return g_strdup_value_contents (v);
}

/* mcp_object_id maps a GIMP object onto the integer id the protocol carries.
 * The scalar and array converters both go through here, so the set of types
 * they recognise cannot drift apart. */
static gint32
mcp_object_id (GObject *object)
{
  if (object == NULL)
    return -1;

  if (GIMP_IS_IMAGE (object))
    return gimp_image_get_id (GIMP_IMAGE (object));

  if (GIMP_IS_ITEM (object))
    return gimp_item_get_id (GIMP_ITEM (object));

  if (GIMP_IS_RESOURCE (object))
    return gimp_resource_get_id (GIMP_RESOURCE (object));

  return -1;
}

gint32
mcp_value_object_id (GimpValueArray *values, int i)
{
  return mcp_object_id (g_value_get_object (gimp_value_array_index (values, i)));
}

char *
mcp_value_color_css (GimpValueArray *values, int i)
{
  GValue    *v     = gimp_value_array_index (values, i);
  GeglColor *color = g_value_get_object (v);
  gdouble    rgba[4];

  if (color == NULL)
    return g_strdup ("");

  gegl_color_get_pixel (color, babl_format ("R'G'B'A double"), rgba);

  return g_strdup_printf ("rgba(%d,%d,%d,%.4f)",
                          (int) (rgba[0] * 255.0 + 0.5),
                          (int) (rgba[1] * 255.0 + 0.5),
                          (int) (rgba[2] * 255.0 + 0.5),
                          rgba[3]);
}

char *
mcp_value_file_path (GimpValueArray *values, int i)
{
  GValue *v    = gimp_value_array_index (values, i);
  GFile  *file = g_value_get_object (v);

  if (file == NULL)
    return NULL;

  return g_file_get_path (file);
}

int
mcp_value_object_array_len (GimpValueArray *values, int i)
{
  GObject **array = g_value_get_boxed (gimp_value_array_index (values, i));
  int       n     = 0;

  if (array == NULL)
    return 0;

  while (array[n] != NULL)
    n++;

  return n;
}

gint32
mcp_value_object_array_id (GimpValueArray *values, int i, int j)
{
  GObject **array = g_value_get_boxed (gimp_value_array_index (values, i));

  if (array == NULL)
    return -1;

  return mcp_object_id (array[j]);
}

int
mcp_value_double_array_len (GimpValueArray *values, int i)
{
  GimpArray *array = g_value_get_boxed (gimp_value_array_index (values, i));

  if (array == NULL)
    return 0;

  return (int) (array->length / sizeof (gdouble));
}

double
mcp_value_double_array_at (GimpValueArray *values, int i, int j)
{
  GimpArray *array = g_value_get_boxed (gimp_value_array_index (values, i));

  if (array == NULL)
    return 0.0;

  return ((const gdouble *) array->data)[j];
}

void
mcp_values_free (GimpValueArray *values)
{
  if (values != NULL)
    gimp_value_array_unref (values);
}
