//go:build linux

package main

/*
#cgo linux pkg-config: gtk+-3.0
#cgo !webkit2_41 pkg-config: webkit2gtk-4.0
#cgo webkit2_41 pkg-config: webkit2gtk-4.1
#include <stdlib.h>
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

// Page inspector for a TAB, not the dashboard chrome.
//
// The dashboard renders inside a single WebKitWebView, so attaching WebKitGTK's
// inspector to it would debug the dashboard DOM instead of the service page the
// tab loads. To solve this we keep a dedicated *inspection webview* (a small
// top-level window) that loads the tab's real URL (same default web context,
// hence same cookies/localStorage/session) and enable devtools on it. All mode
// changes are marshalled onto the GTK main thread via g_idle_add.
//
// Modes: 0 = inspector docked at the bottom of the inspection window (native
// attach), 1 = free-floating inspector window, 2 = side panel on the right,
// 3 = side panel on the left, 4 = close the inspector.

static GtkWidget *g_ins_win = NULL;   // inspection (minibrowser) window
static GtkWidget *g_ins_box = NULL;   // its vertical box
static WebKitWebView *g_ins_view = NULL;
static int g_ins_side = -1;           // -1 none/float, 0 bottom, 1 right, 2 left
static gulong g_ins_configure_id = 0;

// Create (once) the inspection window + its webview with devtools enabled.
static WebKitWebView *ins_ensure_view(void) {
	if (g_ins_view)
		return g_ins_view;

	g_ins_win = gtk_window_new(GTK_WINDOW_TOPLEVEL);
	gtk_window_set_title(GTK_WINDOW(g_ins_win), "Ispettore — pagina del tab");
	gtk_window_set_default_size(GTK_WINDOW(g_ins_win), 1150, 800);
	// Closing the window with X hides it; the inspection webview (and its
	// loaded page) stays alive so sessions are preserved between inspections.
	g_signal_connect(g_ins_win, "delete-event", G_CALLBACK(gtk_widget_hide_on_delete), NULL);

	g_ins_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
	gtk_container_add(GTK_CONTAINER(g_ins_win), g_ins_box);

	g_ins_view = WEBKIT_WEB_VIEW(webkit_web_view_new());
	WebKitSettings *settings = webkit_web_view_get_settings(g_ins_view);
	webkit_settings_set_enable_developer_extras(settings, TRUE);
	gtk_box_pack_start(GTK_BOX(g_ins_box), GTK_WIDGET(g_ins_view), TRUE, TRUE, 0);

	gtk_widget_show_all(g_ins_win);
	return g_ins_view;
}

// The inspector's own toplevel window (non-NULL once the inspector is shown
// and detached; when attached it is the inspection window itself).
static GtkWindow *ins_inspector_window(WebKitWebView *wv) {
	WebKitWebInspector *ins = webkit_web_view_get_inspector(wv);
	if (!ins)
		return NULL;
	GtkWidget *iv = (GtkWidget *)webkit_web_inspector_get_web_view(ins);
	if (!iv)
		return NULL;
	return GTK_WINDOW(gtk_widget_get_toplevel(iv));
}

// Keeps a detiled inspector window glued to a side of the inspection window.
static void ins_layout(void) {
	if (!g_ins_win || !g_ins_view || g_ins_side != 1 && g_ins_side != 2)
		return;
	if (!gtk_widget_get_window(g_ins_win))
		return;
	GtkWindow *iw = ins_inspector_window(g_ins_view);
	if (!iw || !GTK_IS_WINDOW(iw))
		return;
	if (gtk_widget_get_window(GTK_WIDGET(iw)) == gtk_widget_get_window(g_ins_win))
		return; // still attached: nothing to do

	gint mx, my, mw, mh, iww, ih;
	gtk_window_get_position(GTK_WINDOW(g_ins_win), &mx, &my);
	gtk_window_get_size(GTK_WINDOW(g_ins_win), &mw, &mh);
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

static gboolean ins_layout_once(gpointer data) {
	ins_layout();
	return G_SOURCE_REMOVE;
}

// Re-glue the side panel while the inspection window moves/resizes.
static gboolean ins_main_configure(GtkWidget *widget, GdkEvent *event, gpointer data) {
	ins_layout();
	return FALSE;
}

static void ins_apply(int mode, const char *url) {
	WebKitWebView *wv = ins_ensure_view();
	if (!wv)
		return;

	if (url && url[0])
		webkit_web_view_load_uri(wv, url);

	// Re-show the inspection window in case it was hidden via the close button.
	if (!gtk_widget_get_visible(g_ins_win))
		gtk_widget_show_all(g_ins_win);
	else
		gtk_window_present(GTK_WINDOW(g_ins_win));

	WebKitWebInspector *ins = webkit_web_view_get_inspector(wv);
	if (!ins)
		return;

	if (mode == 4) { // close
		webkit_web_inspector_close(ins);
		g_ins_side = -1;
		return;
	}

	if (g_ins_win && g_ins_configure_id == 0)
		g_ins_configure_id = g_signal_connect(G_OBJECT(g_ins_win),
			"configure-event", G_CALLBACK(ins_main_configure), NULL);

	webkit_web_inspector_show(ins);

	if (mode == 0) { // docked at the bottom of the inspection window
		g_ins_side = 0;
		if (!webkit_web_inspector_is_attached(ins))
			webkit_web_inspector_attach(ins);
	} else {
		g_ins_side = (mode == 2) ? 1 : (mode == 3 ? 2 : -1);
		if (webkit_web_inspector_is_attached(ins))
			webkit_web_inspector_detach(ins);
		if (g_ins_side == 1 || g_ins_side == 2) {
			// The inspector window may be realised lazily; retry shortly after
			// the mode switch so the side panel lands on screen.
			g_timeout_add(60, ins_layout_once, NULL);
		}
	}
}

typedef struct { int mode; char *url; } ins_req;

static gboolean ins_request_cb(gpointer data) {
	ins_req *req = (ins_req *)data;
	ins_apply(req->mode, req->url);
	g_free(req->url);
	g_free(req);
	return G_SOURCE_REMOVE;
}

// Enqueue an inspector command (mode + tab URL) onto the GTK main thread.
static void ins_request(int mode, const char *url) {
	ins_req *req = g_new(ins_req, 1);
	req->mode = mode;
	req->url = url ? g_strdup(url) : NULL;
	g_idle_add(ins_request_cb, req);
}
*/
import "C"

import (
	"context"
	"fmt"
	"unsafe"
)

const (
	inspectorModeBottom = "bottom"
	inspectorModeFloat  = "float"
	inspectorModeRight  = "right"
	inspectorModeLeft   = "left"
)

// modeToCode maps the frontend mode strings to the C shim constants.
func modeToCode(mode string) (int, bool) {
	switch mode {
	case inspectorModeBottom:
		return 0, true
	case inspectorModeFloat:
		return 1, true
	case inspectorModeRight:
		return 2, true
	case inspectorModeLeft:
		return 3, true
	}
	return -1, false
}

// InspectorAvailable reports whether the inspector feature can be used. The
// inspection window enables WebKit developer extras itself, so it is always
// available in this build.
func (a *App) InspectorAvailable(ctx context.Context) bool {
	return true
}

func (a *App) InspectorAvailableNoContext() bool {
	return a.InspectorAvailable(context.Background())
}

// InspectorOpen opens a dedicated inspection window that loads the given tab
// URL and shows the WebKit inspector in the requested layout: "bottom" (docked
// under the page), "right"/"left" (side panel) or "float" (free window).
func (a *App) InspectorOpen(ctx context.Context, mode, url string) error {
	code, ok := modeToCode(mode)
	if !ok {
		return fmt.Errorf("modo ispettore non valido %q", mode)
	}
	curl := C.CString(url)
	defer C.free(unsafe.Pointer(curl))
	logger.Printf("InspectorOpen: mode=%s url=%s", mode, url)
	C.ins_request(C.int(code), curl)
	return nil
}

func (a *App) InspectorOpenNoContext(mode, url string) error {
	return a.InspectorOpen(context.Background(), mode, url)
}

// InspectorClose closes the page inspector of the inspection window.
func (a *App) InspectorClose(ctx context.Context) error {
	logger.Printf("InspectorClose")
	C.ins_request(C.int(4), (*C.char)(nil))
	return nil
}

func (a *App) InspectorCloseNoContext() error {
	return a.InspectorClose(context.Background())
}