//go:build linux

package main

/*
#cgo linux pkg-config: gtk+-3.0
#cgo !webkit2_41 pkg-config: webkit2gtk-4.0
#cgo webkit2_41 pkg-config: webkit2gtk-4.1

#include <stdlib.h>
#include <string.h>
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

// ---------------------------------------------------------------------------
// Native tab shell.
//
// Wails renders the whole dashboard inside ONE WebKitWebView. To get a real
// webview per tab (correct page inspector, no iframe restrictions, native zoom)
// we repackage that webview as a fixed-height "chrome" strip (header + tab bar)
// and put every tab into its own WebKitWebView inside a GtkStack below it.
//
// All tab state lives here in C (the stack owns the webviews); Go only posts
// request structs onto the GTK main thread with g_idle_add. Page title/uri
// changes are pushed to Go through the exported callbacks below (which are
// declared here as extern and defined in tabs_shell_export.go).
// ---------------------------------------------------------------------------

static WebKitWebView *shell_chrome = NULL;   // the main Wails webview (strip)
static GtkWidget *shell_chrome_box = NULL;   // container of the chrome webview
static GtkWidget *shell_stack = NULL;        // GtkStack hosting one webview per tab
static GtkWidget *shell_main_win = NULL;     // main toplevel window
static WebKitWebView *g_shell_settings_view = NULL;
static GtkWidget *g_shell_settings_win = NULL;
static int g_settings_transparent = 0;         // window transparency enabled?
static WebKitUserContentManager *g_settings_ucm = NULL; // dedicated ucm for the settings window
static WebKitWebView *g_shell_notes_view = NULL;    // per-tab notes editor window
static GtkWidget *g_shell_notes_win = NULL;
static int g_notes_transparent = 0;           // notes window transparency enabled?
static WebKitUserContentManager *g_notes_ucm = NULL; // dedicated ucm for the notes window
static int g_notes_tab_id = -1;               // tab whose notes are being edited
static int shell_chrome_h = 104;             // default strip height (px)

extern void exportShellTitle(int id, const char *title);
extern void exportShellUri(int id, const char *uri);
extern void exportShellNavState(int id, int canBack, int canFwd, int loading);
extern void exportSettingsMessage(const char *msg);
extern void exportNotesMessage(const char *msg);
extern void exportShellNotification(guint64 id, const char *title, const char *body);

// Per-tab terminal (VTE) support. The actual terminal widgets and logic live in
// terminal.go (same cgo amalgamation); tabs_shell.go only carries the request
// channel for it. These are defined in terminal.go, declared here so the C in
// this preamble (and the shell_req_cb switch) can call them.
extern void shell_terminal_toggle(int id, const char *host, int port, const char *user,
                                  const char *auth, const char *password,
                                  const char *key, const char *dir, int split);
extern void shell_terminal_open(int id, const char *host, int port, const char *user,
                                const char *auth, const char *password,
                                const char *key, const char *dir, int split);
extern void shell_terminal_close(int id);
extern void shell_terminal_restart(int id);
extern void shell_terminal_destroy(int id);
extern void shell_terminal_kill(int id);
extern void shell_terminal_reset_state(int id);
extern void shell_terminal_split(int id, int orient);

// Path of the SSH_ASKPASS helper (written by Go in the portable data dir). Set
// via shell_term_set_askpass(); read by terminal.go when spawning ssh with a
// stored password.
char *g_term_askpass = NULL;

// Forward declaration (defined in the notification-permissions section below).
static void shell_notif_permissions_install(void);

// Recursively find a widget by its GTK name (Wails names the container of the
// main webview "webview-box").
static GtkWidget *shell_find_named(GtkWidget *w, const char *name)
{
	if (!w || !GTK_IS_CONTAINER(w))
		return NULL;
	if (gtk_widget_get_name(w) && strcmp(gtk_widget_get_name(w), name) == 0)
		return w;
	GList *ch = gtk_container_get_children(GTK_CONTAINER(w));
	GtkWidget *hit = NULL;
	for (GList *l = ch; l && !hit; l = l->next)
		hit = shell_find_named(GTK_WIDGET(l->data), name);
	g_list_free(ch);
	return hit;
}

// Locate the main Wails webview once (its parent is the box named "webview-box").
static void shell_find_chrome(void)
{
	if (shell_chrome)
		return;
	GList *toplevels = gtk_window_list_toplevels();
	for (GList *l = toplevels; l; l = l->next) {
		GtkWidget *box = shell_find_named(GTK_WIDGET(l->data), "webview-box");
		if (!box)
			continue;
		GList *children = gtk_container_get_children(GTK_CONTAINER(box));
		if (children && children->data && WEBKIT_IS_WEB_VIEW(children->data))
			shell_chrome = WEBKIT_WEB_VIEW(children->data);
		g_list_free(children);
		if (shell_chrome)
			break;
	}
	g_list_free(toplevels);
}

// Repackage the widget tree:
//   GtkVBox mainWindow -> [chromeBox [webview], GtkStack #shell-stack]
static void shell_setup(void)
{
	if (shell_chrome && shell_stack)
		return;
	shell_find_chrome();
	if (!shell_chrome || !gtk_widget_get_parent(GTK_WIDGET(shell_chrome)))
		return;

	GtkWidget *wv = GTK_WIDGET(shell_chrome);
	GtkWidget *box = gtk_widget_get_parent(wv);   // "webview-box"
	GtkWidget *vbox = gtk_widget_get_parent(box); // window's vbox

	g_object_ref(wv);
	g_object_ref(box); // keep alive across the remove so we can destroy it cleanly
	gtk_container_remove(GTK_CONTAINER(box), wv);
	gtk_container_remove(GTK_CONTAINER(vbox), box);
	gtk_widget_destroy(box);
	g_object_unref(box);

	GtkWidget *chrome_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
	gtk_box_pack_start(GTK_BOX(chrome_box), wv, TRUE, TRUE, 0);
	shell_chrome_box = chrome_box;

	shell_stack = gtk_stack_new();
	gtk_widget_set_name(shell_stack, "shell-stack");
	gtk_stack_set_homogeneous(GTK_STACK(shell_stack), TRUE);
	gtk_stack_set_transition_type(GTK_STACK(shell_stack), GTK_STACK_TRANSITION_TYPE_NONE);

	gtk_box_pack_start(GTK_BOX(vbox), chrome_box, FALSE, FALSE, 0);
	gtk_box_pack_start(GTK_BOX(vbox), shell_stack, TRUE, TRUE, 0);

	gtk_widget_set_size_request(wv, -1, shell_chrome_h);
	gtk_widget_set_size_request(chrome_box, -1, shell_chrome_h);
	gtk_widget_show_all(vbox);
	shell_main_win = gtk_widget_get_toplevel(vbox);
	g_object_unref(wv);

	// Keep the area under the strip consistent with the dark chrome theme, and
	// style the per-tab terminal header bar (native GTK widgets created lazily
	// by terminal.go — CSS applies as soon as the widgets get their names).
	GtkCssProvider *css = gtk_css_provider_new();
	gtk_css_provider_load_from_data(css,
		"#shell-stack { background-color: #10141d; }"
		"#term-bar { background-color: #161b26; border-top: 1px solid #2a3040; }"
		"#term-bar label { color: #c9d1d9; font-family: monospace; font-size: 11px; padding: 3px 10px; }"
		"#term-bar button { background: transparent; border: none; color: #8b949e; padding: 0 10px; min-width: 24px; min-height: 0; }"
		"#term-bar button:hover { color: #e6edf3; background: #21262d; }"
		"#term-scroll { background-color: #0d1117; }"
		"#term-scroll scrollbar, #term-scroll scrollbar slider { background: transparent; border: none; }"
		"#term-scroll scrollbar.vertical { min-width: 10px; }"
		"#term-scroll scrollbar.vertical slider { background: #30363d; border-radius: 5px; min-width: 6px; margin: 1px; }"
		"#term-scroll scrollbar.vertical slider:hover { background: #484f58; }"
		"#term-scroll scrollbar.vertical slider:active { background: #58a6ff; }", -1, NULL);
	gtk_style_context_add_provider_for_screen(
		gdk_screen_get_default(), GTK_STYLE_PROVIDER(css),
		GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);

	shell_notif_permissions_install();
}

// --- tab notification permissions --------------------------------------------
//
// WebKitGTK 2.52 does NOT surface notification permission requests through the
// WebView "permission-request" signal: for a page whose origin is not in the
// web context's initial permission map, requestPermission() silently resolves
// to "denied" and show-notification never fires. To let tab pages display Web
// Notifications we pre-allow the origins of the configured tabs via
// WebKitWebContext::initialize-notification-permissions (the documented hook:
// it fires whenever a new web process is about to launch, which is when the
// initial permission map is read).

static char *g_notif_origins[64];
static int g_notif_origins_n = 0;

// Called from Go whenever the tab set changes (copies the strings; read by the
// permission handler on the GTK main thread).
void shell_notif_set_origins(char **origins, int n)
{
	if (n > 64)
		n = 64;
	for (int i = 0; i < g_notif_origins_n; i++)
		g_free(g_notif_origins[i]);
	for (int i = 0; i < n; i++)
		g_notif_origins[i] = g_strdup(origins[i]);
	g_notif_origins_n = n;
}

static void shell_notif_init_permissions(WebKitWebContext *ctx, gpointer d)
{
	GList *allowed = NULL;
	for (int i = 0; i < g_notif_origins_n; i++) {
		WebKitSecurityOrigin *o =
			webkit_security_origin_new_for_uri(g_notif_origins[i]);
		if (o)
			allowed = g_list_append(allowed, o);
	}
	// The origins live for the process lifetime; do NOT unref them here
	// (unref'ing the GList crashes: the object is owned by the context).
	webkit_web_context_initialize_notification_permissions(ctx, allowed, NULL);
}

static void shell_notif_permissions_install(void)
{
	static int installed = 0;
	if (installed)
		return;
	installed = 1;
	g_signal_connect(webkit_web_context_get_default(),
		"initialize-notification-permissions",
		G_CALLBACK(shell_notif_init_permissions), NULL);
}

// --- custom paned drag --------------------------------------------------------
//
// Dragging the terminal splitter resizes the webview (child1) on every motion
// event. With WebKit's accelerated compositing the repaint is async, so the
// webview briefly shows its background colour between frames (a flash). The
// default GtkPaned drag does that live resize. We replace it with a custom
// drag: while the pointer moves we only track the target position and draw a
// guide line; gtk_paned_set_position is applied on button release, so the
// webview is resized exactly once per drag.
//
// GTK3's GtkPaned drives its own drag with an internal GtkGestureDrag running
// in the TARGET propagation phase, which claims the pointer sequence BEFORE
// the widget's "button-press-event" signal is emitted (gtkwidget.c
// gtk_widget_event_internal). So a plain button-press handler can never beat
// it. We instead attach our OWN GtkGestureDrag in the CAPTURE phase: capture
// controllers run first, so we claim the sequence and the paned's gesture is
// then denied (its drag-begin set_state(CLAIMED) fails and does nothing).

static void shell_paned_drag_begin(GtkGestureDrag *gesture, gdouble x, gdouble y, gpointer data)
{
	GtkWidget *paned = data;
	// No terminal open (child2 missing): nothing to split, let the event pass.
	if (!gtk_paned_get_child2(GTK_PANED(paned))) {
		gtk_gesture_set_state(GTK_GESTURE(gesture), GTK_EVENT_SEQUENCE_DENIED);
		return;
	}
	int pos = gtk_paned_get_position(GTK_PANED(paned));
	int tol = 12; // separator/handle hit tolerance
	int hit;
	if (gtk_orientable_get_orientation(GTK_ORIENTABLE(paned)) == GTK_ORIENTATION_VERTICAL)
		hit = (y >= pos - tol && y <= pos + tol);
	else
		hit = (x >= pos - tol && x <= pos + tol);
	if (!hit) {
		gtk_gesture_set_state(GTK_GESTURE(gesture), GTK_EVENT_SEQUENCE_DENIED);
		return;
	}
	gtk_gesture_set_state(GTK_GESTURE(gesture), GTK_EVENT_SEQUENCE_CLAIMED);
	g_object_set_data(G_OBJECT(paned), "term-drag", GINT_TO_POINTER(1));
	g_object_set_data(G_OBJECT(paned), "term-drag-pos", GINT_TO_POINTER(pos));
	gtk_widget_queue_draw(paned);
}

static void shell_paned_drag_update(GtkGestureDrag *gesture, gdouble dx, gdouble dy, gpointer data)
{
	GtkWidget *paned = data;
	if (!g_object_get_data(G_OBJECT(paned), "term-drag"))
		return;
	GtkAllocation a;
	gtk_widget_get_allocation(paned, &a);
	gboolean vert = gtk_orientable_get_orientation(GTK_ORIENTABLE(paned)) == GTK_ORIENTATION_VERTICAL;
	double sx, sy;
	gtk_gesture_drag_get_start_point(gesture, &sx, &sy);
	int total = vert ? a.height : a.width;
	int v = (int)(vert ? (sy + dy) : (sx + dx));
	if (v < 1) v = 1;
	if (v > total - 1) v = total - 1;
	g_object_set_data(G_OBJECT(paned), "term-drag-pos", GINT_TO_POINTER(v));
	gtk_widget_queue_draw(paned);
}

static void shell_paned_drag_end(GtkGestureDrag *gesture, gdouble dx, gdouble dy, gpointer data)
{
	GtkWidget *paned = data;
	if (!g_object_get_data(G_OBJECT(paned), "term-drag"))
		return;
	int pos = GPOINTER_TO_INT(g_object_get_data(G_OBJECT(paned), "term-drag-pos"));
	g_object_set_data(G_OBJECT(paned), "term-drag", GINT_TO_POINTER(0));
	if (pos > 0)
		gtk_paned_set_position(GTK_PANED(paned), pos);
	gtk_widget_queue_draw(paned);
}

static gboolean shell_paned_draw(GtkWidget *paned, cairo_t *cr, gpointer data)
{
	if (!g_object_get_data(G_OBJECT(paned), "term-drag"))
		return FALSE;
	int pos = GPOINTER_TO_INT(g_object_get_data(G_OBJECT(paned), "term-drag-pos"));
	GtkAllocation a;
	gtk_widget_get_allocation(paned, &a);
	gboolean vert = gtk_orientable_get_orientation(GTK_ORIENTABLE(paned)) == GTK_ORIENTATION_VERTICAL;
	cairo_save(cr);
	cairo_set_source_rgb(cr, 0.24, 0.35, 0.75); // accent guide line
	if (vert) {
		cairo_move_to(cr, 0, pos);
		cairo_line_to(cr, a.width, pos);
	} else {
		cairo_move_to(cr, pos, 0);
		cairo_line_to(cr, pos, a.height);
	}
	cairo_set_line_width(cr, 2.0);
	cairo_stroke(cr);
	cairo_restore(cr);
	return FALSE;
}

static void shell_paned_install_drag(GtkWidget *paned)
{
	// CAPTURE-phase gesture: it sees the pointer press before the paned's own
	// TARGET-phase drag gesture, so it can claim the sequence first. See the
	// comment block at the top of this section.
	GtkGesture *g = gtk_gesture_drag_new(paned);
	gtk_event_controller_set_propagation_phase(GTK_EVENT_CONTROLLER(g),
	                                           GTK_PHASE_CAPTURE);
	g_signal_connect(g, "drag-begin", G_CALLBACK(shell_paned_drag_begin), paned);
	g_signal_connect(g, "drag-update", G_CALLBACK(shell_paned_drag_update), paned);
	g_signal_connect(g, "drag-end", G_CALLBACK(shell_paned_drag_end), paned);
	g_signal_connect_after(paned, "draw", G_CALLBACK(shell_paned_draw), NULL);
}

// Each tab lives inside a GtkBox stack child ("tab-<id>-box") that carries the
// tab-id g_object data; the box holds the tab's WebKitWebView plus (lazily, when
// the per-tab terminal is opened) the terminal header bar and the VTE terminal.
// Not static: also called from terminal.go's cgo preamble (separate unit).
GtkWidget *shell_find_tab_box(int id)
{
	if (!shell_stack)
		return NULL;
	GList *ch = gtk_container_get_children(GTK_CONTAINER(shell_stack));
	GtkWidget *hit = NULL;
	for (GList *l = ch; l && !hit; l = l->next) {
		if (g_object_get_data(G_OBJECT(l->data), "tab-id") == GINT_TO_POINTER(id))
			hit = GTK_WIDGET(l->data);
	}
	g_list_free(ch);
	return hit;
}

// The per-tab GtkPaned that wraps the webview (child1) and, when the terminal
// is open, the terminal box (child2). Created at tab creation in shell_show_tab.
// Not static: also called from terminal.go's cgo preamble (separate unit).
GtkWidget *shell_find_tab_paned(int id)
{
	GtkWidget *box = shell_find_tab_box(id);
	if (!box)
		return NULL;
	return GTK_WIDGET(g_object_get_data(G_OBJECT(box), "term-paned"));
}

// Descend into the tab box to find its WebKitWebView. The webview always
// lives inside the tab's GtkPaned (child1, see shell_show_tab); the paned is
// stored as "term-paned" g_object_data on the box.
WebKitWebView *shell_find_tab(int id)
{
	GtkWidget *box = shell_find_tab_box(id);
	if (!box)
		return NULL;
	GList *ch = gtk_container_get_children(GTK_CONTAINER(box));
	WebKitWebView *hit = NULL;
	for (GList *l = ch; l && !hit; l = l->next) {
		if (WEBKIT_IS_WEB_VIEW(l->data)) {
			hit = WEBKIT_WEB_VIEW(l->data);
		} else if (GTK_IS_PANED(l->data)) {
			GList *pc = gtk_container_get_children(GTK_CONTAINER(l->data));
			for (GList *m = pc; m && !hit; m = m->next) {
				if (WEBKIT_IS_WEB_VIEW(m->data))
					hit = WEBKIT_WEB_VIEW(m->data);
			}
			g_list_free(pc);
		}
	}
	g_list_free(ch);
	return hit;
}

// --- page title/uri notifications -> Go -----------------------------------
static void shell_title_cb(GObject *o, GParamSpec *p, gpointer d)
{
	int id = GPOINTER_TO_INT(d);
	const char *t = webkit_web_view_get_title(WEBKIT_WEB_VIEW(o));
	exportShellTitle(id, t ? t : "");
}

static void shell_uri_cb(GObject *o, GParamSpec *p, gpointer d)
{
	int id = GPOINTER_TO_INT(d);
	const char *u = webkit_web_view_get_uri(WEBKIT_WEB_VIEW(o));
	exportShellUri(id, u ? u : "");
	// Navigation state (back/forward availability, loading) can change on any
	// load progress tick; keep the chrome chrom buttons in sync.
	exportShellNavState(id,
		webkit_web_view_can_go_back(WEBKIT_WEB_VIEW(o)) ? 1 : 0,
		webkit_web_view_can_go_forward(WEBKIT_WEB_VIEW(o)) ? 1 : 0,
		webkit_web_view_is_loading(WEBKIT_WEB_VIEW(o)) ? 1 : 0);
}

// Fires continuously while a page loads (progress 0..1) so the chrome can show
// the reload->stop state switch and keep the back/forward availability updated.
static void shell_load_state_cb(WebKitWebView *wv, GParamSpec *p, gpointer d)
{
	int id = GPOINTER_TO_INT(d);
	exportShellNavState(id,
		webkit_web_view_can_go_back(wv) ? 1 : 0,
		webkit_web_view_can_go_forward(wv) ? 1 : 0,
		webkit_web_view_is_loading(wv) ? 1 : 0);
}

// --- tab notifications -> desktop -------------------------------------------
//
// Tab pages are real webviews, so they can use the Web Notifications API
// (new Notification(...)). WebKitGTK 2.52 is NOT built with libnotify on this
// stack, so the default show-notification handler would silently do nothing;
// we take over the pipeline here:
//
//  1. "permission-request": notification permission requests are allowed
//     (everything else stays denied).
//  2. "show-notification": the WebKitNotification (id/title/body) is forwarded
//     to Go, which displays it through D-Bus org.freedesktop.Notifications.
//  3. When the desktop notification is dismissed, Go calls
//     shell_notification_close(id) so the page's onclose fires.

static gboolean shell_notif_permission(WebKitWebView *wv, WebKitPermissionRequest *req, gpointer d)
{
	if (WEBKIT_IS_NOTIFICATION_PERMISSION_REQUEST(req))
		webkit_permission_request_allow(req);
	else
		webkit_permission_request_deny(req);
	return TRUE;
}

#define SHELL_NOTIF_MAX 32

typedef struct {
	guint64 id;                // WebKit notification id
	WebKitNotification *n;     // ref'ed so it survives until dismissed
} shell_notif_entry;

static shell_notif_entry g_shell_notifs[SHELL_NOTIF_MAX];
static int g_shell_notifs_n = 0;

static void shell_notif_add(guint64 id, WebKitNotification *n)
{
	if (g_shell_notifs_n >= SHELL_NOTIF_MAX) {
		// Bound the registry: evict the oldest by closing it (the page may no
		// longer be interested, and its onclose still fires).
		webkit_notification_close(g_shell_notifs[0].n);
		g_object_unref(g_shell_notifs[0].n);
		memmove(&g_shell_notifs[0], &g_shell_notifs[1],
			(size_t)(g_shell_notifs_n - 1) * sizeof(shell_notif_entry));
		g_shell_notifs_n--;
	}
	g_object_ref(n);
	g_shell_notifs[g_shell_notifs_n++] = (shell_notif_entry){id, n};
}

static void shell_notif_remove_idx(int i)
{
	g_object_unref(g_shell_notifs[i].n);
	memmove(&g_shell_notifs[i], &g_shell_notifs[i + 1],
		(size_t)(g_shell_notifs_n - i - 1) * sizeof(shell_notif_entry));
	g_shell_notifs_n--;
}

static void shell_notif_close_id(guint64 id)
{
	for (int i = 0; i < g_shell_notifs_n; i++) {
		if (g_shell_notifs[i].id == id) {
			webkit_notification_close(g_shell_notifs[i].n);
			shell_notif_remove_idx(i);
			return;
		}
	}
}

static gboolean shell_notif_shown(WebKitWebView *wv, WebKitNotification *n, gpointer d)
{
	guint64 id = webkit_notification_get_id(n);
	shell_notif_add(id, n);
	exportShellNotification(id, webkit_notification_get_title(n),
		webkit_notification_get_body(n));
	// We handled the notification (Go shows it); keep the default libnotify
	// handler (a no-op without libnotify) from also running.
	g_signal_stop_emission_by_name(G_OBJECT(wv), "show-notification");
	return TRUE;
}

static gboolean shell_notif_close_idle(gpointer d)
{
	guint64 *idp = (guint64 *)d;
	shell_notif_close_id(*idp);
	g_free(idp);
	return G_SOURCE_REMOVE;
}

// Called from Go (any goroutine) when the desktop notification was dismissed;
// runs the webkit_notification_close on the GTK main thread.
void shell_notif_close_impl(guint64 id)
{
	guint64 *idp = g_new(guint64, 1);
	*idp = id;
	g_idle_add(shell_notif_close_idle, idp);
}

// Create (lazily) the webview of a tab inside its box and show it in the stack.
static void shell_show_tab(int id, const char *url, double zoom)
{
	if (!shell_stack)
		return;
	GtkWidget *box = shell_find_tab_box(id);
	if (!box) {
		box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
		g_object_set_data(G_OBJECT(box), "tab-id", GINT_TO_POINTER(id));
		WebKitWebView *wv = WEBKIT_WEB_VIEW(webkit_web_view_new());
		// Avoid the whole-widget redraw GTK queues on every allocation change
		// (the webview manages its own repaint). Background is set to the chrome
		// color so any transient gap during a resize is dark, not default white.
		gtk_widget_set_redraw_on_allocate(GTK_WIDGET(wv), FALSE);
		GdkRGBA bg = { 0.063, 0.078, 0.114, 1.0 }; // #10141d, matches #shell-stack
		webkit_web_view_set_background_color(wv, &bg);
		g_object_set_data(G_OBJECT(wv), "tab-id", GINT_TO_POINTER(id));
		WebKitSettings *set = webkit_web_view_get_settings(wv);
		webkit_settings_set_enable_developer_extras(set, TRUE);
		g_signal_connect(G_OBJECT(wv), "notify::title", G_CALLBACK(shell_title_cb), GINT_TO_POINTER(id));
		g_signal_connect(G_OBJECT(wv), "notify::uri", G_CALLBACK(shell_uri_cb), GINT_TO_POINTER(id));
		g_signal_connect(G_OBJECT(wv), "notify::estimated-load-progress", G_CALLBACK(shell_load_state_cb), GINT_TO_POINTER(id));
		g_signal_connect(G_OBJECT(wv), "permission-request", G_CALLBACK(shell_notif_permission), NULL);
		g_signal_connect(G_OBJECT(wv), "show-notification", G_CALLBACK(shell_notif_shown), NULL);
		gtk_widget_show(GTK_WIDGET(wv));
		// The tab webview lives inside a GtkPaned (child1); the per-tab
		// terminal is packed as child2 when opened (terminal.go). The webview
		// is NEVER reparented: moving a realized/mapped WebKitWebView into a
		// new container corrupts GTK's css/draw state (blank page + CSS
		// assertion), so the paned is built here, at tab creation.
		GtkWidget *paned = gtk_paned_new(GTK_ORIENTATION_VERTICAL);
		gtk_widget_set_hexpand(paned, TRUE);
		gtk_widget_set_vexpand(paned, TRUE);
		g_object_set_data(G_OBJECT(box), "term-paned", paned);
		shell_paned_install_drag(paned);
		gtk_paned_pack1(GTK_PANED(paned), GTK_WIDGET(wv), TRUE, FALSE);
		gtk_box_pack_start(GTK_BOX(box), paned, TRUE, TRUE, 0);
		gtk_widget_show(paned);
		if (zoom > 0.01)
			webkit_web_view_set_zoom_level(wv, zoom);
		if (url && url[0])
			webkit_web_view_load_uri(wv, url);
		gtk_widget_show(GTK_WIDGET(box));
		char name[32];
		snprintf(name, sizeof name, "tab-%d", id);
		gtk_stack_add_named(GTK_STACK(shell_stack), box, name);
	}
	gtk_stack_set_visible_child(GTK_STACK(shell_stack), box);
	WebKitWebView *wv = shell_find_tab(id);
	if (wv && zoom > 0.01)
		webkit_web_view_set_zoom_level(wv, zoom);
	// A pre-created tab (eager precreate, no URL loaded yet) gets its URL on
	// first activation.
	if (wv && url && url[0] && !webkit_web_view_get_uri(wv))
		webkit_web_view_load_uri(wv, url);
	if (wv)
		gtk_widget_grab_focus(GTK_WIDGET(wv));
}

// ----- eager pre-creation of tab webviews -----
// Creating a tab's webview (webkit_web_view_new + paned + GtkStack wiring) is
// cheap but not free: doing it on FIRST activation adds latency to the first
// tab switch. We pre-create the webviews for every stored tab a moment after
// startup (once shell_setup has built the stack), WITHOUT loading any URL, so
// activating a tab only issues its load_uri. The URL is loaded lazily by
// shell_show_tab exactly as if the tab had been created on demand.
static int *g_precreate_ids = NULL;
static int g_precreate_n = 0;

// Store the tab ids to pre-create (dispatched through the request channel so
// this runs on the GTK main thread).
void shell_set_precreate_ids(const int *ids, int n)
{
	if (g_precreate_ids)
		g_free(g_precreate_ids);
	g_precreate_ids = NULL;
	g_precreate_n = n;
	if (n > 0 && ids) {
		g_precreate_ids = g_new(int, n);
		memcpy(g_precreate_ids, ids, sizeof(int) * (size_t)n);
	}
}

static gboolean shell_precreate_cb(gpointer data)
{
	if (!g_precreate_ids)
		return G_SOURCE_REMOVE;
	// The stack may not exist yet (shell_setup runs on the first idle
	// iteration); retry until it does.
	if (!shell_stack)
		return G_SOURCE_CONTINUE;
	for (int i = 0; i < g_precreate_n; i++) {
		int id = g_precreate_ids[i];
		if (id > 0 && !shell_find_tab_box(id))
			shell_show_tab(id, NULL, 0.0);
	}
	g_free(g_precreate_ids);
	g_precreate_ids = NULL;
	g_precreate_n = 0;
	return G_SOURCE_REMOVE;
}

// Arm the pre-create pass ~1.2s after startup. Runs on the GTK main thread
// (registered from the request handler / idle).
void shell_precreate_start(void)
{
	g_timeout_add(1200, shell_precreate_cb, NULL);
}

static void shell_destroy_tab(int id)
{
	if (!shell_stack)
		return;
	GtkWidget *box = shell_find_tab_box(id);
	if (!box)
		return;
	// Kill a running terminal session (ssh/shell) before tearing down the box;
	// destroying the VTE widget would also close the pty, but an explicit
	// SIGHUP makes the child exit promptly and predictably.
	shell_terminal_kill(id);
	shell_terminal_reset_state(id);
	gtk_container_remove(GTK_CONTAINER(shell_stack), box);
	gtk_widget_destroy(box);
}

static void shell_reorder(const int *ids, int n)
{
	if (!shell_stack)
		return;
	// GtkStack has no reorder_child in GTK3: rebuild the child set in order by
	// removing then re-adding each tab box (order = sequential add order).
	for (int i = 0; i < n; i++) {
		GtkWidget *box = shell_find_tab_box(ids[i]);
		if (!box)
			continue;
		int tid = GPOINTER_TO_INT(g_object_get_data(G_OBJECT(box), "tab-id"));
		gtk_container_remove(GTK_CONTAINER(shell_stack), box);
		char name[32];
		snprintf(name, sizeof name, "tab-%d", tid);
		gtk_stack_add_named(GTK_STACK(shell_stack), box, name);
	}
}

static void shell_zoom(int id, double level)
{
	WebKitWebView *wv = (id > 0) ? shell_find_tab(id) : shell_chrome;
	if (wv && level > 0.01)
		webkit_web_view_set_zoom_level(wv, level);
}

// Navigation helpers for the chrome strip controls. action: 0=back, 1=forward,
// 2=reload, 3=stop loading. id<=0 targets the chrome strip itself.
static void shell_nav(int id, int action)
{
	WebKitWebView *wv = (id > 0) ? shell_find_tab(id) : shell_chrome;
	if (!wv)
		return;
	switch (action) {
	case 0: webkit_web_view_go_back(wv); break;
	case 1: webkit_web_view_go_forward(wv); break;
	case 2: webkit_web_view_reload(wv); break;
	case 3: webkit_web_view_stop_loading(wv); break;
	}
}

// --- settings window -------------------------------------------------------
//
// The settings window uses its OWN webview + user content manager (NOT the
// Wails one): Wails delivers Go->JS responses ("ExecJS") only to the main
// webview, so a second webview sharing the chrome's IPC could never receive a
// reply (the app bundle would hang on CreateApp). Instead the settings page
// ("wails://wails/settings.html", a separate vite entry without the Wails
// runtime) talks to Go through the WebKit message handler "dashboardSettings"
// and Go answers with webkit_web_view_run_javascript on THIS webview.

static void on_settings_message(WebKitUserContentManager *ucm,
                                WebKitJavascriptResult *result, gpointer data)
{
	char *msg = NULL;
#if WEBKIT_MAJOR_VERSION >= 2 && WEBKIT_MINOR_VERSION >= 22
	JSCValue *value = webkit_javascript_result_get_js_value(result);
	msg = jsc_value_to_string(value);
#else
	JSGlobalContextRef context = webkit_javascript_result_get_global_context(result);
	JSValueRef value = webkit_javascript_result_get_value(result);
	JSStringRef js = JSValueToStringCopy(context, value, NULL);
	size_t size = JSStringGetMaximumUTF8CStringSize(js);
	msg = g_new(char, size);
	JSStringGetUTF8CString(js, msg, size);
	JSStringRelease(js);
#endif
	if (msg) {
		exportSettingsMessage(msg);
		g_free(msg);
	}
}

static char *settings_page_url(void)
{
	const char *base = shell_chrome ? webkit_web_view_get_uri(shell_chrome) : NULL;
	if (!base || !base[0])
		return g_strdup("wails://wails/settings.html");

	char *tmp = g_strdup(base);
	char *hash = strchr(tmp, '#');
	if (hash)
		*hash = '\0';
	char *query = strchr(tmp, '?');
	if (query)
		*query = '\0';
	size_t len = strlen(tmp);
	if (len > 0 && tmp[len - 1] != '/') {
		char *slash = strrchr(tmp, '/');
		if (slash)
			slash[1] = '\0';
	}
	char *res = g_strconcat(tmp, "settings.html",
		g_settings_transparent ? "?t=1" : "", NULL);
	g_free(tmp);
	return res;
}

// The settings window is frameless (no WM decorations); pressing the CARD
// HEADER starts a GTK move-drag. The decision of "is this a drag area?" is made
// by the PAGE (elementFromPoint) because the draggable area is the .modal-header
// and its close/control buttons must stay clickable. The async JS answer arrives
// a moment later, still on the GTK main thread, and only then does the drag begin.
static GdkEventButton g_settings_press;
static gboolean g_settings_press_valid = FALSE;

static void settings_drag_check_done(GObject *src, GAsyncResult *res, gpointer data)
{
	WebKitWebView *view = WEBKIT_WEB_VIEW(src);
	GError *gerr = NULL;
	JSCValue *v = webkit_web_view_evaluate_javascript_finish(view, res, &gerr);
	if (gerr) {
		g_error_free(gerr);
		g_settings_press_valid = FALSE;
		return;
	}
	if (!v || !jsc_value_is_boolean(v)) {
		if (v)
			g_object_unref(v);
		g_settings_press_valid = FALSE;
		return;
	}
	gboolean drag = jsc_value_to_boolean(v);
	g_object_unref(v);
	if (!drag || !g_settings_press_valid || !g_shell_settings_win) {
		g_settings_press_valid = FALSE;
		return;
	}
	gtk_window_begin_move_drag(GTK_WINDOW(g_shell_settings_win),
		g_settings_press.button, (gint)g_settings_press.x_root,
		(gint)g_settings_press.y_root, g_settings_press.time);
	g_settings_press_valid = FALSE;
}

static gboolean settings_drag_press(GtkWidget *w, GdkEvent *event, gpointer data)
{
	GdkEventButton *ev = (GdkEventButton *)event;
	if (ev->type != GDK_BUTTON_PRESS || ev->button != 1)
		return FALSE;
	WebKitWebView *view = g_shell_settings_view;
	if (!view)
		return FALSE;
	g_settings_press = *ev;
	g_settings_press_valid = TRUE;
	const char *tpl =
		"(function(x,y){var e=document.elementFromPoint(x,y);"
		"if(!e)return false;"
		"for(var n=e;n;n=n.parentElement){"
		"if(n.tagName==='BUTTON'||(n.classList&&n.classList.contains('btn')))return false;"
		"if(n.classList&&n.classList.contains('modal-header'))return true;}"
		"return false;})(%d,%d)";
	char *js = g_strdup_printf(tpl, (int)ev->x, (int)ev->y);
	webkit_web_view_evaluate_javascript(view, js, -1, NULL, NULL, NULL, settings_drag_check_done, NULL);
	g_free(js);
	return FALSE;
}

// ---------------------------------------------------------------------------
// Content-fit fallback for the settings window.
//
// The page itself measures the rendered '.modal' and asks to be resized via
// the bridge's "resize" method. That depends on the freshly built frontend
// bundle; if WebKit serves a stale cached settings.html the JS bridge may be
// old or missing. This C-side probe re-measures the modal from *C* after the
// page finishes loading (works with any bundle that renders '.modal') and
// resizes the window to match. Runs a handful of times to cover slow layout.
// ---------------------------------------------------------------------------

static int settings_fit_attempts_left = 0;

static void settings_fit_eval_done(GObject *src, GAsyncResult *res, gpointer data)
{
	WebKitWebView *view = WEBKIT_WEB_VIEW(src);
	GError *gerr = NULL;
	JSCValue *v = webkit_web_view_evaluate_javascript_finish(view, res, &gerr);
	if (gerr) {
		g_error_free(gerr);
		return;
	}
	if (!v)
		return;
	if (jsc_value_is_string(v)) {
		char *str = jsc_value_to_string(v);
		if (str && str[0]) {
			int w = 0, h = 0;
			if (sscanf(str, "%d,%d", &w, &h) == 2 && w >= 200 && h >= 100 && g_shell_settings_win) {
				settings_fit_attempts_left = 0; // stop the re-arm loop
				gtk_window_resize(GTK_WINDOW(g_shell_settings_win), w, h);
				g_free(str);
				g_object_unref(v);
				return;
			}
		}
		if (str)
			g_free(str);
	}
	g_object_unref(v);
}

static gboolean settings_fit_timeout(gpointer data)
{
	WebKitWebView *view = g_shell_settings_view;
	if (!view || !g_shell_settings_win) {
		settings_fit_attempts_left = 0;
		return FALSE;
	}
	const char *js =
		"(function(){var o=document.querySelector('.modal');"
		"if(!o)return '';"
		"var r=o.getBoundingClientRect();"
		"return Math.round(r.width+24)+','+Math.round(Math.min(r.height,780)+24);})()";
	webkit_web_view_evaluate_javascript(view, js, -1, NULL, NULL, NULL, settings_fit_eval_done, NULL);
	settings_fit_attempts_left--;
	return settings_fit_attempts_left > 0 ? TRUE : FALSE;
}

// Arm the C-side fit probe. Called from the settings bridge the first time the
// page talks to Go (i.e. right after the page finished loading), so the probe
// does NOT need a "load-finished" connection (which, on this GTK stack, can
// emit on a destroyed webview during teardown and spam a GLib CRITICAL).
static void settings_fit_start(void)
{
	if (!g_shell_settings_view)
		return;
	settings_fit_attempts_left = 6;
	g_timeout_add(500, settings_fit_timeout, NULL);
}

void shell_settings_fit_start(void)
{
	if (g_shell_settings_view)
		settings_fit_start();
}

// ---- Transparent settings window ------------------------------------------
// The settings window is a floating card: the GtkWindow, its webview and the
// page body are all fully transparent, so only the .modal card is visible and
// the desktop shows through around it. (The historical "black apron" came from
// leaving the window opaque — GTK paints its theme background over the whole
// rect even where the webview is transparent; putting the window on a per-pixel
// RGBA visual and clearing its background with cairo fixes that.)
//
// When the compositor / RGBA visual is unavailable the window STAYS opaque; the
// page then keeps its painted background (the fallback "card on a panel" look)
// instead of showing GTK's black void.

static gboolean settings_draw_clear(GtkWidget *w, cairo_t *cr, gpointer data)
{
	cairo_set_operator(cr, CAIRO_OPERATOR_CLEAR);
	cairo_paint(cr);
	return FALSE;
}

static void settings_enable_transparency(void)
{
	g_settings_transparent = 0;
	if (!g_shell_settings_win)
		return;
	GdkScreen *screen = gtk_widget_get_screen(GTK_WIDGET(g_shell_settings_win));
	if (!gdk_screen_is_composited(screen))
		return;
	GdkVisual *visual = gdk_screen_get_rgba_visual(screen);
	if (!visual)
		return;
	g_settings_transparent = 1;
	gtk_widget_set_visual(GTK_WIDGET(g_shell_settings_win), visual);
	gtk_widget_set_app_paintable(GTK_WIDGET(g_shell_settings_win), TRUE);
	g_signal_connect(g_shell_settings_win, "draw", G_CALLBACK(settings_draw_clear), NULL);
	// UTILITY type -> KWin renders a shadow around the window's rectangle; on an
	// ARGB (transparent) window that shadow shows as a hard black border around
	// the floating card. The UTILITY hint suppresses decorative shadows while
	// keeping the window focusable and (as a transient) above its parent.
	gtk_window_set_type_hint(GTK_WINDOW(g_shell_settings_win), GDK_WINDOW_TYPE_HINT_UTILITY);
}

static void shell_open_settings(void)
{
	if (g_shell_settings_win) {
		if (gtk_widget_get_visible(g_shell_settings_win))
			gtk_window_present(GTK_WINDOW(g_shell_settings_win));
		else
			gtk_widget_show_all(g_shell_settings_win);
		return;
	}

	g_shell_settings_win = gtk_window_new(GTK_WINDOW_TOPLEVEL);
	gtk_window_set_title(GTK_WINDOW(g_shell_settings_win), "Impostazioni");
	gtk_window_set_default_size(GTK_WINDOW(g_shell_settings_win), 900, 720);
	// Frameless like the main window; dragging is done on the CARD HEADER
	// (settings_drag_press asks the page via elementFromPoint).
	gtk_window_set_decorated(GTK_WINDOW(g_shell_settings_win), FALSE);
	settings_enable_transparency();
	// Build the page URL AFTER transparency is decided: settings_page_url()
	// appends ?t=1 only when the shim enabled window transparency, and reading
	// g_settings_transparent before enable_transparency() (still 0 on the first
	// open in a fresh process) would give the page the flat dict background.
	char *url = settings_page_url();
	// NOT modal: a modal grab blocks dragging/clicking the main window while the
	// settings card is open. Transient + raise-above keeps the card in front of
	// the chrome while both windows stay interactive.
	if (shell_main_win) {
		gtk_window_set_transient_for(GTK_WINDOW(g_shell_settings_win), GTK_WINDOW(shell_main_win));
		gtk_window_set_keep_above(GTK_WINDOW(g_shell_settings_win), TRUE);
	}
	g_signal_connect(g_shell_settings_win, "delete-event", G_CALLBACK(gtk_widget_hide_on_delete), NULL);

	// Dedicated user content manager for the settings IPC (see above).
	g_settings_ucm = webkit_user_content_manager_new();
	webkit_user_content_manager_register_script_message_handler(g_settings_ucm,
		"dashboardSettings");
	g_signal_connect(g_settings_ucm, "script-message-received::dashboardSettings",
		G_CALLBACK(on_settings_message), NULL);

	g_shell_settings_view = WEBKIT_WEB_VIEW(
		webkit_web_view_new_with_user_content_manager(g_settings_ucm));
	WebKitSettings *set = webkit_web_view_get_settings(g_shell_settings_view);
	webkit_settings_set_enable_developer_extras(set, TRUE);

	// Transparent page background when the C shim enabled window transparency:
	// only the .modal card is painted, the rest lets the desktop show through.
	// In the opaque fallback the webview background stays auto and the page's
	// own body paints the themed panel instead.
	if (g_settings_transparent) {
		GdkRGBA clear = { 0, 0, 0, 0 };
		webkit_web_view_set_background_color(g_shell_settings_view, &clear);
	}

	g_signal_connect(G_OBJECT(g_shell_settings_view), "button-press-event",
		G_CALLBACK(settings_drag_press), NULL);

	gtk_container_add(GTK_CONTAINER(g_shell_settings_win), GTK_WIDGET(g_shell_settings_view));
	gtk_widget_show_all(g_shell_settings_win);
	if (url && url[0])
		webkit_web_view_load_uri(g_shell_settings_view, url);
	g_free(url);
}

// Evaluate JS inside the settings webview (called from Go to deliver bridge
// replies — never from the GTK thread directly).
static gboolean settings_eval_idle(gpointer d)
{
	char *js = (char *)d;
	if (g_shell_settings_view)
		webkit_web_view_evaluate_javascript(g_shell_settings_view, js, -1, NULL, NULL, NULL, NULL, NULL);
	g_free(js);
	return FALSE;
}

void shell_settings_eval(const char *js)
{
	if (!js)
		return;
	g_idle_add(settings_eval_idle, g_strdup(js));
}

// Resize the settings window to fit its HTML content (measured by the page and
// reported via the bridge's "resize" method). Runs on the GTK main thread.
static gboolean settings_resize_cb(gpointer d)
{
	int *wh = (int *)d;
	if (g_shell_settings_win)
		gtk_window_resize(GTK_WINDOW(g_shell_settings_win), wh[0], wh[1]);
	g_free(wh);
	return FALSE;
}

void shell_settings_resize(int w, int h)
{
	int *wh = g_malloc(2 * sizeof(int));
	wh[0] = w;
	wh[1] = h;
	g_idle_add(settings_resize_cb, wh);
}

static void shell_close_settings(void)
{
	if (g_shell_settings_win) {
		gtk_widget_destroy(g_shell_settings_win);
		g_shell_settings_win = NULL;
		g_shell_settings_view = NULL;
		if (g_settings_ucm) {
			g_object_unref(g_settings_ucm);
			g_settings_ucm = NULL;
		}
	}
}

// --- notes window -----------------------------------------------------------
//
// The per-tab notes editor is a DEDICATED floating window (same architecture as
// the settings window: its own webview + user content manager + custom bridge
// "dashboardNotes", no Wails IPC). It is deliberately NOT modal — transient_for
// + keep_above leaves the main window fully usable and clickable — and unlike
// the former DOM notes card it does NOT touch the chrome strip, so the tab pages
// below never get resized/shifted while the notes editor is open.
//
// The page is "wails://wails/notes.html?tab=<id>", a separate vite entry
// (frontend/src/notes/main.js) that talks to Go through "dashboardNotes" and
// receives replies via window.__dashReply (notes_bridge.go -> shell_notes_eval).

static void on_notes_message(WebKitUserContentManager *ucm,
                             WebKitJavascriptResult *result, gpointer data)
{
	char *msg = NULL;
#if WEBKIT_MAJOR_VERSION >= 2 && WEBKIT_MINOR_VERSION >= 22
	JSCValue *value = webkit_javascript_result_get_js_value(result);
	msg = jsc_value_to_string(value);
#else
	JSGlobalContextRef context = webkit_javascript_result_get_global_context(result);
	JSValueRef value = webkit_javascript_result_get_value(result);
	JSStringRef js = JSValueToStringCopy(context, value, NULL);
	size_t size = JSStringGetMaximumUTF8CStringSize(js);
	msg = g_new(char, size);
	JSStringGetUTF8CString(js, msg, size);
	JSStringRelease(js);
#endif
	if (msg) {
		exportNotesMessage(msg);
		g_free(msg);
	}
}

static char *notes_page_url(int tab_id)
{
	const char *base = shell_chrome ? webkit_web_view_get_uri(shell_chrome) : NULL;
	if (!base || !base[0])
		return g_strdup_printf("wails://wails/notes.html?tab=%d%s", tab_id,
			g_notes_transparent ? "&t=1" : "");

	char *tmp = g_strdup(base);
	char *hash = strchr(tmp, '#');
	if (hash)
		*hash = '\0';
	char *query = strchr(tmp, '?');
	if (query)
		*query = '\0';
	size_t len = strlen(tmp);
	if (len > 0 && tmp[len - 1] != '/') {
		char *slash = strrchr(tmp, '/');
		if (slash)
			slash[1] = '\0';
	}
	char *res = g_strdup_printf("%snotes.html?tab=%d%s", tmp, tab_id,
		g_notes_transparent ? "&t=1" : "");
	g_free(tmp);
	return res;
}

// Drag the notes window on the CARD HEADER only (.notes-header, sibling buttons
// excluded). Mirrors settings_drag_press with the notes window's globals.
static GdkEventButton g_notes_press;
static gboolean g_notes_press_valid = FALSE;

static void notes_drag_check_done(GObject *src, GAsyncResult *res, gpointer data)
{
	WebKitWebView *view = WEBKIT_WEB_VIEW(src);
	GError *gerr = NULL;
	JSCValue *v = webkit_web_view_evaluate_javascript_finish(view, res, &gerr);
	if (gerr) {
		g_error_free(gerr);
		g_notes_press_valid = FALSE;
		return;
	}
	if (!v || !jsc_value_is_boolean(v)) {
		if (v)
			g_object_unref(v);
		g_notes_press_valid = FALSE;
		return;
	}
	gboolean drag = jsc_value_to_boolean(v);
	g_object_unref(v);
	if (!drag || !g_notes_press_valid || !g_shell_notes_win) {
		g_notes_press_valid = FALSE;
		return;
	}
	gtk_window_begin_move_drag(GTK_WINDOW(g_shell_notes_win),
		g_notes_press.button, (gint)g_notes_press.x_root,
		(gint)g_notes_press.y_root, g_notes_press.time);
	g_notes_press_valid = FALSE;
}

static gboolean notes_drag_press(GtkWidget *w, GdkEvent *event, gpointer data)
{
	GdkEventButton *ev = (GdkEventButton *)event;
	if (ev->type != GDK_BUTTON_PRESS || ev->button != 1)
		return FALSE;
	WebKitWebView *view = g_shell_notes_view;
	if (!view)
		return FALSE;
	g_notes_press = *ev;
	g_notes_press_valid = TRUE;
	const char *tpl =
		"(function(x,y){var e=document.elementFromPoint(x,y);"
		"if(!e)return false;"
		"for(var n=e;n;n=n.parentElement){"
		"if(n.tagName==='BUTTON'||(n.classList&&n.classList.contains('btn')))return false;"
		"if(n.classList&&(n.classList.contains('notes-header')||n.classList.contains('modal-header')))return true;}"
		"return false;})(%d,%d)";
	char *js = g_strdup_printf(tpl, (int)ev->x, (int)ev->y);
	webkit_web_view_evaluate_javascript(view, js, -1, NULL, NULL, NULL, notes_drag_check_done, NULL);
	g_free(js);
	return FALSE;
}

// Content-fit fallback for the notes window (same idea as settings_fit_* but
// measuring ".notes-card"). The page itself reports the card size over the
// bridge ("resize"); this probe covers stale bundles, running a few times.
static int notes_fit_attempts_left = 0;

static void notes_fit_eval_done(GObject *src, GAsyncResult *res, gpointer data)
{
	WebKitWebView *view = WEBKIT_WEB_VIEW(src);
	GError *gerr = NULL;
	JSCValue *v = webkit_web_view_evaluate_javascript_finish(view, res, &gerr);
	if (gerr) {
		g_error_free(gerr);
		return;
	}
	if (!v)
		return;
	if (jsc_value_is_string(v)) {
		char *str = jsc_value_to_string(v);
		if (str && str[0]) {
			int w = 0, h = 0;
			if (sscanf(str, "%d,%d", &w, &h) == 2 && w >= 200 && h >= 100 && g_shell_notes_win) {
				notes_fit_attempts_left = 0;
				gtk_window_resize(GTK_WINDOW(g_shell_notes_win), w, h);
				g_free(str);
				g_object_unref(v);
				return;
			}
		}
		if (str)
			g_free(str);
	}
	g_object_unref(v);
}

static gboolean notes_fit_timeout(gpointer data)
{
	WebKitWebView *view = g_shell_notes_view;
	if (!view || !g_shell_notes_win) {
		notes_fit_attempts_left = 0;
		return FALSE;
	}
	const char *js =
		"(function(){var o=document.querySelector('.notes-card');"
		"if(!o)return '';"
		"var r=o.getBoundingClientRect();"
		"return Math.round(r.width+24)+','+Math.round(Math.min(r.height,780)+24);})()";
	webkit_web_view_evaluate_javascript(view, js, -1, NULL, NULL, NULL, notes_fit_eval_done, NULL);
	notes_fit_attempts_left--;
	return notes_fit_attempts_left > 0 ? TRUE : FALSE;
}

static void notes_fit_start(void)
{
	if (!g_shell_notes_view)
		return;
	notes_fit_attempts_left = 6;
	g_timeout_add(500, notes_fit_timeout, NULL);
}

void shell_notes_fit_start(void)
{
	if (g_shell_notes_view)
		notes_fit_start();
}

// Transparent window for the notes card (same recipe as the settings window).
static void notes_enable_transparency(void)
{
	g_notes_transparent = 0;
	if (!g_shell_notes_win)
		return;
	GdkScreen *screen = gtk_widget_get_screen(GTK_WIDGET(g_shell_notes_win));
	if (!gdk_screen_is_composited(screen))
		return;
	GdkVisual *visual = gdk_screen_get_rgba_visual(screen);
	if (!visual)
		return;
	g_notes_transparent = 1;
	gtk_widget_set_visual(GTK_WIDGET(g_shell_notes_win), visual);
	gtk_widget_set_app_paintable(GTK_WIDGET(g_shell_notes_win), TRUE);
	g_signal_connect(g_shell_notes_win, "draw", G_CALLBACK(settings_draw_clear), NULL);
	gtk_window_set_type_hint(GTK_WINDOW(g_shell_notes_win), GDK_WINDOW_TYPE_HINT_UTILITY);
}

static void shell_open_notes(int tab_id)
{
	if (g_shell_notes_win) {
		if (gtk_widget_get_visible(g_shell_notes_win))
			gtk_window_present(GTK_WINDOW(g_shell_notes_win));
		else
			gtk_widget_show_all(g_shell_notes_win);
		if (g_notes_tab_id != tab_id) {
			// Already showing another tab's notes: reload the page for this tab.
			char *url = notes_page_url(tab_id);
			g_notes_tab_id = tab_id;
			if (url && url[0])
				webkit_web_view_load_uri(g_shell_notes_view, url);
			g_free(url);
		}
		return;
	}

	g_shell_notes_win = gtk_window_new(GTK_WINDOW_TOPLEVEL);
	gtk_window_set_title(GTK_WINDOW(g_shell_notes_win), "Note");
	gtk_window_set_default_size(GTK_WINDOW(g_shell_notes_win), 520, 480);
	// Frameless like the main/settings windows; dragged on the card header.
	gtk_window_set_decorated(GTK_WINDOW(g_shell_notes_win), FALSE);
	notes_enable_transparency();
	// Build the page URL AFTER transparency is decided (same ordering fix as the
	// settings window): notes_page_url() appends &t=1 only when the shim enabled
	// window transparency, and reading g_notes_transparent before
	// notes_enable_transparency() (still 0 on first open) would give the page the
	// flat opaque background.
	char *url = notes_page_url(tab_id);
	// NOT modal: like the settings window, transient + raise-above keeps the
	// card in front while the main window stays fully usable and clickable.
	if (shell_main_win) {
		gtk_window_set_transient_for(GTK_WINDOW(g_shell_notes_win), GTK_WINDOW(shell_main_win));
		gtk_window_set_keep_above(GTK_WINDOW(g_shell_notes_win), TRUE);
	}
	g_signal_connect(g_shell_notes_win, "delete-event", G_CALLBACK(gtk_widget_hide_on_delete), NULL);

	// Dedicated user content manager for the notes IPC (see above).
	g_notes_ucm = webkit_user_content_manager_new();
	webkit_user_content_manager_register_script_message_handler(g_notes_ucm,
		"dashboardNotes");
	g_signal_connect(g_notes_ucm, "script-message-received::dashboardNotes",
		G_CALLBACK(on_notes_message), NULL);

	g_shell_notes_view = WEBKIT_WEB_VIEW(
		webkit_web_view_new_with_user_content_manager(g_notes_ucm));
	WebKitSettings *set = webkit_web_view_get_settings(g_shell_notes_view);
	webkit_settings_set_enable_developer_extras(set, TRUE);

	if (g_notes_transparent) {
		GdkRGBA clear = { 0, 0, 0, 0 };
		webkit_web_view_set_background_color(g_shell_notes_view, &clear);
	}

	g_signal_connect(G_OBJECT(g_shell_notes_view), "button-press-event",
		G_CALLBACK(notes_drag_press), NULL);

	gtk_container_add(GTK_CONTAINER(g_shell_notes_win), GTK_WIDGET(g_shell_notes_view));
	gtk_widget_show_all(g_shell_notes_win);
	g_notes_tab_id = tab_id;
	if (url && url[0])
		webkit_web_view_load_uri(g_shell_notes_view, url);
	g_free(url);
}

// Evaluate JS inside the notes webview (called from Go to deliver bridge
// replies — never from the GTK thread directly).
static gboolean notes_eval_idle(gpointer d)
{
	char *js = (char *)d;
	if (g_shell_notes_view)
		webkit_web_view_evaluate_javascript(g_shell_notes_view, js, -1, NULL, NULL, NULL, NULL, NULL);
	g_free(js);
	return FALSE;
}

void shell_notes_eval(const char *js)
{
	if (!js)
		return;
	g_idle_add(notes_eval_idle, g_strdup(js));
}

// Resize the notes window to fit its HTML content (bridge "resize" method).
static gboolean notes_resize_cb(gpointer d)
{
	int *wh = (int *)d;
	if (g_shell_notes_win)
		gtk_window_resize(GTK_WINDOW(g_shell_notes_win), wh[0], wh[1]);
	g_free(wh);
	return FALSE;
}

void shell_notes_resize(int w, int h)
{
	int *wh = g_malloc(2 * sizeof(int));
	wh[0] = w;
	wh[1] = h;
	g_idle_add(notes_resize_cb, wh);
}

static void shell_close_notes(void)
{
	if (g_shell_notes_win) {
		gtk_widget_destroy(g_shell_notes_win);
		g_shell_notes_win = NULL;
		g_shell_notes_view = NULL;
		g_notes_tab_id = -1;
		if (g_notes_ucm) {
			g_object_unref(g_notes_ucm);
			g_notes_ucm = NULL;
		}
		notes_fit_attempts_left = 0;
	}
}

// --- page inspector of a specific TAB --------------------------------------
static int g_ins_side = -1; // -1 none/float, 0 bottom, 1 right, 2 left
static gulong g_ins_configure_id = 0;

static GtkWindow *shell_ins_inspector_window(WebKitWebView *wv)
{
	WebKitWebInspector *ins = webkit_web_view_get_inspector(wv);
	if (!ins)
		return NULL;
	GtkWidget *iv = (GtkWidget *)webkit_web_inspector_get_web_view(ins);
	if (!iv)
		return NULL;
	return GTK_WINDOW(gtk_widget_get_toplevel(iv));
}

static void shell_ins_layout(void)
{
	if (!shell_main_win || (g_ins_side != 1 && g_ins_side != 2))
		return;

	GtkWindow *iw = NULL;
	// The detached inspector window is a toplevel; find the one differing from
	// the main window and the settings window.
	GList *toplevels = gtk_window_list_toplevels();
	for (GList *l = toplevels; l; l = l->next) {
		GtkWidget *w = GTK_WIDGET(l->data);
		if (w == shell_main_win || (g_shell_settings_win && w == g_shell_settings_win)
			|| (g_shell_notes_win && w == g_shell_notes_win))
			continue;
		if (GTK_IS_WINDOW(w) && gtk_widget_get_visible(w)) {
			iw = GTK_WINDOW(w);
			break;
		}
	}
	g_list_free(toplevels);
	if (!iw)
		return;

	gint mx, my, mw, mh, iww, ih;
	gtk_window_get_position(GTK_WINDOW(shell_main_win), &mx, &my);
	gtk_window_get_size(GTK_WINDOW(shell_main_win), &mw, &mh);
	gtk_window_get_size(iw, &iww, &ih);

	if (g_ins_side == 1) { // right
		gtk_window_move(iw, mx + mw, my);
		if (ih != mh)
			gtk_window_resize(iw, iww, mh);
	} else if (g_ins_side == 2) { // left
		gtk_window_move(iw, mx - iww, my);
		if (ih != mh)
			gtk_window_resize(iw, iww, mh);
	}
	gtk_window_present(iw);
}

static gboolean shell_ins_layout_once(gpointer data)
{
	shell_ins_layout();
	return G_SOURCE_REMOVE;
}

static gboolean shell_ins_main_configure(GtkWidget *widget, GdkEvent *event, gpointer data)
{
	shell_ins_layout();
	return FALSE;
}

static void shell_inspector(int mode, int tab_id)
{
	WebKitWebView *wv = (tab_id > 0) ? shell_find_tab(tab_id) : shell_chrome;
	if (!wv)
		return;
	WebKitWebInspector *ins = webkit_web_view_get_inspector(wv);
	if (!ins)
		return;

	if (mode == 4) { // close
		webkit_web_inspector_close(ins);
		g_ins_side = -1;
		return;
	}

	if (shell_main_win && g_ins_configure_id == 0)
		g_ins_configure_id = g_signal_connect(G_OBJECT(shell_main_win),
			"configure-event", G_CALLBACK(shell_ins_main_configure), NULL);

	webkit_web_inspector_show(ins);

	if (mode == 0) { // docked at the bottom of the tab
		g_ins_side = 0;
		if (!webkit_web_inspector_is_attached(ins))
			webkit_web_inspector_attach(ins);
	} else {
		g_ins_side = (mode == 2) ? 1 : (mode == 3 ? 2 : -1);
		if (webkit_web_inspector_is_attached(ins))
			webkit_web_inspector_detach(ins);
		if (g_ins_side == 1 || g_ins_side == 2)
			g_timeout_add(60, shell_ins_layout_once, NULL);
	}
}

// --- request channel -------------------------------------------------------
typedef struct {
	int op;        // 0 setup, 1 show, 2 destroy, 3 reorder, 4 zoom,
	               // 5 chrome height, 6 open settings, 7 close settings,
	               // 8 inspector, 9 open notes, 10 close notes,
	               // 11 terminal toggle, 12 terminal open/close,
	               // 13 terminal destroy, 14 terminal restart, 15 terminal split,
	               // 16 precreate tab webviews, 17 navigation (back/fwd/reload/stop)
	int id;
	double zoom;
	int height;
	int op_extra;
	char *url;
	int *ids;
	int n;
	// terminal fields (ops 11-15)
	int term_visible;
	char *term_host;
	int term_port;
	char *term_user;
	char *term_auth;
	char *term_password;
	char *term_key;
	char *term_dir;
	int term_split;
} shell_req;

static gboolean shell_req_cb(gpointer data)
{
	shell_req *req = (shell_req *)data;
	switch (req->op) {
	case 0: shell_setup(); break;
	case 1: shell_show_tab(req->id, req->url, req->zoom); break;
	case 2: shell_destroy_tab(req->id); break;
	case 3: shell_reorder(req->ids, req->n); break;
	case 4: shell_zoom(req->id, req->zoom); break;
	case 5:
		if (shell_chrome && req->height > 0) {
			shell_chrome_h = req->height;
			gtk_widget_set_size_request(GTK_WIDGET(shell_chrome), -1, shell_chrome_h);
			if (shell_chrome_box)
				gtk_widget_set_size_request(shell_chrome_box, -1, shell_chrome_h);
		}
		break;
	case 6: shell_open_settings(); break;
	case 7: shell_close_settings(); break;
	case 8: shell_inspector(req->op_extra, req->id); break;
	case 9: shell_open_notes(req->id); break;
	case 10: shell_close_notes(); break;
	case 11: shell_terminal_toggle(req->id, req->term_host, req->term_port, req->term_user,
		req->term_auth, req->term_password, req->term_key, req->term_dir, req->term_split); break;
	case 12:
		if (req->term_visible)
			shell_terminal_open(req->id, req->term_host, req->term_port, req->term_user,
				req->term_auth, req->term_password, req->term_key, req->term_dir, req->term_split);
		else
			shell_terminal_close(req->id);
		break;
	case 13: shell_terminal_destroy(req->id); break;
	case 14: shell_terminal_restart(req->id); break;
	case 15: shell_terminal_split(req->id, req->term_split); break;
	case 16:
		shell_set_precreate_ids(req->ids, req->n);
		shell_precreate_start();
		break;
	case 17: shell_nav(req->id, req->op_extra); break;
	}
	if (req->url) g_free(req->url);
	if (req->ids) g_free(req->ids);
	if (req->term_host) g_free(req->term_host);
	if (req->term_user) g_free(req->term_user);
	if (req->term_auth) g_free(req->term_auth);
	if (req->term_password) g_free(req->term_password);
	if (req->term_key) g_free(req->term_key);
	if (req->term_dir) g_free(req->term_dir);
	g_free(req);
	return G_SOURCE_REMOVE;
}

// Enqueue a request onto the GTK main thread (safe from any goroutine).
static void shell_request(int op, int id, const char *url, double zoom, int height, const int *ids, int n, int extra)
{
	shell_req *req = g_new0(shell_req, 1);
	req->op = op;
	req->id = id;
	req->url = url ? g_strdup(url) : NULL;
	req->zoom = zoom;
	req->height = height;
	req->n = n;
	req->op_extra = extra;
	if (n > 0 && ids) {
		req->ids = g_new(int, n);
		memcpy(req->ids, ids, sizeof(int) * (size_t)n);
	}
	g_idle_add(shell_req_cb, req);
}

// Enqueue a terminal request (ops 11-14). All strings are strdup'ed immediately
// (the Go caller may free its C strings right after this returns).
void shell_term_request(int op, int id, int visible, const char *host, int port,
                        const char *user, const char *auth, const char *password,
                        const char *key, const char *dir, int split)
{
	shell_req *req = g_new0(shell_req, 1);
	req->op = op;
	req->id = id;
	req->term_visible = visible;
	req->term_host = host ? g_strdup(host) : NULL;
	req->term_port = port;
	req->term_user = user ? g_strdup(user) : NULL;
	req->term_auth = auth ? g_strdup(auth) : NULL;
	req->term_password = password ? g_strdup(password) : NULL;
	req->term_key = key ? g_strdup(key) : NULL;
	req->term_dir = dir ? g_strdup(dir) : NULL;
	req->term_split = split;
	g_idle_add(shell_req_cb, req);
}

// Store the SSH_ASKPASS helper path (written by Go in the portable data dir).
void shell_term_set_askpass(const char *path)
{
	g_free(g_term_askpass);
	g_term_askpass = path ? g_strdup(path) : NULL;
}
*/
import "C"

import (
	"context"
	"unsafe"
)

// Marshal helpers ---------------------------------------------------------------

func shellPost(op int, id int, url string, zoom float64, height int) {
	var cURL *C.char
	if url != "" {
		cURL = C.CString(url)
	}
	C.shell_request(C.int(op), C.int(id), cURL, C.double(zoom), C.int(height), nil, 0, 0)
}

func shellPostIDs(op int, ids []int) {
	var arr *C.int
	if len(ids) > 0 {
		arr = (*C.int)(unsafe.Pointer(&ids[0]))
	}
	C.shell_request(C.int(op), 0, nil, 0, 0, arr, C.int(len(ids)), 0)
}

// shellPostNav enqueues a navigation op on a tab webview (action 0=back,
// 1=forward, 2=reload, 3=stop). id<=0 targets the chrome strip webview.
func shellPostNav(id, action int) {
	C.shell_request(C.int(17), C.int(id), nil, 0, 0, nil, 0, C.int(action))
}

func shellPostInspector(mode string, tabID int) {
	code := 0
	switch mode {
	case "bottom":
		code = 0
	case "float":
		code = 1
	case "right":
		code = 2
	case "left":
		code = 3
	case "close":
		code = 4
	default:
		code = 4
	}
	C.shell_request(C.int(8), C.int(tabID), nil, 0, 0, nil, 0, C.int(code))
}

func shellOpenNotes(tabID int) {
	C.shell_request(C.int(9), C.int(tabID), nil, 0, 0, nil, 0, 0)
}

func shellCloseNotes() {
	C.shell_request(C.int(10), 0, nil, 0, 0, nil, 0, 0)
}

// precreateTabs eagerly creates the webviews of all stored tabs (without
// loading their URLs) a moment after startup, so the first tab switch has no
// creation latency. The ids are dispatched onto the GTK main thread via the
// request channel (op 16); the C side waits for the stack to exist and then
// creates each missing tab box. ListTabs is used so first-run defaults are
// seeded before the ids are collected.
func (a *App) precreateTabs() {
	if a.tabAPI == nil {
		return
	}
	tabs, err := a.tabAPI.ListTabs(context.Background())
	if err != nil {
		logger.Printf("Precreate skipped: %v", err)
		return
	}
	ids := make([]int, 0, len(tabs))
	for _, t := range tabs {
		ids = append(ids, t.ID)
	}
	shellPostIDs(16, ids)
	logger.Printf("Precreate armed for %d tabs", len(ids))
}

// shellSetNotificationOrigins stores the origins that tab pages are allowed to
// display Web Notifications from (see the initialize-notification-permissions
// handler above). WebKitGTK 2.52 needs these pre-allowed or requestPermission()
// silently resolves to "denied" and show-notification never fires.
func shellSetNotificationOrigins(origins []string) {
	if len(origins) == 0 {
		C.shell_notif_set_origins(nil, 0)
		return
	}
	cOrigins := make([]*C.char, len(origins))
	for i, o := range origins {
		cOrigins[i] = C.CString(o)
	}
	defer func() {
		for _, p := range cOrigins {
			C.free(unsafe.Pointer(p))
		}
	}()
	C.shell_notif_set_origins(&cOrigins[0], C.int(len(cOrigins)))
}

// shellSetup schedules the widget repackage to run at the start of the GTK
// main loop (after Wails has packed its webview, before the first paint).
func shellSetup() {
	C.shell_request(C.int(0), 0, nil, 0, 0, nil, 0, 0)
}
