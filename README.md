# Dashboard

A desktop dashboard for monitoring and controlling the web services operated
by a small team. It wraps a set of web panels into a single native window with
per-tab browsing, desktop notifications and an SSH terminal for every tab.

Built with **Go + Wails v2** and a **native GTK3/WebKitGTK shell** on Linux
(KDE/Plasma). Each tab is a real `WebKitWebView` (not an iframe), so sites keep
their cookies, localStorage and HSTS, and work without CORS restrictions.

## Monitored services

The services shown below are **theoretical examples** — configure your own
services and routes in `config.yaml`:

| Service | What it is | Example panel URL |
|---------|------------|-------------------|
| **Alpha** | AI research/admin console | `http://localhost:8080/admin` |
| **Beta** | Gaming community network admin | `https://admin.example.com` |
| **Gamma** | Business backoffice | `https://backoffice.example.com` |

All URLs are configurable and services can be added/edited/removed from the
Settings window at runtime (no code changes).

## Features

- **Native multi-tab shell** — a fixed chrome strip (header + pill tab bar)
  above a stack of per-tab `WebKitWebView`s. Tabs stay alive when you switch.
- **Per-tab SSH terminal** — open a native VTE terminal (local shell or SSH with
  password/agent/key auth) split below or beside the tab page with a draggable
  divider.
- **Desktop notifications** — tab pages using the Web Notifications API get
  real desktop bubbles via D-Bus (`org.freedesktop.Notifications`).
- **Persistent cookies & sessions** — SQLite-backed cookie storage plus a
  session-cookie snapshot/restore, so logins survive restarts.
- **Per-tab notes** — a floating editor window for each tab, persisted locally.
- **System tray** — StatusNotifierItem menu with tab activation, window show/
  hide and quit (no appindicator dependency).
- **Frameless, themed** — custom header with its own min/max/close, dark/light
  theme following the desktop.
- **Page inspector** — WebKit devtools per tab (dock bottom/right/left/float),
  available in `devtools` builds.

## Requirements

- Linux with GTK3, WebKitGTK **4.1** and **VTE 2.91**
  (`webkit2gtk-4.1` – WebKitGTK 2.52+, `vte-2.91`, `gtk+-3.0`)
- Go 1.25+ (toolchain 1.26.x used during development)
- Wails v2 CLI (`wails`), Node.js + npm for the frontend
- A session bus for D-Bus (tray, notifications)
- KDE/Plasma is the primary target; X11 is forced at startup because KWin's
  Wayland ignores frameless windows

## Build & run

```bash
./build.sh
./build/bin/Dashboard
```

`build.sh` runs the npm build then `wails build -s -tags "webkit2_41 devtools"`.

### Build prerequisites

```bash
# openSUSE
sudo zypper install go npm wails webkit2gtk3-devel typelib-1_0-Vte-2_91
# or, for the GTK/VTE dev headers:
#   gtk3-devel, webkit2gtk-4.1-devel, vte2_91-devel
```

### Manual build steps

```bash
cd frontend && npm run build && cd ..
wails build -s -tags "webkit2_41 devtools"
./build/bin/Dashboard
```

> **The `webkit2_41` and `devtools` build tags are mandatory** — without them
> the app segfaults or shows a blank window, and the page inspector silently
> does nothing.

### Root-less/portable data

All config and runtime data live in a `data/` folder **next to the executable**
(`<exedir>/data/`): `config.yaml`, `tabs.json`, `cookies.json`,
`cookies.sqlite`, the WebKit website data and `dashboard.log`. On first run it
is created and migrated from the legacy `~/.config/Dashboard`,
`~/.local/share/Dashboard`, `~/.cache/Dashboard`.

## Configuration

`config.yaml` in the data directory. Key sections:

```yaml
app: {name: Dashboard, version: 1.0.0, debug: true}
services:
  alpha:       # per-service: base_url, admin_path, auth, endpoints, proxy_enabled
    base_url: http://localhost:8080
    auth: {type: none}
  beta:
    base_url: https://admin.example.com
    auth:
      type: basic
      user_env: BETA_USER
      pass_env: BETA_PASS
  gamma:
    backoffice_url: https://backoffice.example.com
    frontend_url: https://www.example.com
    auth: {type: bearer, token_env: GAMMA_TOKEN}
    terminal:          # per-service SSH terminal config
      host, port, user, auth, password_env, key_path, dir, split
proxy: {enabled: true, allowed_hosts: [...], timeout_seconds, max_body_size_mb}
ui: {theme: dark, default_tab: alpha, webview_gpu_policy: always,
     tabs: [{id, label, icon, enabled}]}
```

Every service also carries a `terminal` block (host/port/user/auth/password_env/
key_path/dir/split) used by the per-tab SSH terminal.

### Auth via environment variables (never commit real values)

```bash
export BETA_USER=xxx
export BETA_PASS=xxx
export GAMMA_TOKEN=xxx
```

## Keyboard shortcuts

| Shortcut | Action |
|----------|--------|
| `Ctrl+Tab` / `Ctrl+Shift+Tab` | Cycle tabs |
| `Ctrl+T` | Open Settings |
| Ctrl+`+`/`-`, `Ctrl+0` | Zoom the active tab |
| `Ctrl+Shift+F12` | Open the page inspector (devtools build) |

## Project layout (highlights)

```
main.go        App wiring: config, managers, Wails run + shell setup
tabs_shell.go  Native tab shell (cgo): chrome strip + GtkStack + per-tab WebKitWebView
terminal.go    Per-tab VTE terminal in a GtkPaned split (cgo)
shell_app.go   Wails-bound shell controller (show/destroy/reorder/zoom/notes)
app_config.go  Service config editing via the Settings window
frontend/      Vite SPA: chrome UI + settings & notes floating windows
internal/      Config, cookie jar, proxy, services, tabs, tray, D-Bus notifications
```

Each module is documented in `AGENTS.md` (architecture + known gotchas for
contributors).

## Troubleshooting

- The app log is at `build/bin/data/dashboard.log` (`tail -f` it while
  developing).
- WebKitGTK does not pipe `console.error` to stdout — debug JS through Go
  `logger.Printf` calls or the page inspector.
- Rapid kill/restart cycles can trigger a flaky Wails/WebKit startup SIGABRT —
  just relaunch.

## License

Private/internal project.