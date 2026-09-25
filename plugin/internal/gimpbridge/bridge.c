#include "bridge.h"
#include "_cgo_export.h"

/* set_prop_text is shared by the PDB and GEGL setters below. */
static char *set_prop_text (GObject *config, const char *name, const char *v);

#include <libgimp/gimpui.h>
#include <stdarg.h>
#include <stdlib.h>

/* Plug-in registration ---------------------------------------------------- */

#define MCP_TYPE_PLUG_IN (mcp_plug_in_get_type ())
#define MCP_PROC_START   "plug-in-mcp-server"
#define MCP_PROC_STOP    "plug-in-mcp-server-stop"

typedef struct _McpPlugIn      { GimpPlugIn      parent_instance; } McpPlugIn;
typedef struct _McpPlugInClass { GimpPlugInClass parent_class; }    McpPlugInClass;

G_DEFINE_TYPE (McpPlugIn, mcp_plug_in, GIMP_TYPE_PLUG_IN)

static GMainLoop *mcp_loop = NULL;

/* The bind address defaults to the environment so an unattended GIMP can be
 * configured before launch; the dialog below starts pre-filled with it. */
static const gchar *
mcp_default_host (void)
{
  const gchar *v = g_getenv ("GIMP_MCP_BIND_HOST");

  return (v != NULL && *v != '\0') ? v : MCP_DEFAULT_HOST;
}

static gint
mcp_default_port (void)
{
  const gchar *v = g_getenv ("GIMP_MCP_BIND_PORT");
  gint64       n;

  if (v == NULL || *v == '\0')
    return MCP_DEFAULT_PORT;

  n = g_ascii_strtoll (v, NULL, 10);

  return (n > 0 && n <= 65535) ? (gint) n : MCP_DEFAULT_PORT;
}

/* Both procedures take the same address, so a server started on an unusual
 * port can still be stopped by passing the same one. */
static void
mcp_add_address_arguments (GimpProcedure *procedure)
{
  gimp_procedure_add_string_argument (procedure, "host",
                                      "_Host",
                                      "Address to bind. 127.0.0.1 keeps the "
                                      "server reachable only from this machine",
                                      mcp_default_host (),
                                      G_PARAM_READWRITE);

  gimp_procedure_add_int_argument (procedure, "port",
                                   "_Port",
                                   "TCP port to listen on",
                                   1, 65535, mcp_default_port (),
                                   G_PARAM_READWRITE);
}

/* mcp_report speaks through GIMP's own message handling, which the user has
 * pointed at a dialog, the status bar or the error console. */
static void mcp_report (const gchar *format, ...) G_GNUC_PRINTF (1, 2);

static void
mcp_report (const gchar *format, ...)
{
  va_list  args;
  gchar   *message;

  va_start (args, format);
  message = g_strdup_vprintf (format, args);
  va_end (args);

  gimp_message (message);
  g_free (message);
}

static GimpValueArray *
mcp_run_start (GimpProcedure       *procedure,
               GimpProcedureConfig *config,
               gpointer             run_data)
{
  GimpRunMode  run_mode = GIMP_RUN_NONINTERACTIVE;
  gchar       *host     = NULL;
  gint         port     = 0;
  char        *err      = NULL;

  g_object_get (config, "run-mode", &run_mode, NULL);

  /* Invoked from the menu, offer the address first. GIMP builds the dialog
   * from the argument specs above, so there is no widget code here. */
  if (run_mode == GIMP_RUN_INTERACTIVE)
    {
      GtkWidget *dialog;
      gboolean   confirmed;

      gimp_ui_init ("gimp-mcp-plugin");

      dialog = gimp_procedure_dialog_new (procedure, config, "Start MCP Server");
      gimp_procedure_dialog_set_ok_label (GIMP_PROCEDURE_DIALOG (dialog), "_Start");
      gimp_procedure_dialog_fill (GIMP_PROCEDURE_DIALOG (dialog), "host", "port", NULL);

      confirmed = gimp_procedure_dialog_run (GIMP_PROCEDURE_DIALOG (dialog));
      gtk_widget_destroy (dialog);

      if (! confirmed)
        return gimp_procedure_new_return_values (procedure, GIMP_PDB_CANCEL, NULL);
    }

  g_object_get (config, "host", &host, "port", &port, NULL);

  /* goStartServer binds the listener and returns immediately, serving on a
   * background goroutine; the GIMP API is only ever touched from this thread,
   * via mcp_idle_dispatch. A non-NULL return is the bind error. */
  err = goStartServer (host, port);

  if (err != NULL)
    {
      mcp_report ("Cannot start the MCP server: %s", err);
      free (err);
      g_free (host);

      /* Entering the loop anyway would leave a plug-in process running with
       * no listener: invisible in the UI, and with nothing to stop it. */
      return gimp_procedure_new_return_values (procedure,
                                               GIMP_PDB_EXECUTION_ERROR, NULL);
    }

  mcp_report ("MCP server listening on %s:%d.\n"
              "Stop it with Tools > MCP > Stop MCP Server.", host, port);
  g_free (host);

  mcp_loop = g_main_loop_new (NULL, FALSE);
  g_main_loop_run (mcp_loop);
  g_clear_pointer (&mcp_loop, g_main_loop_unref);

  return gimp_procedure_new_return_values (procedure, GIMP_PDB_SUCCESS, NULL);
}

/* GIMP runs every procedure in its own process, so this one cannot reach the
 * serving process's main loop in memory. It asks over the socket instead, and
 * that process quits itself. */
static GimpValueArray *
mcp_run_stop (GimpProcedure       *procedure,
              GimpProcedureConfig *config,
              gpointer             run_data)
{
  gchar *host = NULL;
  gint   port = 0;
  char  *err  = NULL;

  g_object_get (config, "host", &host, "port", &port, NULL);

  err = goStopServer (host, port);

  if (err != NULL)
    {
      mcp_report ("Cannot stop the MCP server: %s", err);
      free (err);
      g_free (host);

      return gimp_procedure_new_return_values (procedure,
                                               GIMP_PDB_EXECUTION_ERROR, NULL);
    }

  mcp_report ("MCP server on %s:%d stopped.", host, port);
  g_free (host);

  return gimp_procedure_new_return_values (procedure, GIMP_PDB_SUCCESS, NULL);
}

static GList *
mcp_query_procedures (GimpPlugIn *plug_in)
{
  GList *procedures = NULL;

  procedures = g_list_append (procedures, g_strdup (MCP_PROC_START));
  procedures = g_list_append (procedures, g_strdup (MCP_PROC_STOP));

  return procedures;
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

      gimp_procedure_set_menu_label (procedure, "Start MCP Server");
      gimp_procedure_set_documentation
        (procedure,
         "Start the MCP server",
         "Serves this GIMP instance over a local socket so MCP clients can drive it",
         name);
    }
  else if (! g_strcmp0 (name, MCP_PROC_STOP))
    {
      procedure = gimp_procedure_new (plug_in, name,
                                      GIMP_PDB_PROC_TYPE_PLUGIN,
                                      mcp_run_stop, NULL, NULL);

      gimp_procedure_set_menu_label (procedure, "Stop MCP Server");
      gimp_procedure_set_documentation
        (procedure,
         "Stop the MCP server",
         "Asks the running MCP server to close its socket and exit",
         name);
    }

  if (procedure == NULL)
    return NULL;

  /* The server is useful before anything is open, so keep the menu entries
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

  mcp_add_address_arguments (procedure);

  gimp_procedure_add_menu_path (procedure, "<Image>/Tools/MCP");
  gimp_procedure_set_attribution (procedure, "ryancurrah", "ryancurrah", "2026");

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

char *
mcp_set_string (GimpProcedureConfig *config, const char *name, const char *v)
{
  return set_prop_text (G_OBJECT (config), name, v);
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

  /* Resources (fonts, brushes, gradients, palettes), units and displays have
   * their own id spaces separate from items, so pick the lookup that matches
   * the argument. A unit's id is GimpUnitID: 0 is pixels. */
  if (g_type_is_a (type, GIMP_TYPE_RESOURCE))
    object = G_OBJECT (gimp_resource_get_by_id (id));
  else if (g_type_is_a (type, GIMP_TYPE_UNIT))
    object = G_OBJECT (gimp_unit_get_by_id (id));
  else if (g_type_is_a (type, GIMP_TYPE_DISPLAY))
    object = G_OBJECT (gimp_display_get_by_id (id));
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

/* prop_range reports a numeric property's declared bounds.
 *
 * Every numeric GParamSpec carries them, and they are the only statement of
 * what an operation will actually accept: nothing in GIMP's or GEGL's
 * published documentation records them. */
static gboolean
prop_range (GParamSpec *spec, double *lo, double *hi)
{
  GType type = G_PARAM_SPEC_VALUE_TYPE (spec);

  if (type == G_TYPE_DOUBLE)
    {
      *lo = G_PARAM_SPEC_DOUBLE (spec)->minimum;
      *hi = G_PARAM_SPEC_DOUBLE (spec)->maximum;
    }
  else if (type == G_TYPE_FLOAT)
    {
      *lo = G_PARAM_SPEC_FLOAT (spec)->minimum;
      *hi = G_PARAM_SPEC_FLOAT (spec)->maximum;
    }
  else if (type == G_TYPE_INT)
    {
      *lo = G_PARAM_SPEC_INT (spec)->minimum;
      *hi = G_PARAM_SPEC_INT (spec)->maximum;
    }
  else if (type == G_TYPE_UINT)
    {
      *lo = G_PARAM_SPEC_UINT (spec)->minimum;
      *hi = G_PARAM_SPEC_UINT (spec)->maximum;
    }
  else
    {
      return FALSE;
    }

  return TRUE;
}

/* set_gegl_prop assigns a numeric value to a GEGL operation property,
 * coercing it to the property's declared type.
 *
 * It refuses rather than absorbs a value the property will not take. GLib
 * discards an out-of-range assignment and leaves the operation's own default
 * in place, and an unknown property name is not an error to GLib at all, so
 * either one would otherwise render as a silently wrong picture. Returns a
 * message the caller must free, or NULL on success. */
static char *
set_gegl_prop (GObject *config, const char *name, double v)
{
  GParamSpec *spec = g_object_class_find_property (G_OBJECT_GET_CLASS (config), name);
  GValue      value = G_VALUE_INIT;
  GType       type;
  double      lo, hi;

  if (spec == NULL)
    return g_strdup_printf ("has no property \"%s\"", name);

  type = G_PARAM_SPEC_VALUE_TYPE (spec);

  if (prop_range (spec, &lo, &hi) && (v < lo || v > hi))
    return g_strdup_printf ("%s must be within %g..%g, got %g", name, lo, hi, v);

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

      return g_strdup_printf ("%s is a %s, which cannot be set as a number",
                              name, g_type_name (type));
    }

  g_object_set_property (config, name, &value);
  g_value_unset (&value);

  return NULL;
}

/* set_prop_text assigns a named choice to a GEGL operation property.
 *
 * GIMP 3 re-declares a GEGL enum property as a GimpChoice carrying a string,
 * so the name is tried as a string first. The enum branch is kept for the
 * properties GIMP leaves as plain enums, where the name is the value's nick. */
static char *
set_prop_text (GObject *config, const char *name, const char *v)
{
  GParamSpec *spec = g_object_class_find_property (G_OBJECT_GET_CLASS (config), name);
  GValue      value = G_VALUE_INIT;
  GType       type;

  if (spec == NULL)
    return g_strdup_printf ("has no property \"%s\"", name);

  type = G_PARAM_SPEC_VALUE_TYPE (spec);

  g_value_init (&value, type);

  if (GIMP_IS_PARAM_SPEC_CHOICE (spec))
    {
      GimpChoice *choice = gimp_param_spec_choice_get_choice (spec);

      if (!gimp_choice_is_valid (choice, v))
        {
          /* The list belongs to the choice and must not be freed. */
          GString *allowed = g_string_new (NULL);
          GList   *l;
          char    *msg;

          for (l = gimp_choice_list_nicks (choice); l != NULL; l = l->next)
            g_string_append_printf (allowed, "%s%s", allowed->len ? ", " : "",
                                    (const char *) l->data);

          msg = g_strdup_printf ("%s has no choice named \"%s\"; it takes %s",
                                 name, v, allowed->str);
          g_string_free (allowed, TRUE);
          g_value_unset (&value);

          return msg;
        }

      g_value_set_string (&value, v);
    }
  else if (type == G_TYPE_STRING)
    {
      g_value_set_string (&value, v);
    }
  else if (G_TYPE_IS_ENUM (type))
    {
      GEnumClass *klass = g_type_class_ref (type);
      GEnumValue *found = g_enum_get_value_by_nick (klass, v);

      if (found == NULL)
        found = g_enum_get_value_by_name (klass, v);

      if (found != NULL)
        {
          g_value_set_enum (&value, found->value);
          g_type_class_unref (klass);
        }
      else
        {
          GString *allowed = g_string_new (NULL);
          char    *msg;
          guint    i;

          for (i = 0; i < klass->n_values; i++)
            g_string_append_printf (allowed, "%s%s", i ? ", " : "",
                                    klass->values[i].value_nick);

          msg = g_strdup_printf ("%s has no choice named \"%s\"; it takes %s",
                                 name, v, allowed->str);
          g_string_free (allowed, TRUE);
          g_type_class_unref (klass);
          g_value_unset (&value);

          return msg;
        }
    }
  else
    {
      g_value_unset (&value);

      return g_strdup_printf ("%s is a %s, which cannot be set by name",
                              name, g_type_name (type));
    }

  g_object_set_property (config, name, &value);
  g_value_unset (&value);

  return NULL;
}

/* mcp_check_number refuses a value a procedure argument's declared range
 * excludes. GObject would otherwise discard it with a warning and run the
 * procedure with the argument's default, so the call would appear to succeed.
 */
char *
mcp_check_number (GimpProcedureConfig *config, const char *name, double v)
{
  GParamSpec *spec = g_object_class_find_property (G_OBJECT_GET_CLASS (config), name);
  double      lo, hi;

  if (spec != NULL && prop_range (spec, &lo, &hi) && (v < lo || v > hi))
    return g_strdup_printf ("must be within %g..%g, got %g", lo, hi, v);

  return NULL;
}

/* describe_spec appends one line of the property table mcp_describe_op and
 * mcp_describe_procedure both return:
 *
 *   name \t type \t min \t max \t default \t choices \t blurb
 */
static void
describe_spec (GString *out, GParamSpec *spec)
{
  GType       type = G_PARAM_SPEC_VALUE_TYPE (spec);
  double      lo, hi;
  const char *blurb;

  g_string_append_printf (out, "%s\t%s\t", g_param_spec_get_name (spec),
                          g_type_name (type));

  /* Bounds are written exactly: %g would round G_MAXINT to 2147480000 and
   * G_MAXDOUBLE past the largest finite double. */
  if (prop_range (spec, &lo, &hi))
    g_string_append_printf (out, "%.17g\t%.17g\t", lo, hi);
  else
    g_string_append (out, "\t\t");

  if (type == G_TYPE_DOUBLE)
    g_string_append_printf (out, "%g", G_PARAM_SPEC_DOUBLE (spec)->default_value);
  else if (type == G_TYPE_FLOAT)
    g_string_append_printf (out, "%g", G_PARAM_SPEC_FLOAT (spec)->default_value);
  else if (type == G_TYPE_INT)
    g_string_append_printf (out, "%d", G_PARAM_SPEC_INT (spec)->default_value);
  else if (type == G_TYPE_UINT)
    g_string_append_printf (out, "%u", G_PARAM_SPEC_UINT (spec)->default_value);
  else if (type == G_TYPE_BOOLEAN)
    g_string_append_printf (out, "%s",
                            G_PARAM_SPEC_BOOLEAN (spec)->default_value ? "true" : "false");
  else if (GIMP_IS_PARAM_SPEC_CHOICE (spec))
    {
      const char *nick = gimp_param_spec_choice_get_default (spec);

      if (nick != NULL)
        g_string_append (out, nick);
    }
  else if (type == G_TYPE_STRING && G_PARAM_SPEC_STRING (spec)->default_value != NULL)
    g_string_append (out, G_PARAM_SPEC_STRING (spec)->default_value);
  else if (G_TYPE_IS_ENUM (type))
    {
      GEnumClass *klass = g_type_class_ref (type);
      GEnumValue *value = g_enum_get_value (klass, G_PARAM_SPEC_ENUM (spec)->default_value);

      if (value != NULL)
        g_string_append (out, value->value_nick);

      g_type_class_unref (klass);
    }

  g_string_append_c (out, '\t');

  /* A GimpChoice knows its own permitted values; a plain enum knows its
   * nicks. Either way the caller gets the spelling it must send. */
  if (GIMP_IS_PARAM_SPEC_CHOICE (spec))
    {
      GimpChoice *choice = gimp_param_spec_choice_get_choice (spec);
      /* The list belongs to the choice. Freeing it here corrupted the
       * choice, and the next describe of the same operation crashed. */
      GList      *nicks  = gimp_choice_list_nicks (choice);
      GList      *l;

      for (l = nicks; l != NULL; l = l->next)
        g_string_append_printf (out, "%s%s", (l == nicks) ? "" : ",",
                                (const char *) l->data);
    }
  else if (G_TYPE_IS_ENUM (type))
    {
      GEnumClass *klass = g_type_class_ref (type);
      guint       v;

      for (v = 0; v < klass->n_values; v++)
        g_string_append_printf (out, "%s%s", (v == 0) ? "" : ",",
                                klass->values[v].value_nick);

      g_type_class_unref (klass);
    }

  blurb = g_param_spec_get_blurb (spec);
  g_string_append_printf (out, "\t%s\n", blurb ? blurb : "");
}

char *
mcp_describe_op (gint32       drawable_id,
                 const char  *operation,
                 char       **err)
{
  GimpDrawable       *drawable = GIMP_DRAWABLE (gimp_item_get_by_id (drawable_id));
  GimpDrawableFilter *filter;
  GObject            *config;
  GParamSpec        **specs;
  guint               n = 0, i;
  GString            *out;

  *err = NULL;

  if (drawable == NULL)
    {
      *err = g_strdup_printf ("no drawable with id %d", drawable_id);

      return NULL;
    }

  filter = gimp_drawable_filter_new (drawable, operation, operation);

  if (filter == NULL)
    {
      *err = g_strdup_printf ("GIMP has no GEGL operation named %s", operation);

      return NULL;
    }

  config = G_OBJECT (gimp_drawable_filter_get_config (filter));
  specs  = g_object_class_list_properties (G_OBJECT_GET_CLASS (config), &n);
  out    = g_string_new (NULL);

  for (i = 0; i < n; i++)
    describe_spec (out, specs[i]);

  g_free (specs);
  g_object_unref (filter);

  return g_string_free (out, FALSE);
}

char *
mcp_describe_procedure (GimpProcedure *procedure)
{
  int          n     = 0, i;
  GParamSpec **specs = gimp_procedure_get_arguments (procedure, &n);
  GString     *out   = g_string_new (NULL);

  for (i = 0; i < n; i++)
    describe_spec (out, specs[i]);

  return g_string_free (out, FALSE);
}

int
mcp_apply_gegl (gint32        drawable_id,
                const char   *operation,
                const char  **names,
                const double *values,
                int           n,
                const char  **text_names,
                const char  **text_values,
                int           text_n,
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
    {
      char *bad = set_gegl_prop (config, names[i], values[i]);

      if (bad != NULL)
        {
          *err = g_strdup_printf ("%s: %s", operation, bad);
          g_free (bad);
          g_object_unref (filter);

          return 0;
        }
    }

  for (i = 0; i < text_n; i++)
    {
      char *bad = set_prop_text (config, text_names[i], text_values[i]);

      if (bad != NULL)
        {
          *err = g_strdup_printf ("%s: %s", operation, bad);
          g_free (bad);
          g_object_unref (filter);

          return 0;
        }
    }

  gimp_drawable_filter_update (filter);

  /* Merging a filter into a text layer's pixels leaves it a text layer in
   * name only: GIMP then accepts every text-layer-set call and silently
   * ignores it. Kept as a non-destructive filter, as GIMP's own UI does, the
   * effect stays and follows later edits to the text. */
  if (GIMP_IS_TEXT_LAYER (drawable))
    gimp_drawable_append_filter (drawable, filter);
  else
    gimp_drawable_merge_filter (drawable, filter);

  g_object_unref (filter);

  return 1;
}

/* Pixel access ------------------------------------------------------------ */

/* pixel_format is the format pixels cross the bridge in: straight R'G'B'A
 * floats in the drawable's own colour space, so a colour-managed image is not
 * converted to sRGB and back on the way. */
static const Babl *
pixel_format (GimpDrawable *drawable)
{
  return babl_format_with_space ("R'G'B'A float",
                                 babl_format_get_space (gimp_drawable_get_format (drawable)));
}

static GimpDrawable *
pixel_drawable (gint32 drawable_id, char **err)
{
  GimpItem *item = gimp_item_get_by_id (drawable_id);

  if (item == NULL || ! GIMP_IS_DRAWABLE (item))
    {
      *err = g_strdup_printf ("no drawable with id %d", drawable_id);

      return NULL;
    }

  return GIMP_DRAWABLE (item);
}

int
mcp_read_pixels (gint32  drawable_id,
                 int     x,
                 int     y,
                 int     width,
                 int     height,
                 float  *out,
                 char  **err)
{
  GimpDrawable *drawable;
  GeglBuffer   *buffer;

  *err = NULL;

  drawable = pixel_drawable (drawable_id, err);
  if (drawable == NULL)
    return 0;

  buffer = gimp_drawable_get_buffer (drawable);
  gegl_buffer_get (buffer, GEGL_RECTANGLE (x, y, width, height), 1.0,
                   pixel_format (drawable), out,
                   GEGL_AUTO_ROWSTRIDE, GEGL_ABYSS_CLAMP);
  g_object_unref (buffer);

  return 1;
}

int
mcp_write_pixels (gint32       drawable_id,
                  int          x,
                  int          y,
                  int          width,
                  int          height,
                  const float *in,
                  char       **err)
{
  GimpDrawable *drawable;
  GeglBuffer   *buffer;
  GeglBuffer   *shadow;

  *err = NULL;

  drawable = pixel_drawable (drawable_id, err);
  if (drawable == NULL)
    return 0;

  buffer = gimp_drawable_get_buffer (drawable);
  shadow = gimp_drawable_get_shadow_buffer (drawable);

  /* Merging takes the whole shadow inside the selection bounds, and a fresh
   * shadow's contents are undefined, so it starts as a copy of the drawable
   * and only the rectangle is replaced. */
  gegl_buffer_copy (buffer, NULL, GEGL_ABYSS_NONE, shadow, NULL);
  gegl_buffer_set (shadow, GEGL_RECTANGLE (x, y, width, height), 0,
                   pixel_format (drawable), in, GEGL_AUTO_ROWSTRIDE);

  g_object_unref (buffer);
  /* Releasing the shadow flushes its tiles to GIMP, which the merge reads. */
  g_object_unref (shadow);

  if (! gimp_drawable_merge_shadow (drawable, TRUE))
    {
      *err = g_strdup ("GIMP could not merge the new pixels into the drawable");

      return 0;
    }

  gimp_drawable_update (drawable, x, y, width, height);

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

  if (GIMP_IS_UNIT (object))
    return gimp_unit_get_id (GIMP_UNIT (object));

  if (GIMP_IS_DISPLAY (object))
    return gimp_display_get_id (GIMP_DISPLAY (object));

  return -1;
}

gint32
mcp_value_object_id (GimpValueArray *values, int i)
{
  GValue *v = gimp_value_array_index (values, i);

  /* Enums and other non-objects reach here too; asking GObject for an object
   * out of those would log a critical and return NULL. */
  if (!G_VALUE_HOLDS_OBJECT (v))
    return -1;

  return mcp_object_id (g_value_get_object (v));
}

char *
mcp_value_enum_nick (GimpValueArray *values, int i)
{
  GValue     *v    = gimp_value_array_index (values, i);
  GType       type = G_VALUE_TYPE (v);
  GEnumClass *klass;
  GEnumValue *found;
  char       *nick = NULL;

  if (!G_TYPE_IS_ENUM (type))
    return NULL;

  klass = g_type_class_ref (type);
  found = g_enum_get_value (klass, g_value_get_enum (v));

  if (found != NULL)
    nick = g_strdup (found->value_nick);

  g_type_class_unref (klass);

  return nick;
}

static char *
color_css (GeglColor *color)
{
  gdouble rgba[4];

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
mcp_value_color_css (GimpValueArray *values, int i)
{
  return color_css (g_value_get_object (gimp_value_array_index (values, i)));
}

char *
mcp_color_css_normalize (const char *css)
{
  GeglColor *color = gimp_color_parse_css (css);
  char      *out;

  if (color == NULL)
    return NULL;

  out = color_css (color);
  g_object_unref (color);

  return out;
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
