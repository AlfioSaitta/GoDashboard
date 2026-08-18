//go:build linux

package main

/*
#cgo linux pkg-config: gtk+-3.0
#cgo !webkit2_41 pkg-config: webkit2gtk-4.0
#cgo webkit2_41 pkg-config: webkit2gtk-4.1
#include <stdlib.h>
#include <webkit2/webkit2.h>
#include <libsoup/soup.h>
#include <stdio.h>
#include <string.h>

static WebKitCookieManager *cookie_manager(void) {
	return webkit_website_data_manager_get_cookie_manager(
		webkit_web_context_get_website_data_manager(webkit_web_context_get_default()));
}

static void setup_persistent_cookies(const char *path) {
	WebKitCookieManager *cm = cookie_manager();
	if (cm != NULL) {
		webkit_cookie_manager_set_persistent_storage(cm, path, WEBKIT_COOKIE_PERSISTENT_STORAGE_SQLITE);
	}
}

// ---------- JSON string escaping ----------
static void json_escape(FILE *f, const char *s) {
	fputc('"', f);
	for (const char *p = s; *p; p++) {
		unsigned char c = (unsigned char)*p;
		switch (c) {
		case '"': fputs("\\\"", f); break;
		case '\\': fputs("\\\\", f); break;
		case '\n': fputs("\\n", f); break;
		case '\r': fputs("\\r", f); break;
		case '\t': fputs("\\t", f); break;
		default:
			if (c < 0x20) fprintf(f, "\\u%04x", c); else fputc(c, f);
		}
	}
	fputc('"', f);
}

// ---------- snapshot (dump) ----------
static GMainLoop *snapshot_loop = NULL;
static GList *snapshot_merged = NULL;   // accumulated SoupCookie* copies
static SoupCookie **snapshot_cur_nodes = NULL; // alias array for dedup (freed)

// Run a nested default-context loop with a watchdog so a callback that never
// arrives (hung cookie manager / web process) cannot freeze the UI forever.
// crash-safe: the timeout source is cancelled when the loop exits normally,
// and quit-through-timeout marks the loop finished before the (unref'ed) loop
// is gone.
typedef struct {
	GMainLoop *loop;
	guint     *id;
} loop_watch;

static gboolean loop_quit_cb(gpointer data)
{
	loop_watch *w = data;
	*w->id = 0; // timeout fired: do NOT g_source_remove it afterwards
	g_main_loop_quit(w->loop);
	return G_SOURCE_REMOVE;
}

static void run_loop_watchdog(GMainLoop *loop, int ms)
{
	guint id = 0;
	loop_watch w = { loop, &id };
	id = g_timeout_add(ms > 0 ? ms : 1000, loop_quit_cb, &w);
	g_main_loop_run(loop);
	if (id != 0)
		g_source_remove(id);
}

// Dedup key compare on (domain, path, name).
static gboolean cookie_key_equal(SoupCookie *a, SoupCookie *b) {
	const char *da = soup_cookie_get_domain(a);
	const char *db = soup_cookie_get_domain(b);
	const char *pa = soup_cookie_get_path(a);
	const char *pb = soup_cookie_get_path(b);
	const char *na = soup_cookie_get_name(a);
	const char *nb = soup_cookie_get_name(b);
	return strcmp(da, db) == 0 && strcmp(pa, pb) == 0 && strcmp(na, nb) == 0;
}

static void snapshot_cb(GObject *o, GAsyncResult *res, gpointer data) {
	GList *list = webkit_cookie_manager_get_cookies_finish(WEBKIT_COOKIE_MANAGER(o), res, NULL);
	if (list != NULL) {
		int n = (int)g_list_length(list);
		int add = 0;
		for (GList *l = list; l; l = l->next) {
			SoupCookie *c = (SoupCookie *)l->data;
			gboolean dup = FALSE;
			for (GList *m = snapshot_merged; m; m = m->next) {
				if (cookie_key_equal((SoupCookie *)m->data, c)) { dup = TRUE; break; }
			}
			if (!dup) { snapshot_merged = g_list_append(snapshot_merged, soup_cookie_copy(c)); add++; }
		}
	}
	if (snapshot_loop != NULL) g_main_loop_quit(snapshot_loop);
}

// Write the accumulated cookie list to `path` as JSON.
static void snapshot_write(const char *path) {
	FILE *f = fopen(path, "w");
	if (!f) return;
	fputs("[", f);
	int i = 0;
	for (GList *l = snapshot_merged; l; l = l->next) {
		SoupCookie *c = (SoupCookie *)l->data;
		if (i++) fputs(",", f);
		fputs("{\"name\":", f); json_escape(f, soup_cookie_get_name(c));
		fputs(",\"value\":", f); json_escape(f, soup_cookie_get_value(c));
		fputs(",\"domain\":", f); json_escape(f, soup_cookie_get_domain(c));
		fputs(",\"path\":", f); json_escape(f, soup_cookie_get_path(c));
		fprintf(f, ",\"secure\":%s", soup_cookie_get_secure(c) ? "true" : "false");
		fprintf(f, ",\"httponly\":%s}", soup_cookie_get_http_only(c) ? "true" : "false");
	}
	fputs("]", f);
	fclose(f);
}

static void snapshot_clear(void) {
	if (snapshot_merged) {
		for (GList *l = snapshot_merged; l; l = l->next)
			soup_cookie_free((SoupCookie *)l->data);
		g_list_free(snapshot_merged);
		snapshot_merged = NULL;
	}
}

// Collect cookies for a single URI, merging into snapshot_merged. The result
// must be written via snapshot_write(). Returns the number of cookies the
// manager reported for this URI (-1 if the manager was unavailable).
static int snapshot_uri_collect(const char *uri) {
	WebKitCookieManager *cm = cookie_manager();
	if (!cm) return -1;
	GMainLoop *loop = g_main_loop_new(NULL, FALSE);
	snapshot_loop = loop;
	webkit_cookie_manager_get_cookies(cm, uri, NULL, snapshot_cb, NULL);
	// nested loop + watchdog: the cookie manager async callback may never
	// arrive if the web process is stuck; bail out after ~3s instead of
	// freezing the whole desktop until the timeout of the outer background
	// loop gets to run again.
	run_loop_watchdog(loop, 3000);
	g_main_loop_unref(loop);
	snapshot_loop = NULL;
	// return count collected so far so callers can log progress
	return (int)g_list_length(snapshot_merged);
}

// ---------- restore ----------
static volatile int add_done = 0;
static GMainLoop *restore_loop = NULL;

static void addcookie_cb(GObject *o, GAsyncResult *res, gpointer data) {
	webkit_cookie_manager_add_cookie_finish(WEBKIT_COOKIE_MANAGER(o), res, NULL);
	add_done = 1;
	if (restore_loop != NULL) g_main_loop_quit(restore_loop);
}

// Read cookies from the snapshot file and re-add them synchronously.
// Returns the number of cookies added.
static int restore_snapshot_sync(const char *path) {
	FILE *f = fopen(path, "r");
	if (!f) return 0;
	char buf[1048576];
	size_t r = fread(buf, 1, sizeof(buf) - 1, f);
	fclose(f);
	if (r == 0) return 0;
	buf[r] = 0;

	WebKitCookieManager *cm = cookie_manager();
	if (!cm) return 0;

	int count = 0;
	const char *p = buf;
	while ((p = strstr(p, "{\"name\":")) != NULL) {
		// name
		const char *n1 = strchr(p, '"'); n1 = strchr(n1 + 1, '"') + 1;
		if (*n1 != ':') break;
		const char *nq = strchr(n1 + 1, '"');
		const char *np_end = nq + 1;
		// find "value":
		const char *vpos = strstr(np_end, "\"value\":");
		if (!vpos) break;
		const char *vq = strchr(vpos + 8, '"');
		const char *vp_end = vq + 1;
		const char *dpos = strstr(vp_end, "\"domain\":");
		if (!dpos) break;
		const char *dq = strchr(dpos + 9, '"');
		const char *dp_end = dq + 1;
		const char *pathpos = strstr(dp_end, "\"path\":");
		if (!pathpos) break;
		const char *pq = strchr(pathpos + 7, '"');
		const char *pp_end = pq + 1;
		const char *spos = strstr(pp_end, "\"secure\":");
		if (!spos) break;
		int secure = strncmp(spos + 9, "true", 4) == 0;
		const char *hpos = strstr(spos, "\"httponly\":");
		int httponly = hpos && strncmp(hpos + 11, "true", 4) == 0;

		char name[512], value[32768], domain[512], path_[512];
		int k;

		k = 0; for (const char *q = nq + 1; *q && *q != '"' && k < (int)sizeof(name)-1; q++) { if (*q=='\\' && (q[1]=='"'||q[1]=='\\')) { name[k++]=q[1]; q++; } else name[k++]=*q; } name[k]=0;
		k = 0; for (const char *q = vq + 1; *q && *q != '"' && k < (int)sizeof(value)-1; q++) { if (*q=='\\' && (q[1]=='"'||q[1]=='\\')) { value[k++]=q[1]; q++; } else value[k++]=*q; } value[k]=0;
		k = 0; for (const char *q = dq + 1; *q && *q != '"' && k < (int)sizeof(domain)-1; q++) { if (*q=='\\' && (q[1]=='"'||q[1]=='\\')) { domain[k++]=q[1]; q++; } else domain[k++]=*q; } domain[k]=0;
		k = 0; for (const char *q = pq + 1; *q && *q != '"' && k < (int)sizeof(path_)-1; q++) { if (*q=='\\' && (q[1]=='"'||q[1]=='\\')) { path_[k++]=q[1]; q++; } else path_[k++]=*q; } path_[k]=0;

		SoupCookie *c = soup_cookie_new(name, value, domain, path_, -1);
		soup_cookie_set_secure(c, secure);
		soup_cookie_set_http_only(c, httponly);
		if (add_done) add_done = 0;
		GMainLoop *loop = g_main_loop_new(NULL, FALSE);
		restore_loop = loop;
		webkit_cookie_manager_add_cookie(cm, c, NULL, addcookie_cb, NULL);
		// watchdog: if the async add never completes (hung web process) we
		// still progress to the next cookie / finish instead of wedging the
		// main thread forever.
		run_loop_watchdog(loop, 3000);
		g_main_loop_unref(loop);
		restore_loop = NULL;
		count++;
		p = spos;
	}
	return count;
}

// ----- async restore on the GTK main thread -----
// App.OnDomReady fires on Wails' message goroutine, NOT on the GTK main thread.
// Running the cookie restore (creation of GMainLoops, nesting into the GTK
// default context) from that goroutine is unsafe — the nested loop must run on
// the GTK main thread. We dispatch the whole restore through an idle callback
// so it executes on the main loop, like every other GTK/WebKit call, and
// report the result through the //export below.
extern void exportCookiesRestored(int count, char *path);

static gboolean restore_snapshot_idle(gpointer data)
{
	char *path = data;
	int n = restore_snapshot_sync(path);
	exportCookiesRestored(n, path);
	g_free(path);
	return G_SOURCE_REMOVE;
}

// Schedule the restore (idle, GTK thread). `path` is copied.
static void restore_snapshot_async(const char *path)
{
	g_idle_add(restore_snapshot_idle, g_strdup(path));
}
// ---------- periodical snapshot scheduling ----------
static const char *g_snap_path = NULL;
static GStrv g_snap_uris = NULL;

static gboolean periodic_snapshot_cb(gpointer user_data) {
	snapshot_clear();
	int i = 0;
	while (g_snap_uris && g_snap_uris[i] != NULL) {
		snapshot_uri_collect(g_snap_uris[i]);
		i++;
	}
	if (snapshot_merged != NULL)
		snapshot_write(g_snap_path);
	else
		snapshot_clear();
	return G_SOURCE_CONTINUE;
}

// Schedule a GLib timeout (main thread) that snapshots every known URI.
static void schedule_periodic_snapshots(const char *path, char **uris, int n, int seconds) {
	if (g_snap_path) { g_free((gpointer)g_snap_path); }
	g_snap_path = g_strdup(path);
	if (g_snap_uris) g_strfreev(g_snap_uris);
	// build NULL-terminated array
	g_snap_uris = (GStrv)g_new0(char *, n + 1);
	for (int i = 0; i < n; i++) g_snap_uris[i] = g_strdup(uris[i]);
	g_snap_uris[n] = NULL;
	g_timeout_add_seconds(seconds > 0 ? seconds : 15, periodic_snapshot_cb, NULL);
}

// Snapshot all URIs immediately (used at shutdown / on demand).
static void snapshot_all_sync(const char *path, char **uris, int n) {
	snapshot_clear();
	for (int i = 0; i < n && uris[i] != NULL; i++) {
		snapshot_uri_collect(uris[i]);
	}
	if (snapshot_merged != NULL)
		snapshot_write(path);
	else
		snapshot_clear();
}
*/
import "C"

import (
	"net/url"
	"os"
	"strings"
	"unsafe"

	"dashboard/internal/paths"
)

// hostURI reduces any absolute URL (with or without scheme) to a root URI of
// the form "<scheme>://<host>/" used for cookie snapshotting.
func hostURI(raw string) string {
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/"
}

// webviewCookiesPath returns the JSON snapshot of WebKitGTK session cookies.
// Since WebKitGTK only persists cookies that carry an explicit expiry, the
// fresh session cookies (typical for logins) are snapshotted here and
// re-injected on startup.
func webviewCookiesPath() (string, error) {
	return paths.WebviewCookiesFile(), nil
}

// enablePersistentCookies configures WebKitGTK cookie storage. Must be called
// before wails.Run (before the first webview exists).
func enablePersistentCookies() error {
	dir := paths.WebviewDataDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := paths.CookieSQLite()
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	C.setup_persistent_cookies(cpath)
	logger.Printf("WebKit cookie storage: %s", path)
	return nil
}

// restoreWebviewCookies re-injects the snapshotted session cookies into the
// WebKit cookie manager. Called during App startup, before tabs load.
// The restore itself runs on the GTK main thread (idle callback), because the
// nested loops must live in the GTK default main context; this function only
// schedules it and returns immediately. The cookie count is reported via the
// exportCookiesRestored callback.
func restoreWebviewCookies() (int, error) {
	path, err := webviewCookiesPath()
	if err != nil {
		return 0, err
	}
	cpath := C.CString(path)
	C.restore_snapshot_async(cpath)
	C.free(unsafe.Pointer(cpath))
	logger.Printf("Webview cookie restore scheduled (%s)", path)
	return 0, nil
}

//export exportCookiesRestored
func exportCookiesRestored(count C.int, path *C.char) {
	p := C.GoString(path)
	if count > 0 {
		logger.Printf("Restored %d webview cookies from %s", int(count), p)
	}
}

// snapshotWebviewCookies dumps the cookies for a given URI into the JSON file.
func snapshotWebviewCookies(uri string) error {
	path, err := webviewCookiesPath()
	if err != nil {
		return err
	}
	curi := C.CString(uri)
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(curi))
	defer C.free(unsafe.Pointer(cpath))
	C.snapshot_clear()
	n := int(C.snapshot_uri_collect(curi))
	if n > 0 {
		C.snapshot_write(cpath)
	}
	logger.Printf("WebKit cookie snapshot %s: %d cookies", uri, n)
	return nil
}

// scheduleWebviewCookieSnapshots installs a GLib timer (GTK main thread) that
// snapshots the session cookies of the given URIs every interval seconds.
func scheduleWebviewCookieSnapshots(uris []string, interval int) error {
	if len(uris) == 0 {
		return nil
	}
	path, err := webviewCookiesPath()
	if err != nil {
		return err
	}
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	arr := C.malloc(C.size_t((len(uris) + 1)) * C.size_t(unsafe.Sizeof(uintptr(0))))
	if arr == nil {
		return os.ErrInvalid
	}
	defer C.free(arr)

	us := unsafe.Slice((*uintptr)(arr), len(uris)+1)
	for i, u := range uris {
		us[i] = uintptr(unsafe.Pointer(C.CString(u)))
	}
	us[len(uris)] = 0

	C.schedule_periodic_snapshots(cpath, (**C.char)(arr), C.int(len(uris)), C.int(interval))
	for i := 0; i < len(uris); i++ {
		C.free(unsafe.Pointer(us[i]))
	}
	logger.Printf("WebKit cookie snapshots scheduled for %d URIs (every %ds)", len(uris), interval)
	return nil
}

// snapshotAllWebviewCookies dumps cookies for all URIs immediately.
func snapshotAllWebviewCookies(uris []string) {
	if len(uris) == 0 {
		return
	}
	path, err := webviewCookiesPath()
	if err != nil {
		return
	}
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	arr := C.malloc(C.size_t((len(uris) + 1)) * C.size_t(unsafe.Sizeof(uintptr(0))))
	if arr == nil {
		return
	}
	defer C.free(arr)
	us := unsafe.Slice((*uintptr)(arr), len(uris)+1)
	for i, u := range uris {
		us[i] = uintptr(unsafe.Pointer(C.CString(u)))
	}
	us[len(uris)] = 0
	C.snapshot_all_sync(cpath, (**C.char)(arr), C.int(len(uris)))
	for i := 0; i < len(uris); i++ {
		C.free(unsafe.Pointer(us[i]))
	}
}