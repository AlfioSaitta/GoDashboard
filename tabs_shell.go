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
static int shell_chrome_h = 104;             // default strip height (px)

extern void exportShellTitle(int id, const char *title);
extern void exportShellUri(int id, const char *uri);
extern void exportSettingsMessage(const char *msg);

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

	// Keep the area under the strip consistent with the dark chrome theme.
	GtkCssProvider *css = gtk_css_provider_new();
	gtk_css_provider_load_from_data(css,
		"#shell-stack { background-color: #10141d; }", -1, NULL);
	gtk_style_context_add_provider_for_screen(
		gdk_screen_get_default(), GTK_STYLE_PROVIDER(css),
		GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);
}

static WebKitWebView *shell_find_tab(int id)
{
	if (!shell_stack)
		return NULL;
	GList *ch = gtk_container_get_children(GTK_CONTAINER(shell_stack));
	WebKitWebView *hit = NULL;
	for (GList *l = ch; l && !hit; l = l->next) {
		if (g_object_get_data(G_OBJECT(l->data), "tab-id") == GINT_TO_POINTER(id))
			hit = WEBKIT_WEB_VIEW(l->data);
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
}

// Create (lazily) the webview of a tab and show it in the stack.
static void shell_show_tab(int id, const char *url, double zoom)
{
	if (!shell_stack)
		return;
	WebKitWebView *wv = shell_find_tab(id);
	if (!wv) {
		wv = WEBKIT_WEB_VIEW(webkit_web_view_new());
		g_object_set_data(G_OBJECT(wv), "tab-id", GINT_TO_POINTER(id));
		WebKitSettings *set = webkit_web_view_get_settings(wv);
		webkit_settings_set_enable_developer_extras(set, TRUE);
		g_signal_connect(G_OBJECT(wv), "notify::title", G_CALLBACK(shell_title_cb), GINT_TO_POINTER(id));
		g_signal_connect(G_OBJECT(wv), "notify::uri", G_CALLBACK(shell_uri_cb), GINT_TO_POINTER(id));
		gtk_widget_show(GTK_WIDGET(wv));
		char name[32];
		snprintf(name, sizeof name, "tab-%d", id);
		gtk_stack_add_named(GTK_STACK(shell_stack), GTK_WIDGET(wv), name);
		if (zoom > 0.01)
			webkit_web_view_set_zoom_level(wv, zoom);
		if (url && url[0])
			webkit_web_view_load_uri(wv, url);
	}
	gtk_stack_set_visible_child(GTK_STACK(shell_stack), GTK_WIDGET(wv));
	if (zoom > 0.01)
		webkit_web_view_set_zoom_level(wv, zoom);
	gtk_widget_grab_focus(GTK_WIDGET(wv));
}

static void shell_destroy_tab(int id)
{
	if (!shell_stack)
		return;
	WebKitWebView *wv = shell_find_tab(id);
	if (!wv)
		return;
	gtk_container_remove(GTK_CONTAINER(shell_stack), GTK_WIDGET(wv));
	gtk_widget_destroy(GTK_WIDGET(wv));
}

static void shell_reorder(const int *ids, int n)
{
	if (!shell_stack)
		return;
	// GtkStack has no reorder_child in GTK3: rebuild the child set in order by
	// removing then re-adding each webview (order = sequential add order).
	for (int i = 0; i < n; i++) {
		WebKitWebView *wv = shell_find_tab(ids[i]);
		if (!wv)
			continue;
		int tid = GPOINTER_TO_INT(g_object_get_data(G_OBJECT(wv), "tab-id"));
		gtk_container_remove(GTK_CONTAINER(shell_stack), GTK_WIDGET(wv));
		char name[32];
		snprintf(name, sizeof name, "tab-%d", tid);
		gtk_stack_add_named(GTK_STACK(shell_stack), GTK_WIDGET(wv), name);
	}
}

static void shell_zoom(int id, double level)
{
	WebKitWebView *wv = (id > 0) ? shell_find_tab(id) : shell_chrome;
	if (wv && level > 0.01)
		webkit_web_view_set_zoom_level(wv, level);
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

	char *url = settings_page_url();

	g_shell_settings_win = gtk_window_new(GTK_WINDOW_TOPLEVEL);
	gtk_window_set_title(GTK_WINDOW(g_shell_settings_win), "Impostazioni");
	gtk_window_set_default_size(GTK_WINDOW(g_shell_settings_win), 900, 720);
	// Frameless like the main window; dragging is done on the CARD HEADER
	// (settings_drag_press asks the page via elementFromPoint).
	gtk_window_set_decorated(GTK_WINDOW(g_shell_settings_win), FALSE);
	settings_enable_transparency();
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
		if (w == shell_main_win || (g_shell_settings_win && w == g_shell_settings_win))
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
	               // 8 inspector
	int id;
	double zoom;
	int height;
	int op_extra;
	char *url;
	int *ids;
	int n;
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
	}
	if (req->url) g_free(req->url);
	if (req->ids) g_free(req->ids);
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
*/
import "C"

import (
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

// shellSetup schedules the widget repackage to run at the start of the GTK
// main loop (after Wails has packed its webview, before the first paint).
func shellSetup() {
	C.shell_request(C.int(0), 0, nil, 0, 0, nil, 0, 0)
}