package main

import (
	"context"
	"fmt"

	"dashboard/internal/config"
	"dashboard/internal/paths"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// configMap renders a section of cfg as plain JSON-able maps for the settings
// window. Auth/Terminal fields carry ENV VAR NAMES (never the secret values).
func serviceConfigMap(svc config.ServiceConfig) map[string]interface{} {
	return map[string]interface{}{
		"name":            svc.Name,
		"base_url":        svc.BaseURL,
		"backoffice_url":  svc.BackofficeURL,
		"frontend_url":    svc.FrontendURL,
		"admin_path":      svc.AdminPath,
		"api_prefix":      svc.APIPrefix,
		"timeout_seconds": svc.TimeoutSeconds,
		"proxy_enabled":   svc.ProxyEnabled,
		"auth": map[string]interface{}{
			"type":         svc.Auth.Type,
			"username_env": svc.Auth.UsernameEnv,
			"password_env": svc.Auth.PasswordEnv,
			"token_env":    svc.Auth.TokenEnv,
		},
		"terminal": map[string]interface{}{
			"enabled":      svc.Terminal.Enabled,
			"host":         svc.Terminal.Host,
			"port":         svc.Terminal.Port,
			"user":         svc.Terminal.User,
			"auth":         svc.Terminal.Auth,
			"password_env": svc.Terminal.PasswordEnv,
			"key_path":     svc.Terminal.KeyPath,
			"dir":          svc.Terminal.Dir,
			"split":        svc.Terminal.Split,
		},
	}
}

// GetAppConfig returns the current configuration for the settings window:
// ui (theme/default tab/gpu policy), proxy and the per-service sections.
func (a *App) GetAppConfig() map[string]interface{} {
	services := make(map[string]interface{}, len(a.cfg.Services))
	for key, svc := range a.cfg.Services {
		services[key] = serviceConfigMap(svc)
	}
	return map[string]interface{}{
		"ui": map[string]interface{}{
			"theme":              a.cfg.UI.Theme,
			"default_tab":        a.cfg.UI.DefaultTab,
			"webview_gpu_policy": a.cfg.UI.WebviewGpuPolicy,
		},
		"proxy": map[string]interface{}{
			"enabled":          a.cfg.Proxy.Enabled,
			"allowed_hosts":    a.cfg.Proxy.AllowedHosts,
			"timeout_seconds":  a.cfg.Proxy.TimeoutSeconds,
			"max_body_size_mb": a.cfg.Proxy.MaxBodySizeMB,
		},
		"services": services,
	}
}

// SaveAppConfig applies a partial configuration patch (same shape and keys as
// GetAppConfig) coming from the settings window, persists config.yaml and
// re-applies the affected runtime components (service clients, reverse proxy,
// notification origins). Only the provided keys are touched.
func (a *App) SaveAppConfig(patch map[string]interface{}) error {
	if a.cfg == nil {
		return fmt.Errorf("configurazione non inizializzata")
	}

	ui, _ := asMap(patch["ui"])
	themeChanged := false
	if ui != nil {
		if v, ok := asStringSafe(ui["theme"]); ok {
			switch v {
			case "dark", "light", "system":
				if a.cfg.UI.Theme != v {
					themeChanged = true
				}
				a.cfg.UI.Theme = v
			}
		}
		if v, ok := asStringSafe(ui["default_tab"]); ok {
			a.cfg.UI.DefaultTab = v
		}
		if v, ok := asStringSafe(ui["webview_gpu_policy"]); ok {
			switch v {
			case "always", "ondemand", "never":
				a.cfg.UI.WebviewGpuPolicy = v
			}
		}
	}

	proxy, _ := asMap(patch["proxy"])
	if proxy != nil {
		if v, ok := asBoolSafe(proxy["enabled"]); ok {
			a.cfg.Proxy.Enabled = v
		}
		if v, ok := asIntSafe(proxy["timeout_seconds"]); ok {
			a.cfg.Proxy.TimeoutSeconds = v
		}
		if v, ok := asIntSafe(proxy["max_body_size_mb"]); ok {
			a.cfg.Proxy.MaxBodySizeMB = v
		}
		if v, ok := asStringsSafe(proxy["allowed_hosts"]); ok {
			a.cfg.Proxy.AllowedHosts = v
		}
	}

	services, _ := asMap(patch["services"])
	for key, raw := range services {
		svcPatch, ok := asMapSafe(raw)
		if !ok {
			continue
		}
		svc, ok := a.cfg.Services[key]
		if !ok {
			svc = defaultServiceConfig()
		}
		if v, ok := asStringSafe(svcPatch["name"]); ok {
			svc.Name = v
		}
		if v, ok := asStringSafe(svcPatch["base_url"]); ok {
			svc.BaseURL = v
		}
		if v, ok := asStringSafe(svcPatch["backoffice_url"]); ok {
			svc.BackofficeURL = v
		}
		if v, ok := asStringSafe(svcPatch["frontend_url"]); ok {
			svc.FrontendURL = v
		}
		if v, ok := asStringSafe(svcPatch["admin_path"]); ok {
			svc.AdminPath = v
		}
		if v, ok := asStringSafe(svcPatch["api_prefix"]); ok {
			svc.APIPrefix = v
		}
		if v, ok := asIntSafe(svcPatch["timeout_seconds"]); ok {
			svc.TimeoutSeconds = v
		}
		if v, ok := asBoolSafe(svcPatch["proxy_enabled"]); ok {
			svc.ProxyEnabled = v
		}
		if auth, _ := asMap(svcPatch["auth"]); auth != nil {
			if v, ok := asStringSafe(auth["type"]); ok {
				switch v {
				case "none", "basic", "bearer":
					svc.Auth.Type = v
				}
			}
			if v, ok := asStringSafe(auth["username_env"]); ok {
				svc.Auth.UsernameEnv = v
			}
			if v, ok := asStringSafe(auth["password_env"]); ok {
				svc.Auth.PasswordEnv = v
			}
			if v, ok := asStringSafe(auth["token_env"]); ok {
				svc.Auth.TokenEnv = v
			}
		}
		if term, _ := asMap(svcPatch["terminal"]); term != nil {
			if v, ok := asBoolSafe(term["enabled"]); ok {
				svc.Terminal.Enabled = v
			}
			if v, ok := asStringSafe(term["host"]); ok {
				svc.Terminal.Host = v
			}
			if v, ok := asIntSafe(term["port"]); ok {
				svc.Terminal.Port = v
			}
			if v, ok := asStringSafe(term["user"]); ok {
				svc.Terminal.User = v
			}
			if v, ok := asStringSafe(term["auth"]); ok {
				switch v {
				case "password", "key", "agent":
					svc.Terminal.Auth = v
				}
			}
			if v, ok := asStringSafe(term["password_env"]); ok {
				svc.Terminal.PasswordEnv = v
			}
			if v, ok := asStringSafe(term["key_path"]); ok {
				svc.Terminal.KeyPath = v
			}
			if v, ok := asStringSafe(term["dir"]); ok {
				svc.Terminal.Dir = v
			}
			if v, ok := asStringSafe(term["split"]); ok {
				switch v {
				case "h", "v":
					svc.Terminal.Split = v
				}
			}
		}
		a.cfg.Services[key] = svc
	}

	if err := a.cfg.Save(paths.ConfigFile()); err != nil {
		logger.Printf("SaveAppConfig: failed to save config: %v", err)
		return err
	}
	logger.Printf("SaveAppConfig: configuration saved to %s", paths.ConfigFile())

	// Re-apply runtime components so a settings change takes effect live.
	a.manager.Reconfigure(a.cfg)
	a.refreshNotificationOrigins()
	a.TabsChanged(context.Background())

	if themeChanged {
		wailsRuntime.EventsEmit(a.ctx, "shell:theme", a.cfg.UI.Theme)
	}

	return nil
}

func defaultServiceConfig() config.ServiceConfig {
	return config.ServiceConfig{Auth: config.AuthConfig{Type: "none"}}
}

func asStringSafe(v interface{}) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func asIntSafe(v interface{}) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}

func asBoolSafe(v interface{}) (bool, bool) {
	b, ok := v.(bool)
	return b, ok
}

func asMapSafe(v interface{}) (map[string]interface{}, bool) {
	m, ok := v.(map[string]interface{})
	return m, ok
}

func asStringsSafe(v interface{}) ([]string, bool) {
	arr, ok := v.([]interface{})
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out, true
}
