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

// exportShellNavState is called by the C shim (on the GTK main thread)
// whenever a tab page's navigation state changes (back/forward availability,
// loading progress). The chrome strip uses it to enable/disable the back,
// forward and reload/stop buttons.
//
//export exportShellNavState
func exportShellNavState(id C.int, canBack, canFwd, loading C.int) {
	wailsRuntime.EventsEmit(shellCtx, "shell:nav-state", map[string]interface{}{
		"tabId":        int(id),
		"canGoBack":    int(canBack) == 1,
		"canGoForward": int(canFwd) == 1,
		"loading":      int(loading) == 1,
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

// exportNotesMessage is called by the C shim whenever the notes webview posts
// a bridge message ("dashboardNotes", see tabs_shell.go). The message is a JSON
// {id, method, args} box handled by notes_bridge.go.
//
//export exportNotesMessage
func exportNotesMessage(msg *C.char) {
	if msg == nil {
		return
	}
	handleNotesMessage(C.GoString(msg))
}

// exportShellNotification is called by the C shim (on the GTK main thread)
// whenever a tab page displays a Web Notification. The notification is shown
// on the desktop via D-Bus org.freedesktop.Notifications (see internal/notify).
//
//export exportShellNotification
func exportShellNotification(id C.ulong, title, body *C.char) {
	t := ""
	if title != nil {
		t = C.GoString(title)
	}
	b := ""
	if body != nil {
		b = C.GoString(body)
	}
	if activeApp != nil {
		activeApp.showTabNotification(uint64(id), t, b)
	} else {
		logger.Printf("tab notification dropped (app not ready): id=%d title=%q", uint64(id), t)
	}
}
