//go:build linux

package main

/*
#include <stdlib.h>
extern void shell_notes_eval(const char *js);
extern void shell_notes_resize(int w, int h);
extern void shell_notes_fit_start(void);
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"unsafe"

	"dashboard/internal/api"
	"dashboard/internal/tab"
)

// NotesBridgeMessage is the JSON box the notes webview posts through the
// "dashboardNotes" WebKit message handler (see tabs_shell.go).
type NotesBridgeMessage struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Args   json.RawMessage `json:"args"`
}

// handleNotesMessage is called on the GTK main thread by exportNotesMessage.
// It dispatches the requested method on App and answers via window.__dashReply.
func handleNotesMessage(raw string) {
	var req NotesBridgeMessage
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		logger.Printf("notes bridge: bad message: %v", err)
		notesReplyJSON(req.ID, false, nil, "messaggio non valido")
		return
	}
	// The first outgoing message is "getTab" (right after the page finished
	// loading) — arm the C-side fit probe then (GTK main thread).
	if req.Method == "getTab" {
		C.shell_notes_fit_start()
	}
	go func() {
		app := activeApp
		if app == nil {
			notesReplyJSON(req.ID, false, nil, "app non inizializzata")
			return
		}
		result, err := dispatchNotes(app, req.Method, argsSlice(req.Args))
		if err != nil {
			logger.Printf("notes bridge: method=%s error=%v", req.Method, err)
			notesReplyJSON(req.ID, false, nil, err.Error())
			return
		}
		notesReplyJSON(req.ID, true, result, "")
	}()
}

func dispatchNotes(a *App, method string, args []interface{}) (interface{}, error) {
	ctx := context.Background()
	switch method {
	case "getTab":
		if len(args) < 1 {
			return nil, fmt.Errorf("argomenti mancanti")
		}
		id, err := asInt(args[0])
		if err != nil {
			return nil, err
		}
		t, ok := a.tabManager.Get(id)
		if !ok {
			return nil, fmt.Errorf("tab %d non trovato", id)
		}
		return tabNotesMap(t), nil

	case "saveNote":
		// args: tabId, noteId (0/negativo = nuova), title, content
		if len(args) < 4 {
			return nil, fmt.Errorf("argomenti mancanti")
		}
		id, err := asInt(args[0])
		if err != nil {
			return nil, err
		}
		noteID, err := asInt(args[1])
		if err != nil {
			return nil, err
		}
		title, err := asString(args[2])
		if err != nil {
			return nil, err
		}
		content, err := asString(args[3])
		if err != nil {
			return nil, err
		}
		var tab api.Tab
		if noteID > 0 {
			tab, err = a.tabAPI.UpdateNote(ctx, fmt.Sprintf("%d", id), fmt.Sprintf("%d", noteID), title, content)
		} else {
			tab, err = a.tabAPI.AddNote(ctx, fmt.Sprintf("%d", id), title, content)
		}
		if err != nil {
			return nil, err
		}
		// Let the chrome refresh the per-tab note indicator right away.
		a.TabsChanged(ctx)
		return tabNotesMap(apiToTab(tab)), nil

	case "deleteNote":
		if len(args) < 2 {
			return nil, fmt.Errorf("argomenti mancanti")
		}
		id, err := asInt(args[0])
		if err != nil {
			return nil, err
		}
		noteID, err := asInt(args[1])
		if err != nil {
			return nil, err
		}
		tab, err := a.tabAPI.DeleteNote(ctx, fmt.Sprintf("%d", id), fmt.Sprintf("%d", noteID))
		if err != nil {
			return nil, err
		}
		a.TabsChanged(ctx)
		return tabNotesMap(apiToTab(tab)), nil

	case "getTheme":
		return a.GetThemeNoContext(), nil

	case "getSystemTheme":
		return detectSystemTheme(), nil

	case "closeNotes":
		a.CloseNotes(ctx)
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
		notesResize(w, h)
		return nil, nil

	default:
		return nil, fmt.Errorf("metodo sconosciuto: %q", method)
	}
}

// notesReplyJSON serialises a reply and evaluates it in the notes webview.
func notesReplyJSON(id int, ok bool, result interface{}, errMsg string) {
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
		logger.Printf("notes bridge: marshal reply failed: %v", err)
		return
	}
	notesReplyJS("window.__dashReply(" + string(b) + ");")
}

// notesReplyJS runs the given JS inside the notes webview via the C shim
// (which marshals it onto the GTK main thread).
func notesReplyJS(js string) {
	if js == "" {
		return
	}
	cjs := C.CString(js)
	defer C.free(unsafe.Pointer(cjs))
	C.shell_notes_eval(cjs)
}

// notesResize resizes the notes window (called from the bridge "resize").
func notesResize(w, h int) {
	C.shell_notes_resize(C.int(w), C.int(h))
}

// apiToTab converts an api.Tab (returned by the TabAPI note methods) back to the
// internal tab.Tab so the bridge can expose a uniform shape to the notes page.
func apiToTab(t api.Tab) tab.Tab {
	return tab.Tab{ID: t.ID, Title: t.Title, URL: t.URL, Icon: t.Icon, Notes: t.Notes, Settings: t.Settings}
}

// tabNotesMap serialises a tab together with its note list for the notes page.
func tabNotesMap(t tab.Tab) map[string]interface{} {
	return map[string]interface{}{
		"id":         t.ID,
		"label":      t.Title,
		"icon":       t.Icon,
		"notes":      t.Notes,
		"note_count": len(t.Notes),
	}
}
