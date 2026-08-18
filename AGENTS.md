# AGENTS.md - Dashboard Development Guide

A Go/Wails v2 desktop dashboard for monitoring three services (shown here as
generic placeholders — configure your own services/routes in `config.yaml`):
- **Alpha** - an AI research/admin console at `http://localhost:8080/admin`
- **Beta** - a gaming community network admin panel at `https://admin.example.com`
- **Gamma** - a backoffice at `https://backoffice.example.com`, with a public frontend at `https://www.example.com`

The app **works**: builds, runs, renders the UI, loads tabs from Go, and each tab is a
**native WebKitWebView** (not an iframe). Chrome (header + tab bar) is a thin fixed
strip; content lives in a `GtkStack` of per-tab webviews. Tray/frameless/theme/cookies
work. Tab **Web Notifications** are lifted to the desktop via D-Bus
(org.freedesktop.Notifications). This file documents the REAL project structure
(verified by inspection), not a plan.

## Project Structure
```
/path/to/Dashboard/
├── main.go                # EVERYTHING app-level: App struct, all Wails-bound methods, wails.Run()
├── tabs_shell.go          # cgo (package main): native tab shell — GtkStack + per-tab WebKitWebView (webkit2_41)
├── tabs_shell_export.go   # cgo: //export callbacks (shell title/uri → wails events) + shellCtx
├── terminal.go            # cgo (package main): per-tab SSH/local terminal (VTE) in a GtkPaned split
├── terminal_export.go     # cgo: //export callbacks (terminal state/split → wails events)
├── shell_app.go           # App methods: ShellShowTab/ShellDestroyTab/ShellReorder/ShellZoom/
│                          #   ShellSetChromeHeight/OpenSettings/OpenNotes/TabsChanged + resolveTabURL
├── settings_bridge.go     # Bridge Go for the settings WINDOW (dispatch on App + __dashReply)
├── notes_bridge.go        # Bridge Go for the per-tab NOTES WINDOW (own webview + __dashReply)
├── notifications_bridge.go# cgo: closes a tab's WebKit notification on the GTK thread when its desktop notification is dismissed
├── inspector.go           # cgo: WebKitGTK page inspector per single tab webview (dock bottom/right/left/float)
├── persistent_cookies.go  # cgo: WebKitGTK cookie manager → SQLite persistent storage (webkit2_41)
├── app_config.go          # App methods: GetAppConfig/SaveAppConfig — service config editing (url/auth/terminal)
├── wails.json             # Wails v2 config (embed dir: frontend/dist)
├── config.yaml            # Dev config; migrated into build/bin/data/config.yaml at first run
├── build.sh               # Build: npm run build + wails build -s -tags webkit2_41
├── go.mod / go.sum        # Go 1.25.0, Wails v2.14.0
├── internal/paths/        # NOTE: portable data dir resolver (<exedir>/data + legacy migration)
├── build/
│   ├── icon.png           # Embedded app icon (go:embed)
│   └── bin/Dashboard      # Compiled binary (all runtime data in ./data next to it)
├── frontend/
│   ├── index.html         # Vite input (at frontend ROOT, not src/); gamepad-style placeholder
│   ├── settings.html      # Vite input for the NATIVE settings window (no Wails runtime)
│   ├── notes.html         # Vite input for the per-tab NATIVE notes window (no Wails runtime)
│   ├── src/
│   │   ├── main.js        # Imports main.css + mounts App on DOMContentLoaded
│   │   ├── settings/      # Settings-window SPA (separate bundle): bridge + SettingsModal
│   │   │   └── main.js
│   │   ├── notes/         # Notes-window SPA (separate bundle): bridge + notes card UI
│   │   │   └── main.js
│   │   ├── app.js         # Renders chrome strip (header + tab bar) OR settings view (#settings)
│   │   ├── components/
│   │   │   ├── TabBar/TabBar.js             # Tab bar: active, drag&drop order, context menu, rename
│   │   │   ├── SettingsModal/SettingsModal.js # Modal (full-window in native settings window) to add/edit/remove tabs
│   │   │   └── Shared/{utils,services}.js  # icon; urlForTab
│   │   ├── services/api.js                  # Stateless wrappers over wailsjs bindings (incl. shell*)
│   │   ├── stores/dashboard.js              # Singleton store: tabs, lastStatuses, listeners
│   │   └── styles/main.css                  # THEME: CSS variables, dark/light, tab bar, modal, cards
│   ├── wailsjs/           # AUTO-GENERATED bindings (do not hand-edit)
│   ├── dist/              # Built assets embedded by Wails
│   ├── vite.config.js     # Multi-entry build (index/settings/notes); CreateApp bootstrap only in index.html
│   └── package.json
└── internal/
    ├── api/
    │   └── dashboard.go   # DashboardAPI (service health + statuses), TabAPI (tabs), API response types
    ├── atomicwrite/       # Safe atomic file writes (used by config/tab persistence)
    ├── config/config.go   # Config struct, Load/Default/Save, env-var auth overrides
    ├── models/dashboard.go# Internal model types (services, health statuses)
    ├── notify/notify.go   # D-Bus org.freedesktop.Notifications client (desktop notifications): Notify/Replace + NotificationClosed
    ├── services/
    │   ├── clients.go     # HTTPClient + Alpha/Beta/Gamma clients + auth (env)
    │   └── manager.go     # ServiceManager: CheckAllHealth + per-service statuses
    ├── tab/manager.go     # TabManager: persistent tabs (data/tabs.json)
    └── tray/              # D-Bus StatusNotifierItem tray (sni.go, menu.go) via godbus
```

## Backend Architecture

### main.go — the single source of app wiring
The `App` struct holds everything: `cfg`, `manager`, `dashboardAPI`, `tabAPI`,
`tabManager`, `tray`, `notifier` (D-Bus notifications),
`notifMap`/`notifRev` (webkit-notification-id ↔ desktop-notification-id maps).
`NewApp()` wires them in order. `main()` sets `GDK_BACKEND=x11` for KDE Wayland
(X11/XWayland required for frameless), enables persistent cookies, calls
`shellSetup()` (re-groups the window into the chrome strip + tab stack, see
tabs_shell.go) and `refreshNotificationOrigins()` (pre-allow tab/service origins
for Web Notifications, BEFORE the first web process launches) THEN `wails.Run()`.
- `Frameless: true`, `SingleInstanceLock` (`com.example.Dashboard`), `OpenInspectorOnStartup: false`
  (the WebKit inspector is opened on demand from the header dropdown / Ctrl+Shift+F12 once
  the binary is built with the `devtools` tag, see inspector.go)
- `Linux.WebviewGpuPolicy`: from config `ui.webview_gpu_policy` (`always` default → hardware
  acceleration; `never` = software rendering). Runs on X11 backend.

`go:embed frontend/dist frontend/wailsjs` + `go:embed build/icon.png`.

`App.Startup` stores the context in `app.ctx` AND in the package-level `shellCtx`
(tabs_shell_export.go) so the exported C callbacks can emit events after startup.

### tabs_shell.go — the native tab shell (cgo, tag `webkit2_41`)
The core of the new architecture. The single Wails webview is repackaged into a
**fixed chrome strip** and the rest of the window is filled by a `GtkStack` that
holds one **own WebKitWebView per tab**:
- `main()` runs `shellSetup()` before `wails.Run()`. Because Wails packs the raw
  webview into `vbox → webviewBox` only when `Frontend.Run` executes (before
  `gtk_main`), the idle callback registered for setup runs on the FIRST main-loop
  iteration and reparents/re-sizes the widgets — no visible flash.
- Tree: `GtkWindow → vbox → chromeBox (the Wails webview, height = strip) +   GtkStack "shell-stack" (expand)`.
- Every tab webview is lazy: created on first `shell_show_tab` by name `tab-<id>`,
  `webkit_settings_set_enable_developer_extras(TRUE)` (devtools), `load_uri`, then
  `gtk_widget_grab_focus`. Tab widgets are tagged with `g_object_set_data(... "tab-id")`
  so `shell_find_tab(id)` can find them.
- Tab **title/URI** are forwarded to the chrome via `//export exportShellTitle/
  exportShellUri` (tabs_shell_export.go) → wails events `shell:title` / `shell:uri`
  (`{tabId, title|uri}`) on `shellCtx` → `tabBar.setPageTitle` in app.js.
- Tab **navigation state** (back/forward availability + loading) is forwarded via
  `//export exportShellNavState` (`notify::estimated-load-progress` + every uri
  change) → wails event `shell:nav-state` `{tabId, canGoBack, canGoForward,
  loading}` → `tabBar.setNavState` enables/disables the chrome tab-bar controls.
  Navigation itself is driven by `ShellBack/ShellForward/ShellReload/ShellStop`
  (→ `shell_nav`, op 17, action 0=back/1=fwd/2=reload/3=stop, `id≤0` targets the
  chrome strip webview).
- Tab **Web Notifications** are forwarded to the desktop via D-Bus (see the
  "Tab notifications → desktop" section below).
- All Wails-bound Go methods go through `shell_request(...)` → `g_idle_add` →
  `shell_req_cb`, so every GTK/WebKit call happens on the GTK main thread. Ops:
  `0=setup, 1=show, 2=destroy, 3=reorder, 4=zoom, 5=chrome height, 6=open settings,
  7=close settings, 8=inspector, 9=open notes, 10=close notes, 16=precreate,
  17=navigation` (each shell_* C function keeps a re-entrance guard
  so the idle-reinvoked code path does not repackage twice), then the terminal ops
  `11=toggle, 12=open/close, 13=destroy, 14=restart, 15=split` (see terminal.go).
  Every tab webview connects `notify::title`, `notify::uri` and
  `notify::estimated-load-progress` at creation (also for precreated webviews).
- **Home strip**: repackaging the WebKitView into `vbox` also REMOVES the roaming
  WebKit home-iframe whitespace/resize quirks on the tab strip (the DA does not
  stretch the strip or the stack).
- The native tab shell replaced the old iframe/panel implementation: each tab is now
  a real browsing context (real cookies/localStorage/HSTS via the shared default
  web context, no CORS limits, inspector per tab).

### terminal.go — per-tab SSH/local terminal (VTE in a GtkPaned split)
Every tab can host a native **VTE terminal** (`vte-2.91`) in a **GtkPaned** split:
- **Architecture**: the `GtkPaned` is created at TAB CREATION (shell_show_tab) with
  the **webview as child1 and NO child2**. The webview is NEVER reparented — moving
  a realized/mapped WebKitWebView into another container corrupts GTK's css/draw
  state (blank page + `gtk_css_node_insert_after` assertions). Opening the terminal
  only packs the termbox as child2; closing it removes child2 and the page fills the
  box again. The terminal pane uses `resize=FALSE` + `term_paned_place` so it stays
  glued to the bottom (vertical split) or right (horizontal) at a fixed height
  (`TERM_TERM_PX 160`) while the page absorbs window resizes.
- **Session state**: `term_session` per tab in `term_sessions` (GHashTable keyed by
  tab id, deliberately leaked on tab destroy — a pending `g_child_watch` may still
  reference the struct). Tracks `pid/running/visible/orient` + host/port/user/auth/
  password/key/dir + the widgets (`bar`, `term`, `termbox`, `split_btn`, `paned`).
- **Spawn**: remote host → `ssh -tt -o StrictHostKeyChecking=accept-new` (optional
  `-p port`, `-i key`, `-l user@host`); password auth uses an `SSH_ASKPASS` helper
  (`data/ssh-askpass.sh`, written by `ensureAskpassHelper()` in Go, echoing the
  password from the `DASH_SSH_PASSWORD` env var — a non-NULL envv REPLACES the child
  env, so the current environment is cloned first). No host → local `${SHELL}` in the
  configured dir. `vte_terminal_spawn_async` + `g_child_watch_add`.
- **Custom splitter drag** (tabs_shell.go, `shell_paned_*`): GtkPaned's default drag
  is an internal `GtkGestureDrag` in TARGET phase that CLAIMS the pointer sequence
  BEFORE the widget's `button-press-event` signal is emitted, so plain button-press
  handlers can never intercept it. We attach our OWN `GtkGestureDrag` in **CAPTURE
  phase** (capture controllers run first): it claims the sequence on separator hit
  (±12px), then DURING the drag only tracks the target position + draws a 2px accent
  guide line (`shell_paned_draw`, connect_after); `gtk_paned_set_position` is applied
  ONCE on `drag-end`. This avoids the webview flash caused by resizing the page on
  every motion (WebKit's accelerated compositing repaints asynchronously).
- **Scrollbar**: VteTerminal is a `GtkScrollable` — without a host
  `GtkScrolledWindow` it buffers scrollback but shows NO scrollbar. The terminal is
  packed inside a `GtkScrolledWindow` (`#term-scroll`) with vertical `GTK_POLICY_AUTOMATIC`,
  `vte_terminal_set_scrollback_lines(10000)`, `scroll_on_output=FALSE`,
  `scroll_on_keystroke=TRUE`; the scrollbar appears only when scrollback overflows.
- **Header bar** (`#term-bar`, styled via the CSS provider in shell_setup): host/user
  label + split-orientation toggle (`⇆`/`⇅`, `term_split_cb` → `term_apply_split` +
  `exportShellTerminalSplit` persists the choice) + Restart + Close.
- **Entry points** (ops 11-15 in shell_req_cb, run on the GTK thread):
  `shell_terminal_toggle/open/close/restart/split/destroy/kill` (kill = SIGHUP the pty
  child before tearing down a tab box).
- **App methods / Go side**: `terminalParams(tab)` resolves SSH params from the
  service `TerminalConfig` (service matched by key OR by URL prefix via
  `serviceKeyForURL`); custom tabs get a local shell; `TerminalToggle/Open/Close/
  Restart/Split` (+ NoContext twins) enqueue ops 11-15. `TerminalSplit` persists
  `terminal.split` ("v"/"h") in config.yaml and re-applies it live.
- **State → chrome**: `exportShellTerminalState`/`exportShellTerminalSplit`
  (terminal_export.go) emit the wails event `shell:terminal-state`
  `{tabId, running, visible}` so the tab bar can reflect terminal state.

### Tab notifications → desktop (Web Notifications API, WebKitGTK 2.52)
Tab pages are real webviews, so they can use the Web Notifications API
(`new Notification(...)`). WebKitGTK 2.52 on this stack is NOT built with libnotify,
so the default `show-notification` handler would silently do nothing. The dashboard
takes over the whole pipeline:

1. **Permission — the KEY gotcha**: WebKitGTK 2.52 does NOT emit the webview
   `permission-request` signal for notification permissions. For any origin NOT in
   the web context's initial permission map, `Notification.requestPermission()`
   silently resolves to `"denied"` and `show-notification` NEVER fires — even with a
   `permission-request` handler connected. The ONLY working mechanism is
   pre-allowing origins via `WebKitWebContext::initialize-notification-permissions`:
   - `tabs_shell.go` hooks the signal `initialize-notification-permissions` on the
     default web context (`shell_notif_permissions_install`, called from `shell_setup`)
     and calls `webkit_web_context_initialize_notification_permissions(ctx, allowed,
     NULL)` with the stored origins (`shell_notif_set_origins`, set from Go).
   - **Do NOT `g_object_unref` the `WebKitSecurityOrigin` list afterwards — the
     objects are owned by the context and unref'ing crashes.** Leak them (they are
     static per-process).
   - The origin list is kept in sync by `App.refreshNotificationOrigins()`: the
     resolved URL of every stored tab + every configured service entrypoint
     (base_url, backoffice_url, frontend_url, base_url+admin_path). Called in `main()`
     BEFORE `wails.Run()` (the map is read when the FIRST web process launches) and
     again in `TabsChanged` so custom tabs added later are covered.
   - Pre-allowed origins make `new Notification()` fire `show-notification` even
     though the JS-side `requestPermission()` may STILL resolve to `"denied"` (a
     WebKitGTK 2.52 quirk — pages that gate on `Notification.permission ===
     'granted'` can still be surprised, but `new Notification()` displays anyway).
2. **`permission-request` handler** (`shell_notif_permission`) allows
   `WebKitNotificationPermissionRequest` and denies everything else (media/geo etc.).
3. **`show-notification`** (`shell_notif_shown`) — registers the notification in a
   bounded registry (`shell_notif_add`, max 32, evicts oldest by closing it), then
   `//export exportShellNotification(id, title, body)` (tabs_shell_export.go) →
   `App.showTabNotification` (main.go):
   - logs it, then on a Go goroutine calls `internal/notify`'s `Notifier.Notify`
     (`org.freedesktop.Notifications` via godbus on the session bus).
   - `notifMap`/`notifRev` maps webkit-id ↔ desktop-id so a tab REPLACING a previous
     notification (`replacesID`) updates the same desktop bubble.
   - When the desktop notification is dismissed, `Notifier.OnClosed` → `closeWebNotification`
     (notifications_bridge.go) → `shell_notif_close_impl` → `webkit_notification_close`
     on the GTK main thread (via `g_idle_add`) so the page's `onclose` fires.
   - `g_signal_stop_emission_by_name(wv, "show-notification")` suppresses the default
     libnotify handler.
   - `WebKitNotification` objects are `g_object_ref`'d so they survive until dismissed.
   `App.Startup` creates the `Notifier` (logs `Desktop notification support enabled`
   or drops tab notifications when the daemon is unavailable); `App.Shutdown` closes it.

### shell_app.go — Wails-bound shell controller
Every `App` method below also has a `...NoContext` twin (frontend calls those):
- `ShellShowTab(id)` / `ShellDestroyTab(id)` / `ShellReorder(ids)` / `ShellZoom(id, level)`
- `ShellSetChromeHeight(px)` (clamped 40–800; the high bound lets the frontend
  grow the strip to ~480px while a DOM popup — context menu or inspector dropdown —
  is open; the tab pages underneath stay visible) — repackages the chrome webview
  (`size_request`); the tab-stack keeps the rest of the window
- `OpenSettings()` / `CloseSettings()` — opens/closes the **native settings window**
  (a second WebKitWebView with a DEDICATED user content manager, loading
  `wails://wails/settings.html`; `delete-event` hides it — see tabs_shell.go:
  shell_open_settings)
- `OpenNotes(tabID)` / `CloseNotes()` — opens/closes the **dedicated notes window**
  for a tab (same architecture as the settings window: own webview + ucm + bridge,
  load `wails://wails/notes.html?tab=<id>`; NOT modal), so the chrome strip is never
  resized and the tab pages never shift while editing notes
- `TabsChanged()` — emits `tabs:changed` (for the settings/notes window / chrome to
  reload tabs) and re-syncs both the tray menu and the notification origins
- `resolveTabURL(tab)` — builds the real admin URL for builtin tabs
  (service key → `backoffice_url > base_url+admin_path > base_url`, legacy fallbacks)
- `tabZoomOf(tab)` — reads `settings["zoom"]` (0.5–2.5, default 1)
- `refreshNotificationOrigins()` — rebuilds the pre-allowed Web Notification origins
  (see "Tab notifications → desktop")

### app_config.go — service config editing
Wails-bound service configuration editor used by the Settings window:
- `GetAppConfig()` — returns the whole config as a generic map (services, proxy, ui)
- `SaveAppConfig(patch)` — merges the patch into `config.yaml`: edits/validates
  service url/auth/terminal fields (`app_config.go`), then reconfigures the live
  `ServiceManager` at runtime (`Reconfigure` keeps the health/status clients)
- Helpers `serviceConfigMap`/`asMapSafe`/`asStringSafe`/... serialize the config
  into the JSON-shaped maps the frontend expects.

### Wails method binding pattern (IMPORTANT)
Every frontend-callable method exists in TWO forms on `App`:
- `Method(ctx context.Context, args...)` — Wails' usual signature
- `MethodNoContext(args...)` — wrapper calling the ctx version with `context.Background()`

The frontend's `api.js` calls the `...NoContext` variants via the generated
`wailsjs/go/main/App.js` exports.

### internal/api/dashboard.go
`DashboardAPI` (service health + statuses, delegating to ServiceManager)
and `TabAPI` (tab CRUD):
- `TabAPI.ListTabs` — if tabs.json is empty, seeds the 3 defaults (alpha/beta/gamma)
- `TabAPI.UpdateTab(id, config)` — updates label/url/icon from a `map[string]interface{}`
- `TabAPI.UpdateTabSettings(id, settings)` — replaces the per-tab display settings bag
- `TabAPI.AddNote(id, title, content)` / `TabAPI.UpdateNote(id, noteID, title, content)` /
  `TabAPI.DeleteNote(id, noteID)` — multi-note CRUD on a tab (each returns the updated
  `Tab` with its `Notes []Note` list; `tab.Note{ID,Title,Content,CreatedAt,UpdatedAt}`,
  persisted in `data/tabs.json`; `Tab`/`TabInfo` expose `notes`)
- `TabAPI.ReorderTabs(ids []int)` — reorders by id list (added for drag&drop)
- API response types (`TabInfo`, `ServiceStatus`) all here.
  `TabInfo.Settings` carries the per-tab display settings (`{zoom: float}`).
- NOTE: `AddTab`/`RemoveTab` are NOT on TabAPI; they're methods directly on `App`
  (in main.go) operating on `a.tabManager`.

### inspector.go — WebKitGTK page inspector for TAB pages (defunct; per-tab now)
The old standalone *inspection webview* approach (a second window loading the tab URL)
was REMOVED: each tab already is a real WebKitWebView. `inspector.go` now only exposes
thin Wails methods that turn on the extension via the native tab shell (see tabs_shell.go):
- `InspectorAvailable() → true` (guarded by the `devtools` tag via `//go:build`)
- `InspectorOpen(ctx, mode, tabID)` with `mode ∈ bottom|float|right|left|close`
  → `shellPostInspector(8, mode, tabID)`; `tabID=0` targets the chrome strip
- `InspectorClose(ctx, tabID)` → mode `close`
- Layouts (implemented in C, shell_inspector): `bottom` = `webkit_web_inspector_attach`;
  `right`/`left` = `detach` + glue beside `shell_main_win` via `configure-event`;
  `float` = detached, freely movable.
The main dashboard webview + every tab webview enable developer extras at creation.

### internal/services/{clients,manager}.go
`HTTPClient` wraps an `http.Client`; `addAuth` applies config `Auth.Type`:
- `basic` → username/password from env vars (BETA_USER/PASS)
- `bearer` → token from env (GAMMA_TOKEN)
`ServiceManager` owns one client per configured service; `CheckAllHealth` probes each
(5s timeout) and returns the per-service statuses (used by the header status pills).

### internal/tab/manager.go
`TabManager` persists `[]Tab{ID,Title,URL,Icon,Notes}` to `data/tabs.json`
(`<exedir>/data/tabs.json`; portable). `Tab.Notes` is a `[]Note` list
(`Note{ID,Title,Content,CreatedAt,UpdatedAt}`). `Add` assigns
`nextID`. `Update` keeps old values when empty. `AddNote`/`UpdateNote`/`DeleteNote`
persist the per-tab note list atomically (ids unique per tab; timestamps set in Go).
Legacy tabs.json files storing `notes` as a plain string are migrated at load into a
single-note list. `Reorder(ids)` validates the id set.

### internal/tray
D-Bus StatusNotifierItem on the session bus (no appindicator CGO dependency).
Implements `org.kde.StatusNotifierItem` + `com.canonical.dbusmenu`. Tray icon is the
themed `"dashboard"` icon (installed by build/install-desktop.sh). Shows/hides/focuses
the window; `Quit` closes. `OpenExternal(url)` in main.go uses `xdg-open`.
- **Menu**: `GetLayout` MUST reply as `(u(ia{sv}av))` — the root layout struct
  returned DIRECTLY (a `dbus.Variant` wrapper breaks KF/Qt importers: "Could not
  find DBusMenu interface"), root carries `children-display: submenu`, children
  are variants of `(ia{sv}av)`; `menu_test.go` locks the signature.
- **Tab activation from tray**: `App.ShowTabFromTray(id)` (handler `ShowTab` on
  the `tray.Handler`) shows the window, calls `ShellShowTab(id)` and emits the
  wails event `shell:tab-activated` `{tabId}` so the chrome tab bar highlights
  the newly active tab (app.js `switchTab`). `SetTabs` syncs the tab list into
  the menu (called on startup + `TabsChanged`) and emits `LayoutUpdated` with a
  bumped revision.

## Frontend Architecture

### Entries: index (chrome) + settings.html (settings window)
`frontend/src/main.js` imports `styles/main.css` (must be imported here; tree-shaken
otherwise) then mounts `createApp()` from app.js. `vite.config.js` injects an inline
bootstrap into the generated `dist/index.html` that awaits
`window.go.main.App.CreateApp()` before the bundle loads (so `window.go` exists);
the plugin is scoped to `index.html` only — settings.html runs WITHOUT the Wails
runtime (custom bridge). Any vite.config change needs a full rebuild.

### app.js — UI chrome
`createApp()`:
1. `getSystemTheme()` → sets `document.documentElement.dataset.theme`
2. `mountChrome()` builds header/brand/status pills/settings/win-controls + `tabBar` —
   NO `.dashboard-content`, no panels; tab content lives in native webviews. (The
   `location.hash === '#settings'` branch is a legacy fallback; the native settings
   window loads its own page, see below.)
3. `mountChrome()` builds header/brand/status pills/settings/win-controls + `tabBar`
   with callbacks: onTabChange→switchTab (→ `api.shellShowTab`), onAddTab→openSettings,
   onReorder→`api.reorderTabs`+`api.shellReorder`+reload, onSetDefault,
   onOpenExternal→`api.openExternal`, onRenameTab, onDuplicateTab,
   onZoom/onResetZoom → `api.shellZoom` (native zoom) + persist settings,
   onNav→`api.shellNav(id, back|forward|reload|stop)`.
4. **Chrome strip height**: `measureStripHeight()` = `tabBar.getBoundingClientRect().bottom + 2`
   (min 60, fallback 104) → `api.shellSetChromeHeight(px)`; debounced (30ms) + on window
   resize. **DOM popups** (context menu, inspector dropdown) that would be clipped by the
   thin strip use `expandStrip()` (height 480) / `collapseStrip()` (`stripExpanded` guard).
5. **Native tab lifecycle** (keep-alive): `switchTab(tab)` calls `api.shellShowTab(tab.id)`
   and tracks the active id. `loadTabs()` removes webviews of deleted tabs via
   `knownTabIds` (per-process) → `api.shellDestroyTab(id)`; respects
   `dashboardStore.getDefaultTab()` (localStorage `dashboard_default_tab`, default 'alpha').
6. Events from Go: `runtime.EventsOn('shell:title')` → `tabBar.setPageTitle(id, title)`;
   `EventsOn('shell:nav-state')` → `tabBar.setNavState(id, {...})` (back/forward/reload
   controls of the ACTIVE tab); `EventsOn('shell:terminal-state')` → `tabBar.setTerminalState`
   (marks the per-tab terminal button as open);
   `EventsOn('tabs:changed')` (emitted by the settings window after edits) → `loadTabs({preserveActive:true})`.
7. `loadServiceStatus()` every 30s + on visibilitychange → `tabBar.setStatuses()`.
8. Window controls: min/max/close/to-maximise-state via `api.window*` (wailsjs runtime).
9. Keyboard nav: **Ctrl+Tab**/Ctrl+Shift+Tab cycle tabs, **Ctrl+T** open settings,
   **Ctrl+± / Ctrl+0** zoom the active tab grid (via shellZoom), **Ctrl+R** reload it.

**Tab bar page controls**: a `.tab-nav-controls` cluster (back/forward, a
reload button that swaps to **stop** while the active page is loading, and a
zoom − / % / + group — the % label resets to 100%) sits at the LEFT of the tab
bar and always acts on the ACTIVE tab. Back/forward are disabled by the
`shell:nav-state` flags; the % label is synced from the per-tab `settings.zoom`.
10. **Per-tab display settings** — `settings` map persisted via `api.updateTabSettings`.
    `zoom` (0.5–2.5) is applied NATIVELY via `api.shellZoom(id, level)` at show time +
    live; the SettingsModal zoom slider calls the same path.
11. **Persistent per-tab notes** — the editor is a DEDICATED floating window (like the
    Settings window), NOT a DOM card in the chrome: `openNotes(tab)` →
    `api.openNotes(tab.id)` → `App.OpenNotes` → `shell_open_notes` (tabs_shell.go)
    opens `wails://wails/notes.html?tab=<id>`. A tab owns a LIST of notes; every save
    goes through the notes bridge (`saveNote`/`deleteNote` → `TabAPI.AddNote/
    UpdateNote/DeleteNote`, persisted in `data/tabs.json`), then a `TabsChanged()` so
    the chrome refreshes the per-tab note indicator. Because the editor lives in its
    own window the chrome strip is never expanded and the tab pages never shift while
    editing notes. Open from the tab context menu **Note**, the per-tab `tab-note-btn`,
    or the **Gestisci** button of the settings window's per-tab panel (which shows the
    current note count instead of a single-note textarea).
11. **Inspector dropdown** (header, right of settings) — `api.inspectorOpen(mode, activeTabId)`
    with `bottom|right|left|float` and `api.inspectorClose()`; hidden when
    `api.inspectorAvailable()` is false (non-devtools build).

### Settings window / dedicated page + custom bridge
`OpenSettings()` (Go) opens a second WebKitWebView with its OWN user content manager
(NOT the chrome's — see the gotcha below) and loads `wails://wails/settings.html`.
That page is a **separate vite entry** (`frontend/settings.html` →
`frontend/src/settings/main.js`) with NO Wails runtime/bindings; it reuses
`SettingsModal` and reaches Go through a small custom IPC:
- JS posts `JSON.stringify({id, method, args})` via
  `window.webkit.messageHandlers.dashboardSettings.postMessage` (handler registered on
  the settings webview's dedicated ucm in `shell_open_settings`).
- The C signal `script-message-received::dashboardSettings` → `//export
  exportSettingsMessage` → `handleSettingsMessage` (settings_bridge.go) dispatches on
  `App` (package-level `activeApp`, set in main()).
- Replies come back via `shell_settings_eval` (g_idle → `webkit_web_view_evaluate_javascript`)
  → `window.__dashReply({id, ok, result|error})`.
Bridge methods: getTabs/getTheme/getSystemTheme/setTheme/saveTabConfig/removeTab/
updateTab/updateTabSettings/openNotes/reorderTabs/tabsChanged/closeSettings/shellZoom/resize.
- **Frameless**: the window is `gtk_window_set_decorated(FALSE)` like the main app.
  Dragging is bound to the CARD HEADER, not a strip above it: `settings_drag_press`
  (button-press-event on the webview) asks the *page* via
  `webkit_web_view_evaluate_javascript` + `elementFromPoint` whether the press is
  over the `.modal-header` (walking ancestors; `BUTTON`/`.btn` elements are
  excluded so the close button stays clickable); only then starts a GTK
  move-drag (`gtk_window_begin_move_drag`).
- **Close** closes the whole window: `SettingsModal.close()` → `onClose` →
  `closeSettings` → `shell_close_settings` destroys the window+ucm (recreated on
  next open); a `delete-event` instead only hides it.
- **Not modal, stays on top**: the window is `transient_for` + `keep_above` —
  deliberately NOT `gtk_window_set_modal` (a modal grab blocks dragging the main
  window while settings is open). Both windows stay interactive and the card
  floats above the chrome.
- **Transparent floating card** (the window is per-pixel transparent, so there
  is NO window background around the card — the desktop/main-window shows through).
  The C shim puts the settings window on an RGBA visual (compositor + rgba visual
  required), marks it `app_paintable` and clears its background in a `draw`
  handler (`settings_draw_clear`); the settings webview background is cleared via
  `webkit_web_view_set_background_color`. The page only paints the `.modal` card
  (`body.settings-mode.dash-transparent { background: transparent }`). The C shim
  sets `g_settings_transparent` and loads the page with `?t=1` so main.js toggles
  the class; if the compositor/rgba visual is unavailable the window STAYS opaque
  and the page keeps its painted `--bg-app` background (the "card on a panel"
  fallback), so a black void is never exposed. The historical black apron was the
  opaque window's theme background showing through an un-window-cleared webview.
- **No black border around the card**: the card's CSS
  `box-shadow: 0 16px 44px rgba(0,0,0,.5)` renders as a hard BLACK frame around
  the floating card (over a light desktop it reads as a "black border"), and KWin
  also draws a shadow around the window's *rectangle* on ARGB windows. Fixed two
  ways: `body.settings-mode .modal { box-shadow: none }` (settings mode only; the
  card keeps its crisp 1px border) and `gtk_window_set_type_hint(
  GDK_WINDOW_TYPE_HINT_UTILITY)` in `settings_enable_transparency` — the standard
  recipe for frameless transparent popups: KWin drops the decorative shadow while
  the window stays focusable and (as a transient) above its parent. Verified on
  the live desktop: the white desktop shows through the ~10px transparent margin
  with only a faint lightweight shadow.
- **Content auto-resize**: the page measures the rendered `.modal`
  (`scheduleFit()` in settings/main.js — 120ms debounce + double-rAF, deduped by
  size) and reports it to Go via the bridge method `resize` →
  `settingsResize` → `shell_settings_resize` (`gtk_window_resize` on the GTK
thread). A **C-side fallback** (`settings_fit_timeout` in tabs_shell.go) probes
   the same measurement a few times and resizes the window even if the running
   bundle is too old to have `scheduleFit` (works with any bundle that renders
   `.modal`). The probe is armed from the settings bridge on the first page
   message (`getTabs` → `shell_settings_fit_start`), NOT from a WebKit
   `load-finished` connection — that signal can be emitted on a destroyed
   webview during teardown and spam `GLib-GObject-CRITICAL: signal
   'load-finished' is invalid for instance`. `body.settings-mode .modal` has a
   FIXED width (820px, no `vw`) so the measurement is stable; `.modal-body`
   scrolls beyond `max-height: 780px`. The overlay keeps a small 10px margin
   and the card is flush to the top (drag region = card header).
- **No stale settings bundle across rebuilds**: `main()` calls
  `clearWebviewResourceCache()` before `wails.Run()` — it removes
  `<exedir>/data/cache/Dashboard` (WebKit's on-disk HTTP resource cache; the data
  dir is redirected by `main()`, so *every* reload of `wails://wails/settings.html`
  serves the freshly embedded `dist/`). Without this, an old bundle could be
  served after a rebuild (missing `scheduleFit` → oversized/sloppy window). The
  resource cache is per-run state; websites data (`data/webview`) is untouched.
- Theme changes in the settings window are broadcast to the chrome via the wails
  event `shell:theme` (emitted by `SetTheme`; chrome listens in app.js).

### Notes window (per-tab notes editor) — dedicated floating window
The per-tab notes editor is a SECOND floating window reusing the settings-window
architecture (own webview + ucm + custom bridge, NO Wails IPC) so the chrome strip
is never touched. **Multi-note**: a tab owns a list of notes
(`Tab.Notes []Note`, persisted in `data/tabs.json`; each note has its own
`id/title/content/created_at/updated_at`). The window shows a two-pane card: a
left sidebar lists the tab's notes (title + snippet + date, delete on hover) and
a right pane edits the selected note (title + content). Legacy single-string
`notes` in tabs.json are migrated into a one-element list at load.
- `App.OpenNotes(tabID)` → `shellOpenNotes` (tabs_shell.go op 9) →
  `shell_open_notes` creates a frameless, `transient_for` + `keep_above` (NOT
  modal) window with a dedicated ucm registering the `dashboardNotes` message
  handler and loads `wails://wails/notes.html?tab=<id>` (a separate vite entry:
  `frontend/notes.html` → `frontend/src/notes/main.js`). `App.CloseNotes()` → op 10
  → `shell_close_notes` destroys the window+ucm (recreated on next open).
- The page talks to Go via `window.webkit.messageHandlers.dashboardNotes.postMessage`
  → `exportNotesMessage` → `handleNotesMessage` (notes_bridge.go) dispatching on
  `App`; replies come back via `shell_notes_eval` → `window.__dashReply`.
  Bridge methods: getTab/saveNote/deleteNote/getTheme/getSystemTheme/closeNotes/resize.
  `getTab` returns the tab with its `notes` list + `note_count`; `saveNote` upserts
  (noteId ≤ 0 creates a new note via `TabAPI.AddNote`, otherwise `TabAPI.UpdateNote`);
  `deleteNote` removes it. All three note ops end with `TabsChanged()` so the chrome
  refreshes the per-tab note indicator immediately. The bridge reply returns the
  updated tab so the page re-syncs ids after a create.
- Same window dressing as settings: transparent when the compositor allows
  (`body.notes-mode.dash-transparent`), drag on the card header
  (`notes_drag_press`, `.notes-header`), content-fit (`scheduleFit` in
  notes/main.js + the `notes_fit_timeout` C fallback armed on the first
  `getTab` message), 10px margin around the 720px `.notes-card`. The page
  centres the card horizontally and keeps it flush to the top.
- Because the editor is a real window, opening it NEVER resizes the chrome strip
  or the tab-stack: the tab pages below stay exactly where they were.

### stores/dashboard.js
Tiny singleton: `tabs[]`, `lastStatuses[]`, `listeners`, `setTabs/subscribe/notify`,
`getDefaultTab/setDefaultTab` (localStorage). Used by app.js for default tab + statuses;
tab content itself is native webviews handled by the Go shell.

### TabBar/TabBar.js (extended)
- Renders pill tabs (icon + label) with a per-tab status dot; tabs can only be
  removed from the Settings modal (no close X / middle-click).
- **Drag & drop** reordering: HTML5 dragstart/dragover/drop on `.tab-bar-item`;
  on drop it re-slices `this.tabs` and calls `onReorder(ids)`.
- **Context menu** (right click): Rename (inline input), Set as default,
  Open in browser, Duplicate, **Note** (opens the dedicated per-tab notes window),
  Zoom −/%/+ (keep-open), Reset zoom (toolbar removed with the iframe era).
  `setDefaultTabId()` marks the default.
- **Page navigation controls** (`.tab-nav-controls`, LEFT of the tab bar): back,
  forward, reload (becomes **stop** while loading), zoom −/%/+. All act on the
  ACTIVE tab only. `setNavState(tabId, {canGoBack, canGoForward, loading})`
  (from the `shell:nav-state` event) drives the disabled/stop states;
  `refreshNavControls()` syncs everything on tab switch/zoom.
- **Per-tab action buttons** (`tab-terminal-btn` + `tab-note-btn`): ghost buttons
  (24px hit area) that always act on their own tab. The terminal button is marked
  `.open` by `setTerminalState(tabId, {running, visible})` (from the
  `shell:terminal-state` event) while the terminal is shown; the note button is
  warning-coloured while the tab has notes.
- **Persistent per-tab notes**: every pill has a small `tab-note-btn` (icon) that
  opens the dedicated notes window for that tab; it turns warning-coloured when the
  tab has notes (`tab.notes` is a non-empty `[]` list of note objects).
  `refreshNotes()` updates the indicator in place after a save.
- `setPageTitle(tabId, title)` shows the live page title transiently (ignored while
  an inline rename is active). `onPopupChange(open)` is called when the context menu
  opens/closes so app.js can expand/collapse the strip.
- Tooltips include the URL for non-builtin tabs; labels are `escapeHtml`-encoded.
- `setTabs(tabs, {persist})`, `setActive(id)`, `setStatuses(statuses)`.`

## Theming System (IMPORTANT for styling)
`frontend/src/styles/main.css` defines CSS variables for dark (default) and a
`[data-theme="light"]` override block (KDE Plasma light detection in detectSystemTheme).
- Key vars: `--bg-primary/secondary/tertiary/hover`, `--fg-*`, `--accent`, `--border*`,
  `--success/warning/danger`, `--chrome-bg`, `--radius*`, `--shadow*`, `--font-*`.
- Any new component MUST use these variables, not hardcoded colors.
- Layout: header (drag region via `--wails-draggable: drag`, interactive elts set
  `data-win="no-drag"`), pill tab bar, grid/card content, modals.
- Special regions already handled: drag region, `.win-controls`, status pills, context
  menu (`--bg-secondary`). (Legacy `.panel` / `.url-panel` rules still exist in main.css
  but are inert — no panel/iframe DOM is rendered anymore.)
- Scrollbars are custom (`::-webkit-scrollbar`). Light theme has targeted overrides.

## Frameless Window (KDE Plasma)
- `Frameless: true` + CSS drag regions. On KDE Wayland KWin ignores undecorated windows,
  so `main()` forces `GDK_BACKEND=x11` (GTK reads it at init, must be set before wails.Run).
- Custom min/max/close in JS call `runtime.WindowMinimise/ToggleMaximise/Quit`.

## Config & Environment
### config.yaml (<exedir>/data/config.yaml; portable data dir)
All config + runtime data live in a single `data` folder next to the executable
(`<exedir>/data/`), resolved by `internal/paths`. On first run it is auto-created
and migrated from the legacy `~/.config/Dashboard`, `~/.local/share/Dashboard`,
`~/.cache/Dashboard` (config.yaml is also reused from next to the exe/CWD if legacy
has none). `main()` redirects `XDG_DATA_HOME`/`XDG_CACHE_HOME` to `data/webview`
and `data/cache` so WebKitGTK website data stays portable too.
```yaml
app: {name: Dashboard, version: 1.0.0, debug: true}
services:               # per-service: base_url, backoffice_url, api_prefix,
  alpha: {...}         #   auth: {type: none|basic|bearer, *_env}, endpoints
beta/gamma: ...
proxy: {enabled, allowed_hosts, timeout_seconds, max_body_size_mb}  # legacy, still saved but inert
ui: {theme: dark, default_tab: alpha, webview_gpu_policy: always, tabs: [{id,label,icon,enabled}]}
```
### Auth env vars (never commit real values)
```bash
export BETA_USER=xxx
export BETA_PASS=xxx
export GAMMA_TOKEN=xxx
```

## Known Gotchas
1. **Build tags mandatory**: `wails build -s -tags "webkit2_41 devtools"` for WebKitGTK 2.52 on
   openSUSE Tumbleweed, or you get segfaults/blank window. The `devtools` tag enables WebKit
   developer extras — WITHOUT it the page inspector (InspectorOpen/Close, Ctrl+Shift+F12)
   silently does nothing.
2. **Frontend console invisible to Go logs** — WebKitGTK doesn't pipe console.error to
   stdout. JS troubleshooting uses `dashboard.log` (Printf calls are Go-side) + the
   built-in inspector (header dropdown / `InspectorOpen`; `--inspect=9222` does NOT work
   on Linux WebKitGTK) or the `/dev` dev-log path if exposed via proxy. There is no `/dev`
   page currently.
3. **Do NOT hand-edit `frontend/wailsjs/`** — regenerated on every `wails build` from
   App's exported methods.
4. **vite.config.js changes need full rebuild** of dist + wails.
5. **go.mod says `go 1.25.0`**, installed toolchain 1.26.5 — keep `go` directive ≤ toolchain.
6. **`internal/paths` must not self-call inside `migrate()`**: `migrate()` runs inside
   `DataDir()`'s `sync.Once`; calling `File()`/`DataDir()` from `migrate()` deadlocks
   the bindings generation (goroutine dump shows `sync.Once.doSlow`). Use `dir`
   directly. Symptom: `wails build` hangs forever at "Generating bindings" with the
   `/tmp/wailsbindings` process stuck in `futex_do_wait`.
7. **WebView cookie persistence**: WebKitGTK's default cookie manager is IN-MEMORY
   (localStorage/HSTS persist via WebsiteDataManager, but cookies are lost on exit).
   Two mechanisms are in place:
   - `persistent_cookies.go`: `webkit_cookie_manager_set_persistent_storage` to
     SQLite at `<exedir>/data/cookies.sqlite`, called from `main()` BEFORE
     `wails.Run()` (must be before the first webview exists). Any new linux cgo
     file needs the `webkit2_41` build tag + pkg-config lines (see Wails' gtk.go).
   - **Session-cookie snapshot/restore** (`WebKitCookieManager` only stores
     NON-session cookies to SQLite, so login session cookies would still be lost):
       * `App.Startup` schedules `scheduleCookieSnapshots()` — a GLib
         `g_timeout_add_seconds` timer (GTK main thread) that every 20s pulls
         `webkit_cookie_manager_get_cookies` for each tab + service host URI,
         accumulating/deduping (domain,path,name) into
         `<exedir>/data/webview_cookies.json` (written once per round).
       * `App.OnDomReady` (fired on the GTK main thread via the `load-finished`
         signal, BEFORE the tab webviews navigate) calls `restoreWebviewCookies()`
         which re-adds those cookies via `webkit_cookie_manager_add_cookie`
         (`soup_cookie_new(...,-1)` = session).
       * `App.Shutdown` runs `snapshotAllWebviewCookies()` to catch last-minute
changes.
     KNOWN LIMITATION: the `httponly` flag is dropped on the add→get roundtrip by
     WebKit (server still receives the cookie — httponly only blocks JS access).
   (The legacy app-level cookie jar plus proxy/statuses were REMOVED — all tab
   cookies live in the WebKit cookie manager above.)
 8. **`ServiceManager` data**: live calls hit real endpoints; if a service is down,
    header pills go offline (no panels any more).
9. **DOM popups need strip expansion**: the chrome strip is ~104px, so any DOM popup
   (tab context menu, inspector dropdown) is clipped. app.js expands
   the strip to ~480px (`expandStrip` — `EXPANDED_STRIP`) while the popup is open and
   collapses it on close (`stripExpanded` guard; `syncChromeHeight` is skipped while
   expanded). The height is deliberately MODERATE so the tab pages (GtkStack below
   the strip) stay visible; a taller strip (e.g. 2400) would collapse the pages out
   of view entirely. NOTE: the notes editor is NOT a DOM popup any more — it lives
   in its own floating window (see "Notes window" below), so the strip is never
   expanded for it and the tab pages never shift.
10. **JS debug → Go log**: `window.runtime.LogPrint`/`LogDebug` do NOT reach
    `dashboard.log`. To debug DOM from JS, use `fmt.Printf`/`log.Printf` in a Go method
    (e.g. temporarily add a `DebugLog` bound method to App), regenerate bindings, and
    call it — `log.SetOutput(f)` sends it to the log file.
11. **A second webview cannot use Wails IPC** — Wails delivers Go→JS replies only to
    the MAIN webview (`Frontend.ExecJS` → `webkit_web_view_run_javascript(w.webview)`).
    A secondary WebKitWebView sharing the chrome's user content manager will hang on
    `await window.go.main.App.CreateApp()` (blank window). Any extra window MUST use its
    own webview + ucm and a custom `script-message-received` bridge (see the settings
    window). Also `webkit_web_view_run_javascript` is deprecated on 2.40+ — use
    `webkit_web_view_evaluate_javascript`.
12. **Web Notifications permission in WebKitGTK 2.52** — `permission-request` is NEVER
    emitted for notification permissions; unlisted origins resolve `requestPermission()`
    to `"denied"` and `show-notification` never fires. Pre-allow origins via
    `WebKitWebContext::initialize-notification-permissions` (see the "Tab notifications
    → desktop" section). Even pre-allowed origins may STILL report `"denied"` to JS —
    but `new Notification()` then displays fine. Do NOT `g_object_unref` the
    `WebKitSecurityOrigin` GList passed to `webkit_web_context_initialize_notification_permissions`
    (crash); leak the origins (static per-process). Also: the webview `permission-request`
    handler is still worth keeping (allows `WebKitNotificationPermissionRequest`, denies
    the rest) — it covers other permission types and future WebKit versions.
13. **cgo: `//export` files have per-file preamble rules** — Go code in file X can only
    call `C.foo` if `foo` is in X's OWN preamble (declarations) or is a `//export`; the
    amalgamated preamble is NOT used for type resolution. Symptoms: `could not determine
    what C.foo refers to` for symbols that ARE defined in another file's preamble (e.g.
    `C.free` without `#include <stdlib.h>` in THAT file). And a preamble declaration that
    mismatches a definition in another preamble (e.g. `char**` vs `const char**`) fails the
    whole amalgamated compile — all files then report unresolved C refs. Keep C functions
    called from Go in the SAME file that defines them (see `shellSetNotificationOrigins`
    living in tabs_shell.go, not tabs_shell_export.go), and `static` C helpers used before
    their definition need a forward declaration.
14. **Flaky Wails/WebKit startup crash (SIGABRT in `SetupWebview`)** — intermittent
    "signal arrived during cgo execution" abort during `wails.Run`/`NewWindow`, BEFORE any
    app code runs; not caused by app changes (a crash dump lands on stderr, and the app
    log stops right after "WebKit cookie storage"). Just relaunch; rapid kill+restart
    cycles make it more likely. It is unrelated to the notification code.
15. **C debug prints** — temporary `g_printerr(...)` in the C shim lands on stderr
    (captured when launching with `>log 2>&1`), bypassing the Go `dashboard.log`
    entirely — the fastest way to see which WebKit signals actually fire. Remove them
    before committing.
16. **GtkPaned drag is a GtkGestureDrag, not a button-press handler** — GTK3 drives
    the paned separator drag with an internal `GtkGestureDrag` in TARGET phase that
    claims the pointer sequence before `button-press-event` is delivered. A plain
    `g_signal_connect(paned, "button-press-event", ...)` handler NEVER fires for
    separator presses. To intercept, add your own `GtkGestureDrag` in **CAPTURE
    phase** (`gtk_event_controller_set_propagation_phase(..., GTK_PHASE_CAPTURE)`) —
    capture controllers run first, so it can claim the sequence and the paned's own
    gesture gets denied. Custom drags must also apply `gtk_paned_set_position` ONCE on
    release (live per-motion resize of a WebKitWebView flashes: async accelerated
    compositing repaints).
17. **VTE needs a GtkScrolledWindow for a scrollbar** — VteTerminal is a `GtkScrollable`:
    it buffers scrollback internally but shows NO scrollbar unless hosted inside a
    `GtkScrolledWindow`. Pack it in one with vertical `GTK_POLICY_AUTOMATIC` and set
    `vte_terminal_set_scrollback_lines(> 0)` (positive) or the buffer stays at the
    default 512 lines. `scroll_on_output=FALSE` avoids jumping to the bottom on every
    line of output; `scroll_on_keystroke=TRUE` returns to the bottom when typing.
18. **DO NOT reuse the `.loading` class for small chrome widgets** — legacy styles
    (main.css, "Loading / empty / error" block) give `.loading` `min-height:200px`,
    `padding:3rem` and a 34px `::before` spinner with `animation: spin ... infinite`.
    Hooking it onto the tab-bar reload button (old code) made the whole chrome strip
    ~230px tall and showed a huge endless spinner while a tab loaded. Use a scoped
    class (e.g. `nav-loading`) + a small SVG spin rule instead.

## Debugging Commands
```bash
cd /path/to/Dashboard

# Build frontend
cd frontend && npm run build

# Regenerate bindings + build (embed dist). `-s` skips the wails-managed frontend build.
cd .. && wails build -s -tags "webkit2_41 devtools"

# or the one-shot script
./build.sh

# Run
./build/bin/Dashboard
tail -f build/bin/data/dashboard.log

# Wails bindings only
wails generate module -tags webkit2_41

# Config/tab/cookie files (portable data dir next to the exe)
ls -la build/bin/data/
# WebView data (localStorage/HSTS/WebsiteData) + persistent cookies sqlite
ls -la build/bin/data/webview build/bin/data/cache
ls -la build/bin/data/cookies.sqlite
# WebView session-cookie snapshot (snapshots/restores login session cookies)
cat build/bin/data/webview_cookies.json

# --- Debugging the tab-notification pipeline ---------------------------------
# 1) Serve a test page that calls `new Notification(...)` on a local origin:
#    python3 -m http.server 8123   (a page with document.title staging helps:
#    set document.title = "HAS_NOTIFICATION_API" / "PERM="+p / "NOTIFY_SHOWN"
#    at each step — the shell forwards title changes as shell:title events).
# 2) Add a temporary tab in the Settings window pointing at the test origin,
#    then watch the Go log for `Tab notification: id=...` (main.go).
# 3) Watch the actual D-Bus call go out on the session bus:
#    dbus-monitor --session "interface='org.freedesktop.Notifications',type='method_call'"
#    → the `Notify` member shows summary/body/app_name.
# 4) To see which WebKit signals fire, add temporary g_printerr(...) to the C
#    handlers (permission-request / show-notification) — stderr, see gotcha 15.
# 5) Removing build/bin/data/webview resets per-origin WebKit decisions
#    (notification permission included) for a clean slate.
```

## Build & Run Flow (reliable order)
1. Edit Go in `main.go`/`tabs_shell.go`/`shell_app.go`/`inspector.go`/`internal/...`
2. `wails generate module -tags webkit2_41` (only when exported methods/signatures change)
3. `cd frontend && npm run build` (only when frontend changes)
4. `wails build -s -tags "webkit2_41 devtools"`
5. `./build/bin/Dashboard`

## Environment
- OS: openSUSE Tumbleweed (Linux), KDE/Plasma, Wayland session
- WebKitGTK: 2.52.5 (`webkit2gtk-4.1`, requires `webkit2_41` build tag)
- Go: 1.26.5 (toolchain), go.mod requires 1.25.0
- Wails: v2.14.0
- Node/Vite: 5.4.21