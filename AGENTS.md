# AGENTS.md - Dashboard Development Guide

A Go/Wails v2 desktop dashboard for monitoring three services:
- **NeuroNet** - Neural network AI at `http://localhost:8000/admin`
- **Minecraft Network** - Admin panel at `http://51.75.77.248:9800`
- **SlotBuilder** - Backoffice at `https://backoffice.7casinogames.com`, frontend at `https://7casinogames.com`

The app **works**: builds, runs, renders the UI, loads tabs from Go, refreshes panel data, tray/frameless/theme work. This file documents the REAL project structure (verified by inspection), not a plan.

## Project Structure
```
/home/alfio/Projects/Dashboard/
├── main.go                # EVERYTHING app-level: App struct, all Wails-bound methods, wails.Run()
├── inspector.go           # cgo: WebKitGTK page inspector (dock bottom / right / left / floating window)
├── persistent_cookies.go  # cgo: WebKitGTK cookie manager → SQLite persistent storage (webkit2_41)
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
│   ├── src/
│   │   ├── main.js        # Imports main.css + mounts App on DOMContentLoaded
│   │   ├── app.js         # Renders chrome: header, tab bar, panels, settings, keyboard nav
│   │   ├── components/
│   │   │   ├── TabBar/TabBar.js             # Tab bar: active, close, drag&drop order, context menu, rename
│   │   │   ├── SettingsModal/SettingsModal.js # Modal to add/edit/remove tabs
│   │   │   └── *Panel/                      # NeuroNetPanel, MinecraftPanel, SlotBuilderPanel
│   │   │       ├── *Panel.js                # Lifecycle: render/mount/unmount/refresh
│   │   │       └── *Panel.css               # Per-panel styles
│   │   ├── services/api.js                  # Stateless wrappers over wailsjs bindings
│   │   ├── stores/dashboard.js              # Singleton store: tabs, lastStatuses, listeners
│   │   └── styles/main.css                  # THEME: CSS variables, dark/light, tab bar, modal, cards
│   ├── wailsjs/           # AUTO-GENERATED bindings (do not hand-edit)
│   ├── dist/              # Built assets embedded by Wails
│   ├── vite.config.js     # Single-bundle build; inline runtime bootstrap calling App.CreateApp()
│   └── package.json
└── internal/
    ├── api/
    │   ├── dashboard.go   # DashboardAPI (health/dashboards), TabAPI (tabs), API response types
    │   └── cookies.go     # CookieAPI (list/set/delete/clear per domain) → cookie.Store
    ├── config/config.go   # Config struct, Load/Default/Save, env-var auth overrides
    ├── cookie/store.go    # Persistent cookie jar (data/cookies.json)
    ├── models/dashboard.go# Internal model types (services, dashboards, proxy)
    ├── services/
    │   ├── clients.go     # HTTPClient + NeuroNet/Minecraft/SlotBuilder clients + auth (env)
    │   ├── manager.go     # ServiceManager: CheckAllHealth + per-service dashboards
    │   └── proxy.go       # ProxyService: ReverseProxy handler, ProxyRequest, cookie inject/capture
    ├── tab/manager.go     # TabManager: persistent tabs (data/tabs.json)
    └── tray/              # D-Bus StatusNotifierItem tray (sni.go, menu.go) via godbus
```

## Backend Architecture

### main.go — the single source of app wiring
The `App` struct holds everything: `cfg`, `manager`, `proxy`, `dashboardAPI`, `tabAPI`,
`tabManager`, `cookieStore`, `cookieAPI`, `tray`. `NewApp()` wires them in order.
`main()` sets `GDK_BACKEND=x11` for KDE Wayland (X11/XWayland required for frameless),
then creates the app, then `runApp(app)` calls `wails.Run(opts)` with:
- `Frameless: true`, `SingleInstanceLock` (`it.alfio.Dashboard`), `OpenInspectorOnStartup: false`
  (the WebKit inspector is opened on demand from the header dropdown / Ctrl+Shift+F12 once
  the binary is built with the `devtools` tag, see inspector.go)
- `Linux.WebviewGpuPolicy`: from config `ui.webview_gpu_policy` (`always` default → hardware
  acceleration; `never` = software rendering = slow iframe scrolling). Fixed `Never` caused
  sluggish scrolling of iframed pages; see main.go `webviewGpuPolicy()`. Runs on X11 backend.

`go:embed frontend/dist frontend/wailsjs` + `go:embed build/icon.png`.

### Wails method binding pattern (IMPORTANT)
Every frontend-callable method exists in TWO forms on `App`:
- `Method(ctx context.Context, args...)` — Wails' usual signature
- `MethodNoContext(args...)` — wrapper calling the ctx version with `context.Background()`

The frontend's `api.js` calls the `...NoContext` variants via the generated
`wailsjs/go/main/App.js` exports. HTTP handlers live in `internal` (proxy), NOT in
`main.go`. Startup registers a `/api/proxy/{service}/{path...}` reverse proxy on `:8080`
only when `cfg.Proxy.Enabled`.

### internal/api/dashboard.go
`DashboardAPI` (service health + per-service dashboards, delegating to ServiceManager)
and `TabAPI` (tab CRUD):
- `TabAPI.ListTabs` — if tabs.json is empty, seeds the 3 defaults (neuronet/minecraft/slotbuilder)
- `TabAPI.UpdateTab(id, config)` — updates label/url/icon from a `map[string]interface{}`
- `TabAPI.UpdateTabSettings(id, settings)` — replaces the per-tab display settings bag
- `TabAPI.ReorderTabs(ids []int)` — reorders by id list (added for drag&drop)
- API response types (`TabInfo`, `ServiceStatus`, sub-dashboards, proxy types) all here.
  `TabInfo.Settings` carries the per-tab display settings (`{zoom: float, toolbar: bool}`).
- NOTE: `AddTab`/`RemoveTab` are NOT on TabAPI; they're methods directly on `App`
  (in main.go) operating on `a.tabManager`.

### inspector.go — WebKitGTK page inspector for TAB pages (require `devtools` tag)
cgo shim. The whole dashboard renders in ONE WebKitWebView, so attaching the
inspector to it would debug the dashboard DOM, not the tab's page. Instead a
dedicated *inspection webview* (a top-level window) is created ONCE, loads the
tab's real URL (`App.InspectorOpen(mode, url)`; service admin URL for built-in
panels, the iframe URL for URL tabs), enables developer extras on itself and is
the inspection target. It uses the default web context, so it shares
cookies/localStorage/session with the tab iframes. All GTK/WebKit access is
marshalled onto the GTK main thread via `g_idle_add` (Wails-bound Go methods
only enqueue a request carrying mode + URL). Layouts:
- `bottom` — `webkit_web_inspector_attach` (native dock at the bottom of the
  inspection window)
- `right`/`left` — `webkit_web_inspector_detach` then position/size the
  inspector's own window beside the inspection window, kept glued via a
  `configure-event` handler
- `float` — detached, freely movable
- `close` — `webkit_web_inspector_close`
Requires the `devtools` build tag for the MAIN webview's developer extras
(Ctrl+Shift+F12); the inspection webview enables its own devtools regardless.

### internal/cookie/store.go + api/cookies.go
Persistent per-domain cookie jar backed by `<exedir>/data/cookies.json` (portable).
- `Store` is goroutine-safe; `Set` replaces on (domain,path,name);
  `List(domain)`/`Delete`/`Clear` support suffix-domain matching;
  `HeaderValue(host,path)` builds a Cookie header honouring path/expiry.
- `CookieAPI` exposes Wails methods: `ListCookies`, `SetCookie`, `DeleteCookie`,
  `ClearCookies` (all + NoContext). Cookie fields: domain, path, name, value,
  secure, http_only, expires (RFC3339), created.

### internal/services/proxy.go
Real CORS-bypass reverse proxy, NOT a stub. Uses Go 1.22+ PathValue routing
(`/api/proxy/{service}/{path...}`). Per allowed-host it builds an `httputil.ReverseProxy`
with a `customTransport` that strips Origin/Referer, plus:
- `injectCookies(req, host)` — adds stored jar cookies before forwarding (customTransport)
- `captureCookies(resp, host)` — persists `Set-Cookie` from upstream back into the jar
  (wired in `modifyResponse` and after `ProxyRequest`)
- `ProxyRequest(ctx, models.ProxyRequest)` — Wails-exposed JSON proxy with header map,
  basic/bearer auth from env, cookie inject/capture.
CORS: `Access-Control-Allow-Origin: *`, strips X-Frame-Options/CSP so iframes work.

### internal/services/{clients,manager}.go
`HTTPClient` wraps an `http.Client`; `addAuth` applies config `Auth.Type`:
- `basic` → username/password from env vars (MINECRAFT_USER/PASS)
- `bearer` → token from env (SLOTBUILDER_TOKEN)
`ServiceManager` owns one client per configured service; `CheckAllHealth` probes each
(5s timeout) and `GetXxxDashboard` gathers lists for the panels.

### internal/tab/manager.go
`TabManager` persists `[]Tab{ID,Title,URL,Icon}` to `data/tabs.json`
(`<exedir>/data/tabs.json`; portable). `Add` assigns
`nextID`. `Update` keeps old values when empty. `Reorder(ids)` validates the id set.

### internal/tray
D-Bus StatusNotifierItem on the session bus (no appindicator CGO dependency).
Implements `org.kde.StatusNotifierItem` + `com.canonical.dbusmenu`. Tray icon is the
themed `"dashboard"` icon (installed by build/install-desktop.sh). Shows/hides/focuses
the window; `Quit` closes. `OpenExternal(url)` in main.go uses `xdg-open`.

## Frontend Architecture

### Single entry: frontend/src/main.js
Imports `styles/main.css` (must be imported here; tree-shaken otherwise) then mounts
`createApp()` from app.js. `vite.config.js` injects an inline bootstrap into the
generated `dist/index.html` that awaits `window.go.main.App.CreateApp()` before the
bundle loads (so `window.go` exists). Any vite.config change needs a full rebuild.

### app.js — UI chrome
`createApp()`:
1. `getSystemTheme()` → sets `document.documentElement.dataset.theme`
2. Injects header/brand/status pills/settings/win-controls/sidebar/content skeleton
3. Builds `tabBar` with callbacks: onTabChange→switchTab, onAddTab→openSettings,
   onReorder→`api.reorderTabs`+reload, onSetDefault,
   onOpenExternal→`api.openExternal`, onRenameTab, onDuplicateTab,
   onZoom/onResetZoom/onToggleToolbar → per-tab display settings
4. `panels` map (neuronet/minecraft/slotbuilder) — `panelForTab(tab)` matches by
   URL/label substring; matching → panel view; unknown URL → iframe view
5. **Tab views are kept alive** (`viewCache` Map). `switchTab()` only toggles the
   `active` class (CSS `display`) — it NEVER re-creates or destroys a view, so
   iframes keep their browsing context (login/session/cookies) when switching
   tabs. Non-active `.panel` roots are hidden by CSS (`.dashboard-content > .panel
   { display: none }`, `.panel.active { display: flex }`). `ensureView()` builds a
   view once, mounts panels once (`panel.mounted` flag), refreshes on first
   mount only; panel auto-refresh is `startAutoRefresh`/`stopAutoRefresh` bound
   to visibility (idempotent `startAutoRefresh` prevents duplicate intervals).
   Removed tabs are cleaned up via `destroyView` in `loadTabs`.
   **GOTCHA (fixed)**: `viewCache` keys are STRINGS (`String(tab.id)`). `switchTab`
   previously looked up with the numeric id (`viewCache.get(currentViewId)`), which
   never matched, so the previous panel kept `.active` and both were `display: flex`
   (tabs stacked one under the other). Always use `String(id)`/`String(tab.id)` for
   `viewCache.has()/get()`.
6. `loadServiceStatus()` every 30s → `tabBar.setStatuses()` (no header pills)
7. `loadTabs({preserveActive})` → `api.getTabs()`, drops stale cached views,
   respects current active tab vs `dashboardStore.getDefaultTab()` (localStorage
   `dashboard_default_tab`, default 'neuronet')
8. Window controls: min/max/close/to-maximise-state via `api.window*` (wailsjs runtime)
9. Keyboard nav (on document keydown): **Ctrl+Tab**/Ctrl+Shift+Tab cycle tabs,
   **Ctrl+T** open settings, **Ctrl+± / Ctrl+0** zoom the active tab.
10. **Per-tab display settings** — every tab carries a `settings` map persisted
    through `api.updateTabSettings` (→ `App.UpdateTabSettingsNoContext` → tabs.json).
    `zoom` (0.5–2.5, CSS `zoom` on the view root) and `toolbar` (bool, shows the
    URL-toolbar with reload + zoom on iframe tabs). `applyViewSettings()` applies
    them to kept-alive views; `setTabZoom`/`updateTabSettings` persist + apply live.
    URL tabs render an optional `.url-toolbar` (reload, zoom −/%, +, reset).
11. **Inspector dropdown** (header, right of settings) — `api.inspectorOpen(mode)`
    with `bottom|right|left|float` and `api.inspectorClose()`; hidden automatically
    when `api.inspectorAvailable()` is false.

### stores/dashboard.js
Tiny singleton: `tabs[]`, `lastStatuses[]`, `listeners`, `setTabs/subscribe/notify`,
`getDefaultTab/setDefaultTab` (localStorage). The panels are NOT rendered through the
store currently — switchTab in app.js mops the panel lifecycle.

### TabBar/TabBar.js (extended)
- Renders pill tabs (icon + label) with a per-tab status dot; tabs can only be
  removed from the Settings modal (no close X / middle-click).
- **Drag & drop** reordering: HTML5 dragstart/dragover/drop on `.tab-bar-item`;
  on drop it re-slices `this.tabs` and calls `onReorder(ids)`.
- **Context menu** (right click): Rinomina (inline input), Imposta come predefinito,
  Apri in browser, Duplica, Zoom −/%/+ (keep-open), Reimposta zoom, and a
  "Barra strumenti" toggle for iframe (URL) tabs. `setDefaultTabId()` marks the default.
- Tooltips include the URL for non-builtin tabs; labels are `escapeHtml`-encoded.
- `setTabs(tabs, {persist})`, `setActive(id)`, `setStatuses(statuses)`.

### Panel lifecycle contract (each *Panel.js)
- `constructor()` — no args; `api` imported directly.
- `render()` → root element with `#refresh-btn`; `mount(container)` attaches the
  listener and starts auto-refresh (30s); `unmount()` stops it. (`app.js` uses `refresh`.)
- `refresh()` → fetch via `api.get*Data()`, render content or `renderError(message)`
  with `#retry-btn`. All panels: NeuroNet(dashboard-grid), Minecraft, SlotBuilder.
- `startAutoRefresh` is **idempotent** (guarded by `this.refreshInterval`) so it
  never spawns duplicate intervals when a tab view is re-shown. Views are mounted
  once (`panel.mounted` flag in app.js) and only toggled visible/hidden.

### SettingsModal
Add/edit/remove tabs. Calls `api.saveTabConfig` (=App.AddTabNoContext), `api.updateTab`,
`api.removeTab`. On save it reloads tabs in app.js. Form fields: label, icon (select), URL.

## Theming System (IMPORTANT for styling)
`frontend/src/styles/main.css` defines CSS variables for dark (default) and a
`[data-theme="light"]` override block (KDE Plasma light detection in detectSystemTheme).
- Key vars: `--bg-primary/secondary/tertiary/hover`, `--fg-*`, `--accent`, `--border*`,
  `--success/warning/danger`, `--chrome-bg`, `--radius*`, `--shadow*`, `--font-*`.
- Any new component MUST use these variables, not hardcoded colors.
- Layout: header (drag region via `--wails-draggable: drag`, interactive elts set
  `data-win="no-drag"`), pill tab bar, grid/card content, modals.
- Special regions already handled: drag region, `.win-controls`, status pills, URL
  iframe content, context menu (`--bg-secondary`).
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
  neuronet: {...}       #   auth: {type: none|basic|bearer, *_env}, endpoints, proxy_enabled
minecraft/slotbuilder: ...
proxy: {enabled, allowed_hosts, timeout_seconds, max_body_size_mb}
ui: {theme: dark, default_tab: neuronet, webview_gpu_policy: always, tabs: [{id,label,icon,enabled}]}
```
### Auth env vars (never commit real values)
```bash
export MINECRAFT_USER=xxx
export MINECRAFT_PASS=xxx
export SLOTBUILDER_TOKEN=xxx
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
6. **WebView cookie persistence**: WebKitGTK's default cookie manager is IN-MEMORY
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
         signal, BEFORE the tab iframes navigate) calls `restoreWebviewCookies()`
         which re-adds those cookies via `webkit_cookie_manager_add_cookie`
         (`soup_cookie_new(...,-1)` = session).
       * `App.Shutdown` runs `snapshotAllWebviewCookies()` to catch last-minute
         changes.
     KNOWN LIMITATION: the `httponly` flag is dropped on the add→get roundtrip by
     WebKit (server still receives the cookie — httponly only blocks JS access).
   The app-level cookie jar (`<exedir>/data/cookies.json`) is separate and
   used by the proxy/panels.
7. **`ServiceManager`/panel data**: live calls hit real endpoints; if a service is
   down, refresh shows the error card (per-panel), header pills go offline.
8. **JS debug → Go log**: `window.runtime.LogPrint`/`LogDebug` do NOT reach
   `dashboard.log`. To debug DOM from JS, use `fmt.Printf`/`log.Printf` in a Go method
   (e.g. temporarily add a `DebugLog` bound method to App), regenerate bindings, and
   call it — `log.SetOutput(f)` sends it to the log file.

## Debugging Commands
```bash
cd /home/alfio/Projects/Dashboard

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
```

## Build & Run Flow (reliable order)
1. Edit Go in `main.go`/`internal/...`/`inspector.go`
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