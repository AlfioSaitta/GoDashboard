//go:build linux

package main

/*
#cgo linux pkg-config: gtk+-3.0
#cgo !webkit2_41 pkg-config: webkit2gtk-4.0
#cgo webkit2_41 pkg-config: webkit2gtk-4.1
*/

import "C"

import (
	"context"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// shellCtx is set by App.Startup so the GTK callbacks can reach the Wails
// runtime (events are emitted to the chrome webview, which listens for them).
var shellCtx = context.Background()

// exportShellTitle is called by the C shim (on the GTK main thread) whenever a
// tab page changes its document title. It forwards the event to the frontend.
//
//export exportShellTitle
func exportShellTitle(id C.int, title *C.char) {
	t := ""
	if title != nil {
		t = C.GoString(title)
	}
	wailsRuntime.EventsEmit(shellCtx, "shell:title", map[string]interface{}{
		"tabId": int(id),
		"title": t,
	})
}

// exportShellUri is called by the C shim (on the GTK main thread) whenever a
// tab page navigates to a new URI.
//
//export exportShellUri
func exportShellUri(id C.int, uri *C.char) {
	u := ""
	if uri != nil {
		u = C.GoString(uri)
	}
	wailsRuntime.EventsEmit(shellCtx, "shell:uri", map[string]interface{}{
		"tabId": int(id),
		"uri":   u,
	})
}

// exportSettingsMessage is called by the C shim whenever the settings webview
// posts a bridge message ("dashboardSettings", see tabs_shell.go). The message
// is a JSON {id, method, args} box handled by settings_bridge.go.
//
//export exportSettingsMessage
func exportSettingsMessage(msg *C.char) {
	if msg == nil {
		return
	}
	handleSettingsMessage(C.GoString(msg))
}