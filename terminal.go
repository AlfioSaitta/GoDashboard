//go:build linux

package main

/*
#cgo linux pkg-config: gtk+-3.0 vte-2.91
#cgo !webkit2_41 pkg-config: webkit2gtk-4.0
#cgo webkit2_41 pkg-config: webkit2gtk-4.1

#include <stdlib.h>
#include <string.h>
#include <signal.h>
#include <unistd.h>
#include <glib.h>
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
#include <vte/vte.h>

// Per-tab SSH terminal (VTE). Every tab box ("tab-<id>-box", see tabs_shell.go)
// can host, lazily, a header bar ("#term-bar") plus a VteTerminal. When the
// terminal is visible the tab's WebKitWebView is hidden; closing the terminal
// restores the webview. All entry points run on the GTK main thread (posted
// there by shell_req_cb in tabs_shell.go).

// ---------------------------------------------------------------------------
// Session state
// ---------------------------------------------------------------------------

typedef struct {
	int id;
	gboolean running;   // a child process is live
	gboolean visible;   // terminal widgets are shown
	GPid pid;
	int orient;         // 0 = vertical (page above, terminal below); 1 = horizontal (page left, terminal right)
	char *host;
	int port;
	char *user;
	char *auth;         // "password" | "key" | "agent" | "" (agent)
	char *password;
	char *key;
	char *dir;
	GtkWidget *bar;     // #term-bar header
	GtkWidget *term;    // VteTerminal
	GtkWidget *termbox; // vbox(bar, term)
	GtkWidget *wv;      // the tab's WebKitWebView (moved into/out of the paned)
	GtkWidget *split_btn; // split-orientation toggle
	GtkWidget *paned;      // GtkPaned sharing the box between webview and termbox
	int paned_retries;     // term_paned_place retry counter
} term_session;

// Registry keyed by tab id. Sessions are leaked on tab destroy on purpose: a
// pending g_child_watch may still reference the struct after the tab is gone.
static GHashTable *term_sessions = NULL;

static term_session *term_find(int id)
{
	if (!term_sessions)
		return NULL;
	return g_hash_table_lookup(term_sessions, GINT_TO_POINTER(id));
}

// Defined below; referenced by term_spawn_cb (which is defined first).
static void term_child_watch(GPid pid, gint status, gpointer user_data);

// Split-layout helpers (defined in the "Header bar" section below but used by
// the session code earlier).
static void term_apply_split(term_session *s);
static void term_split_cb(GtkButton *btn, gpointer user_data);

// ---------------------------------------------------------------------------
// Forward declarations (entry points wired from tabs_shell.go shell_req_cb)
// ---------------------------------------------------------------------------

void shell_terminal_toggle(int id, const char *host, int port, const char *user,
                           const char *auth, const char *password,
                           const char *key, const char *dir, int split);
void shell_terminal_open(int id, const char *host, int port, const char *user,
                         const char *auth, const char *password,
                         const char *key, const char *dir, int split);
void shell_terminal_close(int id);
void shell_terminal_restart(int id);
void shell_terminal_destroy(int id);
void shell_terminal_kill(int id);
void shell_terminal_reset_state(int id);

// Exported from terminal.go (Go side); forwards state to the chrome.
extern void exportShellTerminalState(int id, int running, int visible);

// Defined in tabs_shell.go's preamble (request queue + askpass path storage).
extern void shell_term_request(int op, int id, int visible, const char *host, int port,
                               const char *user, const char *auth, const char *password,
                               const char *key, const char *dir, int split);
extern void shell_term_set_askpass(const char *path);

// Cross-unit references to tabs_shell.go's C shim (separate cgo unit).
extern char *g_term_askpass;
extern GtkWidget *shell_find_tab_box(int id);
extern GtkWidget *shell_find_tab_paned(int id);
extern WebKitWebView *shell_find_tab(int id);

// ---------------------------------------------------------------------------
// Spawn / child-watch
// ---------------------------------------------------------------------------

static void term_spawn_cb(VteTerminal *terminal, GPid pid, GError *error, gpointer user_data)
{
	term_session *s = user_data;
	// Free the argv/envv we stashed on the terminal for the spawn lifetime.
	char **argv = g_object_get_data(G_OBJECT(terminal), "term-argv");
	char **envv = g_object_get_data(G_OBJECT(terminal), "term-envv");
	if (argv) {
		g_strfreev(argv);
		g_object_set_data(G_OBJECT(terminal), "term-argv", NULL);
	}
	if (envv) {
		g_strfreev(envv);
		g_object_set_data(G_OBJECT(terminal), "term-envv", NULL);
	}
	if (error) {
		s->running = FALSE;
		s->pid = 0;
		g_printerr("[terminal] spawn error: %s\n", error->message);
		char buf[1024];
		snprintf(buf, sizeof buf, "\r\n[spawn error] %s\r\n", error->message);
		vte_terminal_feed(terminal, buf, -1);
		g_clear_error(&error);
		exportShellTerminalState(s->id, 0, s->visible ? 1 : 0);
		return;
	}
	s->pid = pid;
	s->running = TRUE;
	g_child_watch_add(pid, (GChildWatchFunc)term_child_watch, s);
	exportShellTerminalState(s->id, 1, s->visible ? 1 : 0);
}

static void term_child_watch(GPid pid, gint status, gpointer user_data)
{
	term_session *s = user_data;
	g_spawn_close_pid(pid);
	// Only the current child may clear the running state (a restart respawns
	// a new child before the old one's watch fires).
	if (s->pid == pid) {
		s->running = FALSE;
		s->pid = 0;
		exportShellTerminalState(s->id, 0, s->visible ? 1 : 0);
	}
}

static void term_spawn(term_session *s)
{
	if (!s->term || s->running)
		return;

	char **argv = g_new0(char *, 16);
	char **envv = NULL;
	int i = 0;

	if (s->host && s->host[0]) {
		argv[i++] = g_strdup("ssh");
		argv[i++] = g_strdup("-tt");
		argv[i++] = g_strdup("-o");
		argv[i++] = g_strdup("StrictHostKeyChecking=accept-new");
		argv[i++] = g_strdup("-o");
		argv[i++] = g_strdup("ConnectTimeout=15");
		if (s->port > 0 && s->port != 22) {
			argv[i++] = g_strdup("-p");
			argv[i++] = g_strdup_printf("%d", s->port);
		}
		if (s->auth && strcmp(s->auth, "key") == 0 && s->key && s->key[0]) {
			argv[i++] = g_strdup("-i");
			argv[i++] = g_strdup(s->key);
		}
		if (s->user && s->user[0])
			argv[i++] = g_strdup_printf("%s@%s", s->user, s->host);
		else
			argv[i++] = g_strdup(s->host);

		// Password auth: ask ssh to fetch the password via the SSH_ASKPASS
		// helper (written by Go next to the data dir). A non-NULL envv REPLACES
		// the child environment, so clone the current one and add the vars.
		if (s->auth && strcmp(s->auth, "password") == 0 && g_term_askpass && g_term_askpass[0]) {
			extern char **environ;
			int n = 0;
			while (environ[n])
				n++;
			envv = g_new0(char *, n + 4);
			for (int k = 0; k < n; k++)
				envv[k] = g_strdup(environ[k]);
			int e = n;
			envv[e++] = g_strdup_printf("SSH_ASKPASS=%s", g_term_askpass);
			envv[e++] = g_strdup("SSH_ASKPASS_REQUIRE=force");
			envv[e++] = g_strdup_printf("DASH_SSH_PASSWORD=%s", s->password ? s->password : "");
			envv[e] = NULL;
		}
	} else {
		// No host configured: open a local shell in the configured dir.
		const char *shell = g_getenv("SHELL");
		argv[i++] = g_strdup(shell && shell[0] ? shell : "/bin/sh");
	}
	argv[i] = NULL;

	// Keep argv/envv alive for the (async) spawn; freed in term_spawn_cb.
	g_object_set_data(G_OBJECT(s->term), "term-argv", argv);
	g_object_set_data(G_OBJECT(s->term), "term-envv", envv);

	const char *wd = (s->dir && s->dir[0]) ? s->dir : NULL;
	vte_terminal_spawn_async(VTE_TERMINAL(s->term),
	                         VTE_PTY_DEFAULT,
	                         wd,
	                         argv, envv,
	                         G_SPAWN_SEARCH_PATH | G_SPAWN_DO_NOT_REAP_CHILD,
	                         NULL, NULL, NULL,
	                         -1, NULL,
	                         term_spawn_cb, s);
}

// ---------------------------------------------------------------------------
// Split layout (GtkPaned): the tab webview stays VISIBLE and shares the box
// with the terminal box, split by a draggable divider.
//
// ARCHITECTURE (validated with an isolated GTK3 test): the GtkPaned is created
// at TAB CREATION (tabs_shell.go shell_show_tab) with the webview as child1
// and no child2. The webview is NEVER reparented — moving a realized/mapped
// WebKitWebView into a new container corrupts GTK's css/draw state (blank page
// + gtk_css_node_insert_after assertions). Opening the terminal only packs the
// termbox as child2; closing it removes child2 (the webview then fills the box
// again). The terminal pane uses resize=FALSE so it stays glued to the
// bottom/right at a fixed size while the page absorbs window resizes.
// ---------------------------------------------------------------------------

// Fixed terminal size in pixels along the split axis.
#define TERM_TERM_PX 160

// Place the divider so the terminal keeps TERM_TERM_PX (glued to the bottom/
// right); retries until the paned has a real allocation.
static gboolean term_paned_place(gpointer data)
{
	term_session *s = data;
	if (s->paned && gtk_widget_get_mapped(s->paned)) {
		GtkAllocation a;
		gtk_widget_get_allocation(s->paned, &a);
		int total = (s->orient == 0) ? a.height : a.width;
		if (total > TERM_TERM_PX) {
			gtk_paned_set_position(GTK_PANED(s->paned), total - TERM_TERM_PX);
			return G_SOURCE_REMOVE;
		}
	}
	if (++s->paned_retries > 60) // give up after ~1.8s (terminal may have closed)
		return G_SOURCE_REMOVE;
	return G_SOURCE_CONTINUE;
}

static void term_split_label_update(term_session *s)
{
	if (!s->split_btn)
		return;
	gtk_button_set_label(GTK_BUTTON(s->split_btn), s->orient ? "⇆" : "⇅");
	gtk_widget_set_tooltip_text(s->split_btn,
		s->orient ? "Affianca: pagina a sinistra, terminale a destra"
		          : "Impila: pagina sopra, terminale sotto");
}

// (Re)configure the tab's paned for the terminal. The paned already exists
// (created with the tab box in shell_show_tab, webview = child1); here we only
// pick the orientation and (re)pack the terminal box as child2. The webview is
// never hidden and never reparented.
static void term_apply_split(term_session *s)
{
	if (!s->visible)
		return;
	GtkWidget *paned = s->paned;
	if (!paned)
		return;

	gtk_orientable_set_orientation(GTK_ORIENTABLE(paned),
		s->orient ? GTK_ORIENTATION_HORIZONTAL : GTK_ORIENTATION_VERTICAL);

	// (Re)attach the terminal box as child2 if it is not already there.
	if (gtk_paned_get_child2(GTK_PANED(paned)) != s->termbox) {
		GtkWidget *old = gtk_paned_get_child2(GTK_PANED(paned));
		if (old)
			gtk_container_remove(GTK_CONTAINER(paned), old);
		gtk_paned_pack2(GTK_PANED(paned), s->termbox, FALSE, TRUE);
		gtk_widget_show(s->termbox);
	}

	gtk_widget_show(s->wv);
	gtk_widget_show(s->termbox);
	g_timeout_add(30, term_paned_place, s);
}

// ---------------------------------------------------------------------------
// Header bar (host label + Split / Restart / Close buttons)
// ---------------------------------------------------------------------------

static void term_restart_cb(GtkButton *btn, gpointer user_data)
{
	term_session *s = user_data;
	shell_terminal_restart(s->id);
}

static void term_close_cb(GtkButton *btn, gpointer user_data)
{
	term_session *s = user_data;
	shell_terminal_close(s->id);
}

static void term_split_cb(GtkButton *btn, gpointer user_data)
{
	term_session *s = user_data;
	s->orient = s->orient ? 0 : 1;
	term_split_label_update(s);
	term_apply_split(s);
	// Persist the choice (Go writes it back into the service terminal config).
	extern void exportShellTerminalSplit(int id, int orient);
	exportShellTerminalSplit(s->id, s->orient);
}

static GtkWidget *term_bar_new(term_session *s)
{
	GtkWidget *bar = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
	gtk_widget_set_name(bar, "term-bar");

	const char *host = s->host ? s->host : "";
	const char *user = s->user ? s->user : "";
	char *lbl = g_strdup_printf("%s%s%s", user, user[0] ? "@" : "", host);
	GtkWidget *lab = gtk_label_new(lbl);
	g_free(lbl);
	gtk_widget_set_halign(lab, GTK_ALIGN_START);
	gtk_box_pack_start(GTK_BOX(bar), lab, FALSE, FALSE, 0);

	GtkWidget *sp = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
	gtk_widget_set_hexpand(sp, TRUE);
	gtk_box_pack_start(GTK_BOX(bar), sp, TRUE, TRUE, 0);

	GtkWidget *split = gtk_button_new_with_label(s->orient ? "⇆" : "⇅");
	gtk_widget_set_name(split, "term-split-btn");
	g_signal_connect(split, "clicked", G_CALLBACK(term_split_cb), s);
	gtk_box_pack_start(GTK_BOX(bar), split, FALSE, FALSE, 0);
	s->split_btn = split;

	GtkWidget *restart = gtk_button_new_with_label("Restart");
	g_signal_connect(restart, "clicked", G_CALLBACK(term_restart_cb), s);
	gtk_box_pack_start(GTK_BOX(bar), restart, FALSE, FALSE, 0);

	GtkWidget *close = gtk_button_new_with_label("Close");
	g_signal_connect(close, "clicked", G_CALLBACK(term_close_cb), s);
	gtk_box_pack_start(GTK_BOX(bar), close, FALSE, FALSE, 0);

	return bar;
}

// ---------------------------------------------------------------------------
// Create / show / hide
// ---------------------------------------------------------------------------

static term_session *term_create(int id, const char *host, int port, const char *user,
                                 const char *auth, const char *password,
                                 const char *key, const char *dir, int split)
{
	GtkWidget *box = shell_find_tab_box(id);
	WebKitWebView *wv = shell_find_tab(id);
	if (!box || !wv)
		return NULL;

	term_session *s = g_new0(term_session, 1);
	s->id = id;
	s->orient = split == 1 ? 1 : 0;
	s->visible = TRUE;
	s->wv = GTK_WIDGET(wv);
	s->host = host ? g_strdup(host) : NULL;
	s->port = port;
	s->user = user ? g_strdup(user) : NULL;
	s->auth = auth ? g_strdup(auth) : NULL;
	s->password = password ? g_strdup(password) : NULL;
	s->key = key ? g_strdup(key) : NULL;
	s->dir = dir ? g_strdup(dir) : NULL;
	// The paned already exists: it wraps the webview since tab creation
	// (shell_show_tab). The terminal only uses its child2 slot.
	s->paned = shell_find_tab_paned(id);

	s->bar = term_bar_new(s);
	s->term = vte_terminal_new();
	// Scrollback: VteTerminal is a GtkScrollable — without a host
	// GtkScrolledWindow it can buffer scrollback but has NO visible scrollbar.
	vte_terminal_set_scrollback_lines(VTE_TERMINAL(s->term), 10000);
	vte_terminal_set_scroll_on_output(VTE_TERMINAL(s->term), FALSE);
	vte_terminal_set_scroll_on_keystroke(VTE_TERMINAL(s->term), TRUE);
	GtkWidget *scr = gtk_scrolled_window_new(NULL, NULL);
	gtk_widget_set_name(scr, "term-scroll");
	gtk_scrolled_window_set_policy(GTK_SCROLLED_WINDOW(scr),
	                               GTK_POLICY_NEVER, GTK_POLICY_AUTOMATIC);
	gtk_scrolled_window_set_overlay_scrolling(GTK_SCROLLED_WINDOW(scr), FALSE);
	gtk_widget_set_hexpand(scr, TRUE);
	gtk_widget_set_vexpand(scr, TRUE);
	gtk_container_add(GTK_CONTAINER(scr), s->term);
	gtk_widget_show(s->term);
	s->termbox = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
	gtk_box_pack_start(GTK_BOX(s->termbox), s->bar, FALSE, FALSE, 0);
	gtk_box_pack_start(GTK_BOX(s->termbox), scr, TRUE, TRUE, 0);
	gtk_widget_show(scr);
	gtk_widget_show(s->bar);

	// NOTE: the webview is NOT hidden — the paned shares the box between the
	// page and the terminal (see term_apply_split).
	term_apply_split(s);

	if (!term_sessions)
		term_sessions = g_hash_table_new(g_direct_hash, g_direct_equal);
	g_hash_table_insert(term_sessions, GINT_TO_POINTER(id), s);
	return s;
}

// ---------------------------------------------------------------------------
// Entry points (called from shell_req_cb / shell_destroy_tab)
// ---------------------------------------------------------------------------

void shell_terminal_open(int id, const char *host, int port, const char *user,
                         const char *auth, const char *password,
                         const char *key, const char *dir, int split)
{
	term_session *s = term_find(id);
	if (!s) {
		s = term_create(id, host, port, user, auth, password, key, dir, split);
		if (!s)
			return;
	}
	// The webview stays visible inside the paned (see term_apply_split).
	if (!s->running)
		term_spawn(s);
	if (s->term)
		gtk_widget_grab_focus(s->term);
	exportShellTerminalState(s->id, s->running ? 1 : 0, 1);
}

void shell_terminal_close(int id)
{
	term_session *s = term_find(id);
	if (!s)
		return;
	// Closing the terminal terminates the connection: keeping an invisible
	// ssh running would buffer output forever in the hidden pty.
	if (s->running && s->pid > 0)
		kill(s->pid, SIGHUP);
	s->running = FALSE;
	s->visible = FALSE;
	exportShellTerminalState(s->id, 0, 0);

	// Drop the terminal box from the paned's child2 slot. The paned stays in
	// the tab box with the webview as its only child (the webview then fills
	// the whole box again). The webview is NEVER reparented.
	if (s->paned) {
		GtkWidget *c2 = gtk_paned_get_child2(GTK_PANED(s->paned));
		if (c2 == s->termbox)
			gtk_container_remove(GTK_CONTAINER(s->paned), c2);
		gtk_widget_show(s->wv);
	}
	s->paned = NULL;
	s->bar = NULL;
	s->term = NULL;
	s->termbox = NULL;
	s->split_btn = NULL;
	shell_terminal_reset_state(id);
}

void shell_terminal_toggle(int id, const char *host, int port, const char *user,
                           const char *auth, const char *password,
                           const char *key, const char *dir, int split)
{
	term_session *s = term_find(id);
	if (s && s->visible) {
		shell_terminal_close(id);
		return;
	}
	shell_terminal_open(id, host, port, user, auth, password, key, dir, split);
}

// Apply a split orientation to an ALREADY-OPEN terminal (op 15; programmatic
// calls). The split is persisted by the Go caller.
void shell_terminal_split(int id, int orient)
{
	term_session *s = term_find(id);
	if (!s || !s->visible)
		return;
	int want = orient == 1 ? 1 : 0;
	if (s->orient == want)
		return;
	s->orient = want;
	term_split_label_update(s);
	term_apply_split(s);
}

void shell_terminal_restart(int id)
{
	term_session *s = term_find(id);
	if (!s)
		return;
	if (s->running && s->pid > 0) {
		kill(s->pid, SIGHUP);
		s->running = FALSE;
		s->pid = 0;
	}
	term_spawn(s);
}

// Kill a running child (used by shell_destroy_tab BEFORE tearing down the box,
// so the pty child exits promptly).
void shell_terminal_kill(int id)
{
	term_session *s = term_find(id);
	if (!s || !s->running || s->pid <= 0)
		return;
	kill(s->pid, SIGHUP);
	s->running = FALSE;
}

// Drop the session registry entry for a destroyed tab. The struct itself is
// leaked deliberately: a pending g_child_watch may still point at it (and the
// widgets are destroyed by the caller when the tab box is torn down).
void shell_terminal_reset_state(int id)
{
	if (!term_sessions)
		return;
	g_hash_table_remove(term_sessions, GINT_TO_POINTER(id));
}

void shell_terminal_destroy(int id)
{
	shell_terminal_kill(id);
	shell_terminal_reset_state(id);
}
*/
import "C"

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"unsafe"

	"dashboard/internal/paths"
	"dashboard/internal/tab"
)

// ---------------------------------------------------------------------------
// Go helpers
// ---------------------------------------------------------------------------

var askpassOnce sync.Once

// ensureAskpassHelper writes the SSH_ASKPASS script (echoes the password from
// the DASH_SSH_PASSWORD env var set by the C spawner) into the portable data
// dir and tells the C shim where it lives. Idempotent across the process.
func ensureAskpassHelper() {
	askpassOnce.Do(func() {
		p := paths.File("ssh-askpass.sh")
		content := "#!/bin/sh\n" +
			"# SSH_ASKPASS helper for the Dashboard per-tab terminal (written at runtime).\n" +
			"if [ -n \"$DASH_SSH_PASSWORD\" ]; then\n" +
			"  echo \"$DASH_SSH_PASSWORD\"\n" +
			"  exit 0\n" +
			"fi\n" +
			"exit 1\n"
		if err := os.WriteFile(p, []byte(content), 0o700); err != nil {
			logger.Printf("ensureAskpassHelper: cannot write %s: %v", p, err)
			return
		}
		shellTermSetAskpass(p)
		logger.Printf("ensureAskpassHelper: written %s", p)
	})
}

func shellTermSetAskpass(path string) {
	c := C.CString(path)
	defer C.free(unsafe.Pointer(c))
	C.shell_term_set_askpass(c)
}

func termCStr(s string) *C.char {
	if s == "" {
		return nil
	}
	return C.CString(s)
}

// shellTermRequest enqueues a terminal op (11 toggle, 12 open/close, 14
// restart, 15 split) onto the GTK main thread. Strings are copied by
// shell_term_request. split: 0 = vertical (page above), 1 = horizontal.
func shellTermRequest(op, id, visible int, host string, port int, user, auth, password, key, dir string, split int) {
	ensureAskpassHelper()
	cHost := termCStr(host)
	cUser := termCStr(user)
	cAuth := termCStr(auth)
	cPassword := termCStr(password)
	cKey := termCStr(key)
	cDir := termCStr(dir)
	defer func() {
		if cHost != nil {
			C.free(unsafe.Pointer(cHost))
		}
		if cUser != nil {
			C.free(unsafe.Pointer(cUser))
		}
		if cAuth != nil {
			C.free(unsafe.Pointer(cAuth))
		}
		if cPassword != nil {
			C.free(unsafe.Pointer(cPassword))
		}
		if cKey != nil {
			C.free(unsafe.Pointer(cKey))
		}
		if cDir != nil {
			C.free(unsafe.Pointer(cDir))
		}
	}()
	C.shell_term_request(C.int(op), C.int(id), C.int(visible), cHost, C.int(port),
		cUser, cAuth, cPassword, cKey, cDir, C.int(split))
}

// termSplitInt maps a configured split value to the C orientation code.
func termSplitInt(s string) int {
	if strings.EqualFold(s, "h") {
		return 1
	}
	return 0
}

// exportShellTerminalState forwards the per-tab terminal state (running /
// visible) to the chrome strip so the frontend can reflect it on the tab bar.
// Defined in terminal_export.go (its own file: a //export here would pull this
// file's C preamble into the amalgamated export object and duplicate symbols).

// ---------------------------------------------------------------------------
// App methods (Wails-bound)
// ---------------------------------------------------------------------------

// serviceKeyForURL returns the configured service key whose base/backoffice/
// frontend URL is a prefix of the given URL (tabs persist the RESOLVED real
// URL, e.g. "http://localhost:8000/admin", while service keys are
// "neuronet"/"minecraft"/"slotbuilder"). Exact key match is handled by the
// caller. Empty result means no service owns this URL.
func (a *App) serviceKeyForURL(u string) string {
	u = strings.ToLower(strings.TrimSpace(u))
	u = strings.TrimRight(u, "/")
	for key, svc := range a.cfg.Services {
		for _, base := range []string{svc.BaseURL, svc.BackofficeURL, svc.FrontendURL} {
			base = strings.TrimRight(strings.ToLower(strings.TrimSpace(base)), "/")
			if base == "" || u == "" {
				continue
			}
			if u == base || strings.HasPrefix(u, base) {
				return key
			}
		}
	}
	return ""
}

// terminalParams resolves the SSH terminal parameters for a tab: service tabs
// (matched by key OR by resolved URL prefix against the service base/backoffice/
// frontend URLs) use the service TerminalConfig; any other tab gets a local
// shell (no host). An empty result with auth "" means the terminal is disabled
// for the tab. split is "v" (default) or "h".
func (a *App) terminalParams(t tab.Tab) (host string, port int, user, auth, password, key, dir, split string) {
	svcKey := strings.ToLower(strings.TrimSpace(t.URL))
	svc, ok := a.cfg.Services[svcKey]
	if !ok || svcKey == "" {
		if resolved := a.serviceKeyForURL(t.URL); resolved != "" {
			svcKey = resolved
			svc, ok = a.cfg.Services[resolved]
		}
	}
	if !ok {
		// Custom tab (not a service): open a LOCAL shell (host empty is handled
		// by the C spawner). auth "none" marks the "allowed" default.
		return "", 0, "", "none", "", "", "", "v"
	}
	tc := svc.Terminal
	// An explicitly configured terminal (remote host OR local shell dir) stays
	// active even if the "enabled" flag was not persisted (older runtime configs
	// migrated without it). Only unconfigured blocks are gated off.
	if !tc.Enabled && tc.Host == "" && tc.Dir == "" {
		return
	}
	auth = tc.Auth
	if auth == "" {
		auth = "agent"
	}
	if tc.PasswordEnv != "" {
		password = os.Getenv(tc.PasswordEnv)
	}
	split = tc.Split
	if split == "" {
		split = "v"
	}
	return tc.Host, tc.Port, tc.User, auth, password, tc.KeyPath, tc.Dir, split
}

// terminalServiceKey returns the config service key a tab belongs to (its URL
// as key, or resolved by URL prefix). Empty when the tab is not a service tab.
func (a *App) terminalServiceKey(tabID int) string {
	t, found := a.tabManager.Get(tabID)
	if !found {
		return ""
	}
	u := strings.ToLower(strings.TrimSpace(t.URL))
	if _, ok := a.cfg.Services[u]; ok {
		return u
	}
	return a.serviceKeyForURL(t.URL)
}

func (a *App) terminalParamsForTab(tabID int) (host string, port int, user, auth, password, key, dir, split string, ok bool) {
	t, found := a.tabManager.Get(tabID)
	if !found {
		return "", 0, "", "", "", "", "", "", false
	}
	host, port, user, auth, password, key, dir, split = a.terminalParams(t)
	if auth == "" {
		return "", 0, "", "", "", "", "", "", false
	}
	return host, port, user, auth, password, key, dir, split, true
}

// terminalSetSplitPersist writes the chosen split orientation back into the
// service terminal config (used by the C split-button callback).
func (a *App) terminalSetSplitPersist(tabID, orient int) {
	key := a.terminalServiceKey(tabID)
	if key == "" {
		return
	}
	split := "v"
	if orient != 0 {
		split = "h"
	}
	svc := a.cfg.Services[key]
	if svc.Terminal.Split == split {
		return
	}
	svc.Terminal.Split = split
	a.cfg.Services[key] = svc
	logger.Printf("terminal split: tab=%d service=%s -> %s", tabID, key, split)
	if err := a.cfg.Save(paths.ConfigFile()); err != nil {
		logger.Printf("terminal split: save failed: %v", err)
	}
}

// TerminalToggle toggles the per-tab SSH terminal (op 11).
func (a *App) TerminalToggle(ctx context.Context, tabID int) {
	host, port, user, auth, password, key, dir, split, ok := a.terminalParamsForTab(tabID)
	if !ok {
		logger.Printf("TerminalToggle: tab %d has no terminal config", tabID)
		return
	}
	logger.Printf("TerminalToggle: tab=%d host=%s port=%d user=%s auth=%s split=%s", tabID, host, port, user, auth, split)
	shellTermRequest(11, tabID, 0, host, port, user, auth, password, key, dir, termSplitInt(split))
}

func (a *App) TerminalToggleNoContext(tabID int) {
	a.TerminalToggle(context.Background(), tabID)
}

// TerminalOpen opens the per-tab SSH terminal (op 12, visible).
func (a *App) TerminalOpen(ctx context.Context, tabID int) {
	host, port, user, auth, password, key, dir, split, ok := a.terminalParamsForTab(tabID)
	if !ok {
		logger.Printf("TerminalOpen: tab %d has no terminal config", tabID)
		return
	}
	logger.Printf("TerminalOpen: tab=%d host=%s", tabID, host)
	shellTermRequest(12, tabID, 1, host, port, user, auth, password, key, dir, termSplitInt(split))
}

func (a *App) TerminalOpenNoContext(tabID int) {
	a.TerminalOpen(context.Background(), tabID)
}

// TerminalClose closes (hides + disconnects) the per-tab SSH terminal.
func (a *App) TerminalClose(ctx context.Context, tabID int) {
	logger.Printf("TerminalClose: tab=%d", tabID)
	shellTermRequest(12, tabID, 0, "", 0, "", "", "", "", "", 0)
}

func (a *App) TerminalCloseNoContext(tabID int) {
	a.TerminalClose(context.Background(), tabID)
}

// TerminalRestart kills and respawns the per-tab SSH terminal session.
func (a *App) TerminalRestart(ctx context.Context, tabID int) {
	logger.Printf("TerminalRestart: tab=%d", tabID)
	shellTermRequest(14, tabID, 0, "", 0, "", "", "", "", "", 0)
}

func (a *App) TerminalRestartNoContext(tabID int) {
	a.TerminalRestart(context.Background(), tabID)
}

// TerminalSplit sets the split orientation ("h" horizontal / "v" vertical) for
// the tab's service terminal: it persists the choice in config.yaml AND — when
// the terminal is already open — switches the live layout (op 15).
func (a *App) TerminalSplit(ctx context.Context, tabID int, orient string) error {
	key := a.terminalServiceKey(tabID)
	if key == "" {
		return fmt.Errorf("nessun servizio per la tab %d", tabID)
	}
	want := "v"
	if strings.EqualFold(orient, "h") {
		want = "h"
	}
	svc := a.cfg.Services[key]
	if svc.Terminal.Split != want {
		svc.Terminal.Split = want
		a.cfg.Services[key] = svc
		if err := a.cfg.Save(paths.ConfigFile()); err != nil {
			logger.Printf("TerminalSplit: save failed: %v", err)
			return err
		}
	}
	logger.Printf("TerminalSplit: tab=%d service=%s -> %s", tabID, key, want)
	shellTermRequest(15, tabID, 0, "", 0, "", "", "", "", "", termSplitInt(want))
	return nil
}

func (a *App) TerminalSplitNoContext(tabID int, orient string) error {
	return a.TerminalSplit(context.Background(), tabID, orient)
}
