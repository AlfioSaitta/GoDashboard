//go:build linux

package main

/*
#include <stdlib.h>
extern void shell_settings_eval(const char *js);
extern void shell_settings_resize(int w, int h);
extern void shell_settings_fit_start(void);
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"unsafe"
)

// activeApp is set in main() (right after NewApp) so the settings-window bridge
// can reach the App methods without going through Wails' IPC.
var activeApp *App

// SettingsBridgeMessage is the JSON box the settings webview posts through the
// "dashboardSettings" WebKit message handler.
type SettingsBridgeMessage struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Args   json.RawMessage `json:"args"`
}

// handleSettingsMessage is called on the GTK main thread by exportSettingsMessage.
// It dispatches the requested method on App and answers via window.__dashReply.
func handleSettingsMessage(raw string) {
	var req SettingsBridgeMessage
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		logger.Printf("settings bridge: bad message: %v", err)
		settingsReplyJSON(req.ID, false, nil, "messaggio non valido")
		return
	}
	// The first outgoing message ("getTabs") happens right after the page
	// finished loading — arm the C-side fit probe then (GTK main thread).
	if req.Method == "getTabs" {
		C.shell_settings_fit_start()
	}
	go func() {
		app := activeApp
		if app == nil {
			settingsReplyJSON(req.ID, false, nil, "app non inizializzata")
			return
		}
		result, err := dispatchSettings(app, req.Method, argsSlice(req.Args))
		if err != nil {
			logger.Printf("settings bridge: method=%s error=%v", req.Method, err)
			settingsReplyJSON(req.ID, false, nil, err.Error())
			return
		}
		settingsReplyJSON(req.ID, true, result, "")
	}()
}

// argsSlice turns the "args" JSON array into a []interface{}.
func argsSlice(raw json.RawMessage) []interface{} {
	if len(raw) == 0 {
		return nil
	}
	var parts []interface{}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil
	}
	return parts
}

func asString(v interface{}) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("atteso un valore testuale, ricevuto %T", v)
	}
	return s, nil
}

func asInt(v interface{}) (int, error) {
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	default:
		return 0, fmt.Errorf("atteso un numero, ricevuto %T", v)
	}
}

func asMap(v interface{}) (map[string]interface{}, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("atteso un oggetto, ricevuto %T", v)
	}
	return m, nil
}

func asInts(v interface{}) ([]int, error) {
	arr, ok := v.([]interface{})
	if !ok {
		return nil, fmt.Errorf("attesa una lista di id, ricevuto %T", v)
	}
	out := make([]int, 0, len(arr))
	for _, item := range arr {
		n, err := asInt(item)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func dispatchSettings(a *App, method string, args []interface{}) (interface{}, error) {
	ctx := context.Background()
	switch method {
	case "getTabs":
		return a.tabAPI.ListTabs(ctx)

	case "getAppConfig":
		return a.GetAppConfig(), nil

	case "saveAppConfig":
		if len(args) < 1 {
			return nil, fmt.Errorf("argomenti mancanti")
		}
		patch, err := asMap(args[0])
		if err != nil {
			return nil, err
		}
		return nil, a.SaveAppConfig(patch)

	case "getTheme":
		return a.GetThemeNoContext(), nil

	case "getSystemTheme":
		return detectSystemTheme(), nil

	case "setTheme":
		if len(args) < 1 {
			return nil, fmt.Errorf("argomenti mancanti")
		}
		theme, err := asString(args[0])
		if err != nil {
			return nil, err
		}
		return nil, a.SetThemeNoContext(theme)

	case "saveTabConfig":
		if len(args) < 1 {
			return nil, fmt.Errorf("argomenti mancanti")
		}
		config, err := asMap(args[0])
		if err != nil {
			return nil, err
		}
		return a.AddTab(ctx, config), nil

	case "removeTab":
		if len(args) < 1 {
			return nil, fmt.Errorf("argomenti mancanti")
		}
		id, err := asString(args[0])
		if err != nil {
			return nil, err
		}
		return a.RemoveTab(ctx, id), nil

	case "updateTab":
		if len(args) < 2 {
			return nil, fmt.Errorf("argomenti mancanti")
		}
		id, err := asString(args[0])
		if err != nil {
			return nil, err
		}
		config, err := asMap(args[1])
		if err != nil {
			return nil, err
		}
		return a.UpdateTab(ctx, id, config)

	case "updateTabSettings":
		if len(args) < 2 {
			return nil, fmt.Errorf("argomenti mancanti")
		}
		id, err := asString(args[0])
		if err != nil {
			return nil, err
		}
		settings, err := asMap(args[1])
		if err != nil {
			return nil, err
		}
		return a.UpdateTabSettings(ctx, id, settings)

	case "saveNotes":
		if len(args) < 2 {
			return nil, fmt.Errorf("argomenti mancanti")
		}
		id, err := asString(args[0])
		if err != nil {
			return nil, err
		}
		notes, err := asString(args[1])
		if err != nil {
			return nil, err
		}
		return a.tabAPI.SaveNotes(ctx, id, notes)

	case "reorderTabs":
		if len(args) < 1 {
			return nil, fmt.Errorf("argomenti mancanti")
		}
		ids, err := asInts(args[0])
		if err != nil {
			return nil, err
		}
		return nil, a.ReorderTabs(ctx, ids)

	case "tabsChanged":
		a.TabsChanged(ctx)
		return nil, nil

	case "closeSettings":
		a.CloseSettings(ctx)
		return nil, nil

	case "shellZoom":
		if len(args) < 2 {
			return nil, fmt.Errorf("argomenti mancanti")
		}
		id, err := asInt(args[0])
		if err != nil {
			return nil, err
		}
		level, err := asFloat(args[1])
		if err != nil {
			return nil, err
		}
		a.ShellZoom(ctx, id, level)
		return nil, nil

	case "resize":
		if len(args) < 2 {
			return nil, fmt.Errorf("argomenti mancanti")
		}
		w, err := asInt(args[0])
		if err != nil {
			return nil, err
		}
		h, err := asInt(args[1])
		if err != nil {
			return nil, err
		}
		settingsResize(w, h)
		return nil, nil

	default:
		return nil, fmt.Errorf("metodo sconosciuto: %q", method)
	}
}

func asFloat(v interface{}) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case int:
		return float64(n), nil
	default:
		return 0, fmt.Errorf("atteso un numero, ricevuto %T", v)
	}
}

// settingsReplyJSON serialises a reply and evaluates it in the settings webview.
func settingsReplyJSON(id int, ok bool, result interface{}, errMsg string) {
	payload := struct {
		ID     int         `json:"id"`
		OK     bool        `json:"ok"`
		Result interface{} `json:"result,omitempty"`
		Error  string      `json:"error,omitempty"`
	}{
		ID: id, OK: ok, Result: result, Error: errMsg,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		logger.Printf("settings bridge: marshal reply failed: %v", err)
		return
	}
	settingsReplyJS("window.__dashReply(" + string(b) + ");")
}

// settingsReplyJS runs the given JS inside the settings webview via the C shim
// (which marshals it onto the GTK main thread).
func settingsReplyJS(js string) {
	if js == "" {
		return
	}
	cjs := C.CString(js)
	defer C.free(unsafe.Pointer(cjs))
	C.shell_settings_eval(cjs)
}

// settingsResize resizes the settings window (called from the bridge "resize").
func settingsResize(w, h int) {
	C.shell_settings_resize(C.int(w), C.int(h))
}
