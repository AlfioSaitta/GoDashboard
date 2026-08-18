package main

import (
	"context"
	"crypto/sha1"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"dashboard/internal/api"
	"dashboard/internal/config"
	"dashboard/internal/cookie"
	"dashboard/internal/notify"
	"dashboard/internal/paths"
	"dashboard/internal/services"
	"dashboard/internal/tab"
	"dashboard/internal/tray"
)

//go:embed frontend/dist frontend/wailsjs
var assets embed.FS

//go:embed build/icon.png
var appIcon []byte

var logger *log.Logger
var logFile *os.File

func initLogging() {
	exePath, err := os.Executable()
	if err != nil {
		log.Printf("Failed to get executable path: %v", err)
		logger = log.New(os.Stdout, "", log.LstdFlags)
		return
	}

	logPath := paths.LogFile()

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("Failed to open log file %s: %v", logPath, err)
		logger = log.New(os.Stdout, "", log.LstdFlags)
		return
	}

	logFile = f
	logger = log.New(f, "", log.LstdFlags)
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags | log.Lshortfile | log.Lmicroseconds)

	logger.Printf("=== Dashboard started at %s ===", time.Now().Format(time.RFC3339))
	logger.Printf("Executable: %s", exePath)
	logger.Printf("Log file: %s", logPath)
	logger.Printf("Go version: %s", runtime.Version())
	logger.Printf("PID: %d", os.Getpid())
}

func closeLogging() {
	if logFile != nil {
		logger.Printf("=== Dashboard shutting down at %s ===", time.Now().Format(time.RFC3339))
		logFile.Close()
	}
}

type App struct {
	ctx        context.Context
	cfg        *config.Config
	manager    *services.ServiceManager
	proxy      *services.ProxyService
	dashboardAPI *api.DashboardAPI
	tabAPI      *api.TabAPI
	tabManager  *tab.TabManager
	cookieStore *cookie.Store
	cookieAPI   *api.CookieAPI
	tray        *tray.SNI
	visMu       sync.RWMutex
	windowVisible bool
	notifier    *notify.Notifier
	notifMu     sync.Mutex
	notifMap    map[uint64]uint32 // webkit notification id -> desktop notification id
	notifRev    map[uint32]uint64 // desktop notification id -> webkit notification id
}

func NewApp() *App {
	initLogging()

	cfg, err := config.Load("")
	if err != nil {
		logger.Printf("Warning: failed to load config: %v", err)
		cfg = &config.Config{}
	} else {
		logger.Printf("Config loaded: app=%s, debug=%v, services=%d", cfg.App.Name, cfg.App.Debug, len(cfg.Services))
	}

	manager := services.NewServiceManager(cfg)
	logger.Printf("Service manager initialized")

	proxy := services.NewProxyService(cfg)
	logger.Printf("Proxy service initialized")

	dashboardAPI := api.NewDashboardAPI(manager, proxy)
	tabManager := tab.NewTabManager()
	tabAPI := api.NewTabAPI(tabManager)
	cookieStore := cookie.New()
	cookieAPI := api.NewCookieAPI(cookieStore)
	logger.Printf("API handlers created")

	return &App{
		cfg:          cfg,
		manager:      manager,
		proxy:        proxy,
		dashboardAPI: dashboardAPI,
		tabAPI:       tabAPI,
		tabManager:   tabManager,
		cookieStore:  cookieStore,
		cookieAPI:    cookieAPI,
		notifMap:     map[uint64]uint32{},
		notifRev:     map[uint32]uint64{},
	}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	shellCtx = ctx
	logger.Printf("App.Startup called")

	a.scheduleCookieSnapshots()

	// Eagerly pre-create the tab webviews (without loading URLs) a moment after
	// startup, so the first tab switch has no creation latency. Safe from any
	// goroutine: dispatched onto the GTK main thread via the request channel.
	a.precreateTabs()

	if a.cfg.Proxy.Enabled {
		logger.Printf("Registering proxy handler on /api/proxy/")
		go func() {
			http.Handle("/api/proxy/{service}/{path...}", a.proxy)
			logger.Printf("Proxy HTTP server starting on :8080")
			if err := http.ListenAndServe(":8080", nil); err != nil {
				logger.Printf("Proxy server error: %v", err)
			}
		}()
	}

	a.setWindowVisible(true)

	a.tray, _ = tray.New(&tray.Handler{
		ShowWindow:   a.ShowWindow,
		ToggleWindow: a.ToggleWindow,
		ShowTab:      a.ShowTabFromTray,
		Quit:         func() { wailsRuntime.Quit(a.ctx) },
	})
	if a.tray != nil {
		logger.Printf("Tray icon registered as %s", a.tray.BusName())
		a.refreshTrayTabs()
	} else {
		logger.Printf("Tray icon unavailable on this session")
	}

	if n, err := notify.New(); err != nil {
		logger.Printf("Desktop notifications unavailable: %v", err)
	} else {
		a.notifier = n
		logger.Printf("Desktop notification support enabled")
	}

	wailsRuntime.EventsOn(ctx, "shutdown", func(data ...interface{}) {
		logger.Printf("Shutdown event received")
		closeLogging()
	})
}

func (a *App) OnDomReady(ctx context.Context) {
	logger.Printf("App.OnDomReady called")
	if _, err := restoreWebviewCookies(); err != nil {
		logger.Printf("Warning: failed to restore webview cookies: %v", err)
	}
}

func (a *App) Shutdown(ctx context.Context) {
	snapshotAllWebviewCookies(a.cookieSnapshotURIs())
	if a.tray != nil {
		a.tray.Close()
		a.tray = nil
	}
	if a.notifier != nil {
		a.notifier.Close()
		a.notifier = nil
	}
	logger.Printf("App.Shutdown called")
	closeLogging()
}

// cookieSnapshotURIs returns the root URIs whose webview session cookies must
// be snapshotted (all configured tabs + service endpoints).
func (a *App) cookieSnapshotURIs() []string {
	uris := map[string]bool{}
	if a.tabManager != nil {
		for _, t := range a.tabManager.List() {
			if u := hostURI(t.URL); u != "" {
				uris[u] = true
			}
		}
	}
	if a.cfg != nil {
		for _, svc := range a.cfg.Services {
			for _, u := range []string{svc.BaseURL, svc.BackofficeURL} {
				if h := hostURI(u); h != "" {
					uris[h] = true
				}
			}
		}
	}
	list := make([]string, 0, len(uris))
	for u := range uris {
		list = append(list, u)
	}
	return list
}

// scheduleCookieSnapshots snapshots webview session cookies for all tab and
// service hosts periodically (WebKitGTK only persists cookies with an expiry,
// so session cookies would otherwise be lost on app restart).
func (a *App) scheduleCookieSnapshots() {
	uris := a.cookieSnapshotURIs()
	if err := scheduleWebviewCookieSnapshots(uris, 20); err != nil {
		logger.Printf("Warning: failed to schedule cookie snapshots: %v", err)
	}
}

func (a *App) setWindowVisible(visible bool) {
	a.visMu.Lock()
	a.windowVisible = visible
	a.visMu.Unlock()
}

func (a *App) isWindowVisible() bool {
	a.visMu.RLock()
	defer a.visMu.RUnlock()
	return a.windowVisible
}

// ShowWindow shows and focuses the main window (used by the tray).
func (a *App) ShowWindow() {
	a.setWindowVisible(true)
	if a.ctx != nil {
		wailsRuntime.WindowUnminimise(a.ctx)
		wailsRuntime.WindowShow(a.ctx)
	}
}

// ToggleWindow shows or hides the main window (used by the tray).
func (a *App) ToggleWindow() {
	visible := a.isWindowVisible()
	if a.ctx == nil {
		return
	}
	if visible {
		a.setWindowVisible(false)
		wailsRuntime.WindowHide(a.ctx)
	} else {
		a.setWindowVisible(true)
		wailsRuntime.WindowUnminimise(a.ctx)
		wailsRuntime.WindowShow(a.ctx)
	}
}

// ShowTabFromTray shows the main window and switches to the tab with the given
// id (used by the tray context menu). It also emits "shell:tab-activated" so
// the chrome strip highlights the tab in the tab bar.
func (a *App) ShowTabFromTray(id int) {
	a.ShowWindow()
	a.ShellShowTab(context.Background(), id)
	if a.ctx != nil {
		wailsRuntime.EventsEmit(a.ctx, "shell:tab-activated", map[string]interface{}{"tabId": id})
	}
}

// showTabNotification displays a Web Notification raised by a tab page on the
// desktop (via D-Bus org.freedesktop.Notifications). Called from the C shell on
// the GTK main thread; the D-Bus roundtrip happens off-thread so the UI never
// blocks. When the desktop notification is dismissed, the page's onclose fires
// (webkit_notification_close on the GTK thread).
func (a *App) showTabNotification(webID uint64, title, body string) {
	logger.Printf("Tab notification: id=%d title=%q body=%q", webID, title, body)
	n := a.notifier
	if n == nil {
		logger.Printf("Notification daemon unavailable; tab notification dropped")
		return
	}
	go func() {
		a.notifMu.Lock()
		replaces := a.notifMap[webID]
		a.notifMu.Unlock()

		deskID, err := n.Notify(title, body, replaces)
		if err != nil {
			logger.Printf("Failed to send tab notification: %v", err)
			return
		}
		a.notifMu.Lock()
		a.notifMap[webID] = deskID
		a.notifRev[deskID] = webID
		a.notifMu.Unlock()

		n.OnClosed(deskID, func() {
			a.notifMu.Lock()
			if wID, ok := a.notifRev[deskID]; ok {
				delete(a.notifRev, deskID)
				delete(a.notifMap, wID)
			}
			a.notifMu.Unlock()
			closeWebNotification(webID)
		})
	}()
}

// refreshNotificationOrigins keeps WebKit's pre-allowed notification origins in
// sync with the tabs. WebKitGTK 2.52 needs the origins of every page a tab may
// load pre-allowed via WebKitWebContext::initialize-notification-permissions,
// otherwise requestPermission() silently resolves to "denied" and the tab's Web
// Notifications never surface (see tabs_shell.go).
func (a *App) refreshNotificationOrigins() {
	if a.tabManager == nil {
		return
	}
	seen := map[string]bool{}
	add := func(raw string) {
		if o := originOfURL(raw); o != "" && !seen[o] {
			seen[o] = true
		}
	}
	// Every stored/custom tab URL...
	for _, t := range a.tabManager.List() {
		add(a.resolveTabURL(t.URL))
	}
	// ...plus every configured service entrypoint (covers the first launch,
	// before the tabs have been seeded into tabs.json).
	if a.cfg != nil {
		for _, svc := range a.cfg.Services {
			add(svc.BaseURL)
			add(svc.BackofficeURL)
			add(svc.FrontendURL)
			if svc.AdminPath != "" {
				add(strings.TrimRight(svc.BaseURL, "/") + svc.AdminPath)
			}
		}
	}
	origins := make([]string, 0, len(seen))
	for o := range seen {
		origins = append(origins, o)
	}
	shellSetNotificationOrigins(origins)
}

// originOfURL returns the scheme://host[:port] origin of an absolute http(s)
// URL, or "" when the URL is not absolute or not http(s).
func originOfURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !u.IsAbs() {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	if u.Hostname() == "" {
		return ""
	}
	o := u.Scheme + "://" + u.Hostname()
	if p := u.Port(); p != "" {
		o += ":" + p
	}
	return o
}

// refreshTrayTabs syncs the tab list shown in the tray context menu with the
// current persistent tabs.
func (a *App) refreshTrayTabs() {
	if a.tray == nil || a.tabManager == nil {
		return
	}
	tabs := a.tabManager.List()
	items := make([]tray.TabItem, 0, len(tabs))
	for _, t := range tabs {
		name := t.Title
		if name == "" {
			name = t.URL
		}
		if name == "" {
			name = fmt.Sprintf("Tab %d", t.ID)
		}
		items = append(items, tray.TabItem{ID: t.ID, Name: name})
	}
	a.tray.SetTabs(items)
}

// GetSystemTheme returns "dark" or "light" based on the KDE Plasma color scheme.
func (a *App) GetSystemTheme(ctx context.Context) string {
	return detectSystemTheme()
}

func (a *App) GetSystemThemeNoContext() string {
	return detectSystemTheme()
}

// GetTheme returns the user's theme preference: "dark", "light" or "system"
// (follow the OS/KDE color scheme). Legacy configs without an explicit
// preference resolve to "system".
func (a *App) GetTheme(ctx context.Context) string {
	v := ""
	if a.cfg != nil {
		v = a.cfg.UI.Theme
	}
	switch v {
	case "dark", "light":
		return v
	default:
		return "system"
	}
}

func (a *App) GetThemeNoContext() string {
	return a.GetTheme(context.Background())
}

// SetTheme persists the user's theme preference ("dark"|"light"|"system").
func (a *App) SetTheme(ctx context.Context, theme string) error {
	switch theme {
	case "dark", "light", "system":
	default:
		return fmt.Errorf("tema non valido: %q", theme)
	}
	if a.cfg != nil {
		a.cfg.UI.Theme = theme
	}
	if a.cfg == nil {
		return nil
	}
	if err := a.cfg.Save(paths.ConfigFile()); err != nil {
		logger.Printf("SetTheme: failed to save config: %v", err)
		return err
	}
	logger.Printf("SetTheme: %q saved", theme)
	wailsRuntime.EventsEmit(a.ctx, "shell:theme", theme)
	return nil
}

func (a *App) SetThemeNoContext(theme string) error {
	return a.SetTheme(context.Background(), theme)
}

func detectSystemTheme() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "dark"
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "kdeglobals"))
	if err != nil {
		return "dark"
	}
	inGeneral := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inGeneral = line == "[General]"
			continue
		}
		if inGeneral && strings.HasPrefix(line, "ColorScheme=") {
			scheme := strings.TrimSpace(strings.TrimPrefix(line, "ColorScheme="))
			lower := strings.ToLower(scheme)
			switch {
			case strings.Contains(lower, "dark"):
				return "dark"
			case strings.Contains(lower, "light"):
				return "light"
			case lower == "breeze" || lower == "default":
				return "light"
			}
		}
	}
	return "dark"
}

func (a *App) GetDashboard(ctx context.Context) (*api.DashboardData, error) {
	return a.dashboardAPI.GetDashboard(ctx)
}

func (a *App) GetServicesStatus(ctx context.Context) ([]api.ServiceStatus, error) {
	return a.dashboardAPI.GetServicesStatus(ctx)
}

func (a *App) GetNeuroNetData(ctx context.Context) (*api.NeuroNetDashboard, error) {
	return a.dashboardAPI.GetNeuroNetData(ctx)
}

func (a *App) GetMinecraftData(ctx context.Context) (*api.MinecraftDashboard, error) {
	return a.dashboardAPI.GetMinecraftData(ctx)
}

func (a *App) GetSlotBuilderData(ctx context.Context) (*api.SlotBuilderDashboard, error) {
	return a.dashboardAPI.GetSlotBuilderData(ctx)
}

func (a *App) ProxyRequest(ctx context.Context, req api.ProxyRequest) (*api.ProxyResponse, error) {
	return a.dashboardAPI.ProxyRequest(ctx, req)
}

func (a *App) NeuroNetInference(ctx context.Context, modelID string, input map[string]interface{}) (map[string]interface{}, error) {
	return a.dashboardAPI.NeuroNetInference(ctx, modelID, input)
}

func (a *App) MinecraftConsoleCommand(ctx context.Context, serverID, command string) error {
	return a.dashboardAPI.MinecraftConsoleCommand(ctx, serverID, command)
}

func (a *App) ListTabs(ctx context.Context) ([]api.TabInfo, error) {
	return a.tabAPI.ListTabs(ctx)
}

func (a *App) ListTabsNoContext() ([]api.TabInfo, error) {
	return a.tabAPI.ListTabs(context.Background())
}

func (a *App) GetDashboardNoContext() (*api.DashboardData, error) {
	return a.dashboardAPI.GetDashboard(context.Background())
}

func (a *App) GetServicesStatusNoContext() ([]api.ServiceStatus, error) {
	return a.dashboardAPI.GetServicesStatus(context.Background())
}

func (a *App) GetNeuroNetDataNoContext() (*api.NeuroNetDashboard, error) {
	return a.dashboardAPI.GetNeuroNetData(context.Background())
}

func (a *App) GetMinecraftDataNoContext() (*api.MinecraftDashboard, error) {
	return a.dashboardAPI.GetMinecraftData(context.Background())
}

func (a *App) GetSlotBuilderDataNoContext() (*api.SlotBuilderDashboard, error) {
	return a.dashboardAPI.GetSlotBuilderData(context.Background())
}

func (a *App) AddTabNoContext(config map[string]interface{}) api.Tab {
	url := ""
	if v, ok := config["url"].(string); ok {
		url = v
	}
	title := ""
	if v, ok := config["label"].(string); ok {
		title = v
	}
	icon := ""
	if v, ok := config["icon"].(string); ok {
		icon = v
	}
	tab := a.tabManager.AddWithIcon(url, title, icon)
	return api.Tab{ID: tab.ID, Title: tab.Title, URL: tab.URL, Icon: tab.Icon}
}

func (a *App) RemoveTabNoContext(id string) bool {
	// Try parsing as integer ID first
	intID, err := strconv.Atoi(id)
	if err == nil {
		return a.tabManager.Remove(intID)
	}
	// If not an integer, try removing by URL
	return a.tabManager.RemoveByURL(id)
}

func (a *App) AddTab(ctx context.Context, config map[string]interface{}) api.Tab {
	url := ""
	if v, ok := config["url"].(string); ok {
		url = v
	}
	title := ""
	if v, ok := config["label"].(string); ok {
		title = v
	}
	icon := ""
	if v, ok := config["icon"].(string); ok {
		icon = v
	}
	tab := a.tabManager.AddWithIcon(url, title, icon)
	return api.Tab{ID: tab.ID, Title: tab.Title, URL: tab.URL, Icon: tab.Icon}
}

func (a *App) RemoveTab(ctx context.Context, id string) bool {
	// Try parsing as integer ID first
	intID, err := strconv.Atoi(id)
	if err == nil {
		return a.tabManager.Remove(intID)
	}
	// If not an integer, try removing by URL
	return a.tabManager.RemoveByURL(id)
}

func (a *App) UpdateTab(ctx context.Context, id string, config map[string]interface{}) (api.Tab, error) {
	return a.tabAPI.UpdateTab(ctx, id, config)
}

func (a *App) UpdateTabNoContext(id string, config map[string]interface{}) (api.Tab, error) {
	return a.tabAPI.UpdateTab(context.Background(), id, config)
}

// UpdateTabSettings updates the per-tab display settings (zoom, toolbar, ...).
func (a *App) UpdateTabSettings(ctx context.Context, id string, settings map[string]interface{}) (api.Tab, error) {
	return a.tabAPI.UpdateTabSettings(ctx, id, settings)
}

func (a *App) UpdateTabSettingsNoContext(id string, settings map[string]interface{}) (api.Tab, error) {
	return a.tabAPI.UpdateTabSettings(context.Background(), id, settings)
}

// GetNotes returns the persistent notes of the given tab.
func (a *App) GetNotes(ctx context.Context, id string) (string, error) {
	return a.tabAPI.GetNotes(ctx, id)
}

func (a *App) GetNotesNoContext(id string) (string, error) {
	return a.tabAPI.GetNotes(context.Background(), id)
}

// SaveNotes persists the notes of the given tab.
func (a *App) SaveNotes(ctx context.Context, id string, notes string) (api.Tab, error) {
	return a.tabAPI.SaveNotes(ctx, id, notes)
}

func (a *App) SaveNotesNoContext(id string, notes string) (api.Tab, error) {
	return a.tabAPI.SaveNotes(context.Background(), id, notes)
}

func (a *App) ReorderTabs(ctx context.Context, ids []int) error {
	return a.tabAPI.ReorderTabs(ctx, ids)
}

func (a *App) ReorderTabsNoContext(ids []int) error {
	return a.tabAPI.ReorderTabs(context.Background(), ids)
}

func (a *App) ListCookies(ctx context.Context, domain string) []api.CookieInfo {
	return a.cookieAPI.ListCookies(ctx, domain)
}

func (a *App) ListCookiesNoContext(domain string) []api.CookieInfo {
	return a.cookieAPI.ListCookies(context.Background(), domain)
}

func (a *App) SetCookie(ctx context.Context, ck api.CookieInfo) api.CookieInfo {
	return a.cookieAPI.SetCookie(ctx, ck)
}

func (a *App) SetCookieNoContext(ck api.CookieInfo) api.CookieInfo {
	return a.cookieAPI.SetCookie(context.Background(), ck)
}

func (a *App) DeleteCookie(ctx context.Context, domain, path, name string) bool {
	return a.cookieAPI.DeleteCookie(ctx, domain, path, name)
}

func (a *App) DeleteCookieNoContext(domain, path, name string) bool {
	return a.cookieAPI.DeleteCookie(context.Background(), domain, path, name)
}

func (a *App) ClearCookies(ctx context.Context, domain string) int {
	return a.cookieAPI.ClearCookies(ctx, domain)
}

func (a *App) ClearCookiesNoContext(domain string) int {
	return a.cookieAPI.ClearCookies(context.Background(), domain)
}

// OpenExternal opens a URL in the system's default browser (used by the tab
// context menu "Apri in browser" action).
func (a *App) OpenExternal(ctx context.Context, url string) error {
	openExternal(url)
	return nil
}

func (a *App) OpenExternalNoContext(url string) error {
	openExternal(url)
	return nil
}

// openExternal opens a URL with xdg-open when available.
func openExternal(url string) {
	cmd := exec.Command("xdg-open", url)
	if err := cmd.Start(); err != nil {
		logger.Printf("Failed to open %s: %v", url, err)
	}
}

func (a *App) CreateApp() error {
	logger.Printf("CreateApp called from frontend")
	// The app is already initialized in Startup/OnDomReady
	// This method just confirms the frontend is ready
	return nil
}

func main() {
	// KWin on Wayland ignores gtk_window_set_decorated() (KDE bug 484800),
	// so a Frameless window still shows the title bar. Force the X11/XWayland
	// backend on KDE Wayland sessions where window managers honour undecorated
	// windows. GTK reads GDK_BACKEND during initialisation, so it must be set
	// before wails.Run.
	sessionType := os.Getenv("XDG_SESSION_TYPE")
	desktop := strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP"))
	if sessionType == "wayland" && strings.Contains(desktop, "kde") {
		if err := os.Setenv("GDK_BACKEND", "x11"); err != nil {
			log.Printf("Warning: failed to set GDK_BACKEND=x11: %v", err)
		}
	}

	// Keep WebKitGTK's own website data (localstorage, hsts, cache…) inside
	// the portable data folder next to the executable, so the whole app
	// directory can be copied elsewhere without leaving traces in the user's
	// home. Must be set before wails.Run (before the first webview exists).
	if err := os.Setenv("XDG_DATA_HOME", paths.WebviewDataDir()); err != nil {
		log.Printf("Warning: failed to set XDG_DATA_HOME: %v", err)
	}
	if err := os.Setenv("XDG_CACHE_HOME", paths.WebviewCacheDir()); err != nil {
		log.Printf("Warning: failed to set XDG_CACHE_HOME: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			logger.Printf("PANIC recovered: %v", r)
			logger.Printf("Stack trace:\n%s", debug.Stack())
		}
	}()

	app := NewApp()
	activeApp = app

	// Pre-allow the notification origins before the first web process launches
	// (WebKitGTK 2.52 reads the map at process start).
	app.refreshNotificationOrigins()

	if err := enablePersistentCookies(); err != nil {
		log.Printf("Warning: failed to enable persistent webview cookies: %v", err)
	}

	// Schedule the native tab-shell repackage (chrome strip + tab stack) to run
	// at the start of the GTK main loop, right after Wails pipes its widgets.
	shellSetup()

	// WebKit discards nothing on rebuild: the settings page and its assets get
	// cached under <exedir>/data/cache/Dashboard keyed by URL, so an older
	// bundle can outlive a new binary (stale settings window, no auto-resize).
	// BUT the cache also holds every tab-site resource (WebKitCache blobs), so
	// wiping it on every start would force all tab pages to re-download their
	// assets after every launch. We only invalidate it when the embedded
	// frontend bundle actually changed (hash comparison against a marker in
	// the data dir), so a rebuild still gets a fresh bundle while routine
	// launches keep the tab-site disk cache intact.
	clearWebviewCacheIfStale()

	err := runApp(app)
	if err != nil {
		logger.Fatalf("App error: %v", err)
	}
}

// bundleHash computes a stable hash of the embedded frontend bundle (file
// names + contents, walked in lexical order) so we can detect rebuilds.
func bundleHash() (string, error) {
	h := sha1.New()
	err := fs.WalkDir(assets, "frontend/dist", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if _, err := io.WriteString(h, path); err != nil {
			return err
		}
		f, err := assets.Open(path)
		if err != nil {
			return err
		}
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// clearWebviewCacheIfStale removes WebKit's on-disk resource cache only when
// the embedded frontend bundle hash differs from the marker stored in the data
// dir (i.e. after a rebuild). Keeps the tab-site resource cache across normal
// launches. See main().
func clearWebviewCacheIfStale() {
	appCache := filepath.Join(paths.WebviewCacheDir(), "Dashboard")
	marker := paths.File(".dash-bundle-hash")

	sum, err := bundleHash()
	if err != nil {
		logger.Printf("Warning: cannot hash frontend bundle: %v", err)
		return
	}

	prev, rerr := os.ReadFile(marker)
	if rerr == nil && string(prev) == sum {
		return // bundle unchanged: keep the tab-site cache warm
	}

	logger.Printf("Frontend bundle changed (hash %s); clearing webview cache", sum)
	if err := os.RemoveAll(appCache); err != nil {
		logger.Printf("Warning: failed to clear webview cache: %v", err)
	}
	if err := os.MkdirAll(appCache, 0o755); err != nil {
		logger.Printf("Warning: failed to recreate webview cache dir: %v", err)
	}
	if err := os.WriteFile(marker, []byte(sum), 0o644); err != nil {
		logger.Printf("Warning: failed to write bundle hash marker: %v", err)
	}
}

func runApp(app *App) error {
	opts := &options.App{
		Title:  app.cfg.App.Name,
		Width:  1400,
		Height: 900,
		MinWidth:  1024,
		MinHeight: 768,
		DisableResize: false,
		Fullscreen: false,
		Frameless: true,
		StartHidden: false,
		AlwaysOnTop: false,
		BackgroundColour: &options.RGBA{R: 16, G: 20, B: 29, A: 255},
		Linux: &linux.Options{
			Icon:             appIcon,
			ProgramName:      "Dashboard",
			WebviewGpuPolicy: webviewGpuPolicy(app.cfg.UI.WebviewGpuPolicy),
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "it.alfio.Dashboard",
			OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
				logger.Printf("Second instance launch detected, focusing existing window")
				app.ShowWindow()
			},
		},
		OnStartup: app.Startup,
		OnDomReady: app.OnDomReady,
		OnShutdown: app.Shutdown,
		Bind: []interface{}{
			app,
		},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Debug: options.Debug{
			// The inspector is openable on demand from the header dropdown
			// (App.InspectorOpen) and via Ctrl+Shift+F12, so it must not pop
			// open automatically on every start.
			OpenInspectorOnStartup: false,
		},
	}

	return wails.Run(opts)
}

// webviewGpuPolicy maps the config string (always|ondemand|never) to the Wails
// Linux WebviewGpuPolicy. Defaults to "always" (hardware acceleration) when the
// config value is empty or unknown, since software rendering makes scrolling of
// iframed pages sluggish.
func webviewGpuPolicy(v string) linux.WebviewGpuPolicy {
	switch v {
	case "never":
		return linux.WebviewGpuPolicyNever
	case "ondemand", "on-demand":
		return linux.WebviewGpuPolicyOnDemand
	default:
		return linux.WebviewGpuPolicyAlways
	}
}