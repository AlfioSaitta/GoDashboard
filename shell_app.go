//go:build linux

package main

import (
	"context"
	"strings"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"dashboard/internal/tab"
)

// Shell methods drive the native tab shell (see tabs_shell.go): the main Wails
// webview is a fixed chrome strip and each tab lives in its own WebKitWebView.

// resolveTabURL maps the stored tab URL to the address loaded in the native
// webview: real http(s) URLs are used as-is, while the built-in service keys
// ("neuronet"|"minecraft"|"slotbuilder") resolve to the service admin pages.
func (a *App) resolveTabURL(raw string) string {
	u := strings.TrimSpace(raw)
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	for key, svc := range a.cfg.Services {
		if key != "" && strings.EqualFold(u, key) {
			if svc.BackofficeURL != "" {
				return svc.BackofficeURL
			}
			if svc.AdminPath != "" {
				return strings.TrimRight(svc.BaseURL, "/") + svc.AdminPath
			}
			return svc.BaseURL
		}
	}
	switch strings.ToLower(u) {
	case "neuronet":
		return "http://localhost:8000/admin"
	case "minecraft":
		return "http://51.75.77.248:9800"
	case "slotbuilder":
		return "https://backoffice.7casinogames.com"
	}
	return u
}

func (a *App) tabZoomOf(t tab.Tab) float64 {
	if v, ok := t.Settings["zoom"].(float64); ok && v >= 0.5 && v <= 2.5 {
		return v
	}
	return 1
}

// ShellShowTab shows the native webview of the tab (creating it lazily when this
// is its first activation) and gives it keyboard focus.
func (a *App) ShellShowTab(ctx context.Context, id int) {
	t, ok := a.tabManager.Get(id)
	if !ok {
		logger.Printf("ShellShowTab: tab %d not found", id)
		return
	}
	url := a.resolveTabURL(t.URL)
	if url == "" {
		url = "about:blank"
	}
	logger.Printf("ShellShowTab: id=%d url=%s", id, url)
	shellPost(1, id, url, a.tabZoomOf(t), 0)
}

func (a *App) ShellShowTabNoContext(id int) {
	a.ShellShowTab(context.Background(), id)
}

// ShellDestroyTab destroys the native webview of a removed tab.
func (a *App) ShellDestroyTab(ctx context.Context, id int) {
	logger.Printf("ShellDestroyTab: id=%d", id)
	shellPost(2, id, "", 0, 0)
}

func (a *App) ShellDestroyTabNoContext(id int) {
	a.ShellDestroyTab(context.Background(), id)
}

// ShellReorder reorders the native webviews to match the tab order.
func (a *App) ShellReorder(ctx context.Context, ids []int) {
	shellPostIDs(3, ids)
}

func (a *App) ShellReorderNoContext(ids []int) {
	a.ShellReorder(context.Background(), ids)
}

// ShellZoom applies a (native) zoom level to the tab webview.
func (a *App) ShellZoom(ctx context.Context, id int, level float64) {
	logger.Printf("ShellZoom: id=%d level=%.2f", id, level)
	shellPost(4, id, "", level, 0)
}

func (a *App) ShellZoomNoContext(id int, level float64) {
	a.ShellZoom(context.Background(), id, level)
}

// ShellSetChromeHeight resizes the chrome strip webview to the measured height
// of header + tab bar (reported by the frontend).
func (a *App) ShellSetChromeHeight(ctx context.Context, height int) {
	if height < 40 || height > 400 {
		return
	}
	logger.Printf("ShellSetChromeHeight: %d", height)
	shellPost(5, 0, "", 0, height)
}

func (a *App) ShellSetChromeHeightNoContext(height int) {
	a.ShellSetChromeHeight(context.Background(), height)
}

// OpenSettings opens the Impostazioni window (a native modal hosting the app
// bundle in "#settings" view).
func (a *App) OpenSettings(ctx context.Context) {
	logger.Printf("OpenSettings")
	shellPost(6, 0, "", 0, 0)
}

func (a *App) OpenSettingsNoContext() {
	a.OpenSettings(context.Background())
}

// CloseSettings closes the Impostazioni window.
func (a *App) CloseSettings(ctx context.Context) {
	logger.Printf("CloseSettings")
	shellPost(7, 0, "", 0, 0)
}

func (a *App) CloseSettingsNoContext() {
	a.CloseSettings(context.Background())
}

// TabsChanged notifies the chrome strip that the tab list changed (used by the
// settings window after save/update/remove/reorder).
func (a *App) TabsChanged(ctx context.Context) {
	logger.Printf("TabsChanged")
	wailsRuntime.EventsEmit(a.ctx, "tabs:changed")
}

func (a *App) TabsChangedNoContext() {
	a.TabsChanged(context.Background())
}