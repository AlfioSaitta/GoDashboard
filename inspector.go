//go:build linux

package main

import (
	"context"
	"fmt"
)

// Page inspector for a native TAB webview (see tabs_shell.go/shell_inspector).
//
// Every tab is its own WebKitWebView, so the inspector is attached to the web
// view of the ACTIVE tab ("bottom" docks it under the page, "right"/"left"
// detach and glue it to the side of the main window, "float" detaches it as a
// free window, "close" closes it). Operations are marshalled onto the GTK main
// thread via the shell request channel.

// InspectorAvailable reports whether the inspector feature can be used. All tab
// webviews enable WebKit developer extras themselves, so it is always available.
func (a *App) InspectorAvailable(ctx context.Context) bool {
	return true
}

func (a *App) InspectorAvailableNoContext() bool {
	return a.InspectorAvailable(context.Background())
}

// InspectorOpen opens the WebKit inspector of the given tab's webview in the
// requested layout: "bottom", "right", "left" or "float".
func (a *App) InspectorOpen(ctx context.Context, mode string, tabID int) error {
	switch mode {
	case "bottom", "float", "right", "left", "close":
	default:
		return fmt.Errorf("modo ispettore non valido %q", mode)
	}
	logger.Printf("InspectorOpen: mode=%s tabId=%d", mode, tabID)
	shellPostInspector(mode, tabID)
	return nil
}

func (a *App) InspectorOpenNoContext(mode string, tabID int) error {
	return a.InspectorOpen(context.Background(), mode, tabID)
}

// InspectorClose closes the page inspector of the given tab's webview.
func (a *App) InspectorClose(ctx context.Context, tabID int) error {
	logger.Printf("InspectorClose tabId=%d", tabID)
	shellPostInspector("close", tabID)
	return nil
}

func (a *App) InspectorCloseNoContext(tabID int) error {
	return a.InspectorClose(context.Background(), tabID)
}
