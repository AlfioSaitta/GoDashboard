// Package notify sends desktop notifications over the freedesktop
// org.freedesktop.Notifications D-Bus interface (KDE Plasma, GNOME, …) and
// watches for their dismissal, so callers can forward the close back to the
// source (e.g. a WebKit tab page).
//
// WebKitGTK is NOT built with libnotify on this stack (see the ldd check in
// AGENTS.md), so the Web Notifications API cannot fall back to a native
// notification; this package is the display backend for those notifications.
package notify

import (
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	busName    = "org.freedesktop.Notifications"
	objPath    = "/org/freedesktop/Notifications"
	notifyCall = busName + ".Notify"
	closedCall = busName + ".NotificationClosed"
)

// Notifier is a client of the freedesktop notification daemon.
type Notifier struct {
	conn *dbus.Conn

	mu       sync.Mutex
	onClosed map[uint32]func()
}

// New connects to the session bus and registers interest in NotificationClosed
// signals. It fails if the session bus is unavailable.
func New() (*Notifier, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("session bus: %w", err)
	}
	n := &Notifier{conn: conn, onClosed: map[uint32]func(){}}

	if err := conn.AddMatchSignal(dbus.WithMatchInterface(busName),
		dbus.WithMatchMember("NotificationClosed")); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("add signal match: %w", err)
	}
	ch := make(chan *dbus.Signal, 16)
	conn.Signal(ch)
	go n.listen(ch)
	return n, nil
}

// Notify shows a notification and returns its id. If replacesID is non-zero the
// new notification replaces the previous one with that id (used to keep a
// re-notified page notification from stacking).
func (n *Notifier) Notify(summary, body string, replacesID uint32) (uint32, error) {
	obj := n.conn.Object(busName, objPath)
	var id uint32
	err := obj.Call(notifyCall, 0,
		"Dashboard",            // app name
		replacesID,             // replaces_id
		"dashboard",            // app icon (themed, see build/install-desktop.sh)
		summary,                // summary
		body,                   // body
		[]string{},             // actions (none)
		map[string]dbus.Variant{}, // hints (none)
		int32(-1),              // expire_timeout: server default
	).Store(&id)
	if err != nil {
		return 0, fmt.Errorf("notify: %w", err)
	}
	return id, nil
}

// OnClosed registers cb to run when the notification with the given id is
// closed (dismissed by the user, clicked or expired). The callback runs on the
// notifier's own goroutine and is invoked at most once per id.
func (n *Notifier) OnClosed(id uint32, cb func()) {
	n.mu.Lock()
	n.onClosed[id] = cb
	n.mu.Unlock()
}

// Close releases the D-Bus connection.
func (n *Notifier) Close() {
	if n.conn != nil {
		_ = n.conn.Close()
		n.conn = nil
	}
}

func (n *Notifier) listen(ch <-chan *dbus.Signal) {
	for sig := range ch {
		if sig.Name != closedCall || len(sig.Body) < 2 {
			continue
		}
		id, ok := sig.Body[0].(uint32)
		if !ok {
			continue
		}
		n.mu.Lock()
		cb, ok := n.onClosed[id]
		delete(n.onClosed, id)
		n.mu.Unlock()
		if ok && cb != nil {
			cb()
		}
	}
}
