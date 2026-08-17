//go:build linux

package main

/*
#cgo linux pkg-config: gtk+-3.0 vte-2.91
#cgo !webkit2_41 pkg-config: webkit2gtk-4.0
#cgo webkit2_41 pkg-config: webkit2gtk-4.1
*/

import "C"

import (
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// exportShellTerminalState is called by the C shim (terminal.go, on the GTK
// main thread) whenever a per-tab terminal starts/stops or is shown/hidden. It
// forwards the state to the chrome strip so the frontend can reflect it on the
// tab bar. Kept in its own file WITHOUT the terminal C preamble: a //export in
// terminal.go would pull that preamble into the amalgamated export object and
// duplicate the shell_terminal_* symbols at link time (gotcha 13).
//
//export exportShellTerminalState
func exportShellTerminalState(id C.int, running C.int, visible C.int) {
	wailsRuntime.EventsEmit(shellCtx, "shell:terminal-state", map[string]interface{}{
		"tabId":   int(id),
		"running": int(running) == 1,
		"visible": int(visible) == 1,
	})
}

// exportShellTerminalSplit is called by the C shim (terminal.go, on the GTK
// main thread) when the user toggles the terminal split orientation from the
// terminal header bar. It persists the choice into the service terminal config.
// Same file placement rationale as exportShellTerminalState (gotcha 13).
//
//export exportShellTerminalSplit
func exportShellTerminalSplit(id C.int, orient C.int) {
	app := activeApp
	if app == nil {
		return
	}
	app.terminalSetSplitPersist(int(id), int(orient))
}
