// Package paths resolves the portable data directory for Dashboard.
//
// All configuration and runtime data live in a single "data" folder placed
// next to the executable, so the whole application can be copied elsewhere
// (e.g. a USB stick) keeping its config, tabs, cookies and webview sessions.
package paths

import (
	"io"
	"os"
	"path/filepath"
	"sync"
)

var (
	once sync.Once
	dir  string
)

// DataDir returns the absolute portability data directory (<exedir>/data),
// creating it (and migrating any legacy per-user data) on first use.
func DataDir() string {
	once.Do(func() {
		exe, err := os.Executable()
		if err != nil {
			dir = "data"
		} else {
			dir = filepath.Join(filepath.Dir(exe), "data")
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			dir = "."
		}
		migrate()
	})
	return dir
}

// Dir returns the data directory, resolving it if needed.
func Dir() string { return DataDir() }

// File joins a filename (e.g. "tabs.json") to the data directory.
func File(name string) string { return filepath.Join(DataDir(), name) }

// ConfigFile returns path to config.yaml.
func ConfigFile() string { return File("config.yaml") }

// TabsFile returns path to tabs.json.
func TabsFile() string { return File("tabs.json") }

// CookiesFile returns path to cookies.json (app-level cookie jar).
func CookiesFile() string { return File("cookies.json") }

// CookieSQLite returns path to the WebKit persistent cookie SQLite DB.
func CookieSQLite() string { return File("cookies.sqlite") }

// WebviewCookiesFile returns path to the webview session-cookie snapshot.
func WebviewCookiesFile() string { return File("webview_cookies.json") }

// LogFile returns path to dashboard.log.
func LogFile() string { return File("dashboard.log") }

// WebviewDataDir returns the base data dir used for XDG_DATA_HOME redirect,
// keeping all WebKitGTK website data (localstorage/hsts/etc.) under data/.
func WebviewDataDir() string { return File("webview") }

// WebviewCacheDir returns the base cache dir used for XDG_CACHE_HOME redirect,
// keeping WebKitGTK caches under data/.
func WebviewCacheDir() string { return File("cache") }

// migrate copies per-user legacy data from ~/.config/Dashboard,
// ~/.local/share/Dashboard and ~/.cache/Dashboard into the data folder on the
// first run, so existing tabs/cookies/config are preserved. It must NOT call
// back into DataDir() (that would deadlock on the sync.Once).
func migrate() {
	legacy := legacyConfigDir()
	// Per-user .config files that previously lived in ~/.config/Dashboard.
	names := []string{"config.yaml", "tabs.json", "cookies.json", "cookies.sqlite", "webview_cookies.json"}
	for _, n := range names {
		copyFile(filepath.Join(legacy, n), filepath.Join(dir, n))
	}
	// WebView website data + caches. WebKitGTK stores these under
	// <XDG_DATA_HOME>/<prgname> and <XDG_CACHE_HOME>/<prgname>, where prgname
	// is the executable basename, so legacy data must land in that subfolder
	// (not at the top level) to be visible to the webview.
	appDir := filepath.Base(os.Args[0])
	if appDir == "" || appDir == "." || appDir == string(filepath.Separator) {
		appDir = "Dashboard"
	}
	copyTree(filepath.Join(legacyDataDir(), "Dashboard"), filepath.Join(dir, "webview", appDir))
	copyTree(filepath.Join(legacyCacheDir(), "Dashboard"), filepath.Join(dir, "cache", appDir))
	// Config: legacy config wins; otherwise reuse the one shipped next to the
	// executable or present in the current directory, so previously-adjusted
	// config.yaml is preserved in the portable data folder.
	configDest := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(configDest); err != nil {
		if exe, err := os.Executable(); err == nil {
			copyFile(filepath.Join(filepath.Dir(exe), "config.yaml"), configDest)
		}
		if _, err := os.Stat(configDest); err != nil {
			copyFile(filepath.Join(".", "config.yaml"), configDest)
		}
	}
}

func legacyConfigDir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "Dashboard")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "config", "Dashboard")
	}
	return filepath.Join(home, ".config", "Dashboard")
}

func legacyDataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".local", "share")
	}
	return filepath.Join(home, ".local", "share")
}

func legacyCacheDir() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".cache")
	}
	return filepath.Join(home, ".cache")
}

// copyFile copies src to dst only if dst does not exist yet. No-op on errors.
func copyFile(src, dst string) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	if _, err := os.Stat(dst); err == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return
	}
	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer out.Close()
	_, _ = io.Copy(out, in)
}

// copyTree recursively copies a directory's contents into dst when dst does
// not exist yet. No-op on errors.
func copyTree(src, dst string) {
	if _, err := os.Stat(dst); err == nil {
		return
	}
	_ = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		copyFile(path, target)
		return nil
	})
}
