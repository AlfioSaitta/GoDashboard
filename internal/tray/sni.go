// Package tray provides a desktop status notifier (system tray) icon that
// works on KDE Plasma and other freedesktop environments over D-Bus,
// using the StatusNotifierItem (SNI) and DBusMenu protocols.
//
// It requires no native (CGO) dependencies: the icon is identified by its
// themed icon name ("dashboard"), which must be installed in the hicolor
// icon theme (see build/install-desktop.sh).
package tray

import (
	"fmt"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	sniObj   = "/StatusNotifierItem"
	menuObj  = "/MenuBar"
	sniIface = "org.kde.StatusNotifierItem"
	menuIface = "com.canonical.dbusmenu"
	propsIface = "org.freedesktop.DBus.Properties"
)

// Handler receives user actions from the tray.
type Handler struct {
	// ShowWindow makes the main window visible and focused.
	ShowWindow func()
	// ToggleWindow shows/hides the main window.
	ToggleWindow func()
	// Quit terminates the application.
	Quit func()
}

type iconPixmap struct {
	Width  int32
	Height int32
	Data   []byte
}

// SNI is a StatusNotifierItem tray icon.
type SNI struct {
	conn    *dbus.Conn
	h       *Handler
	menu    *menu
	busName string
	mu      sync.Mutex
}

// New creates and registers the tray icon on the session bus.
// Returns an error if the session bus is unavailable or registration fails.
func New(h *Handler) (*SNI, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("session bus: %w", err)
	}

	s := &SNI{conn: conn, h: h}

	// StatusNotifierItem object
	if err := conn.Export(s, sniObj, sniIface); err != nil {
		s.Close()
		return nil, fmt.Errorf("export sni: %w", err)
	}
	if err := conn.Export(s, sniObj, propsIface); err != nil {
		s.Close()
		return nil, fmt.Errorf("export sni props: %w", err)
	}

	// DBusMenu object
	if h == nil {
		h = &Handler{}
	}
	s.menu = &menu{toggle: h.ToggleWindow, quit: h.Quit}
	if err := conn.Export(s.menu, menuObj, menuIface); err != nil {
		s.Close()
		return nil, fmt.Errorf("export menu: %w", err)
	}
	if err := conn.Export(s.menu, menuObj, propsIface); err != nil {
		s.Close()
		return nil, fmt.Errorf("export menu props: %w", err)
	}

	s.busName = fmt.Sprintf("org.kde.StatusNotifierItem-%d-%d", os.Getpid(), os.Getppid())
	if _, err := conn.RequestName(s.busName, dbus.NameFlagDoNotQueue); err != nil {
		s.Close()
		return nil, fmt.Errorf("request name %s: %w", s.busName, err)
	}

	// Let a running StatusNotifierWatcher know about us (best effort).
	go func() { _ = conn.Object("org.kde.StatusNotifierWatcher", "/StatusNotifierWatcher").
		Call("org.kde.StatusNotifierWatcher.RegisterStatusNotifierItem", 0, s.busName).Err }()

	_ = conn.Emit(menuObj, menuIface+".LayoutUpdated", uint32(0))
	return s, nil
}

// Close removes the tray icon and disconnects from the bus.
func (s *SNI) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return
	}
	_, _ = s.conn.ReleaseName(s.busName)
	_ = s.conn.Close()
	s.conn = nil
}

// BusName returns the D-Bus name the tray icon is registered under.
func (s *SNI) BusName() string {
	return s.busName
}

// --- org.kde.StatusNotifierItem methods ---

func (s *SNI) Activate(x, y int32) *dbus.Error {
	if s.h != nil && s.h.ShowWindow != nil {
		go s.h.ShowWindow()
	}
	return nil
}

func (s *SNI) SecondaryActivate(x, y int32) *dbus.Error {
	return nil
}

func (s *SNI) ContextMenu(x, y int32) *dbus.Error {
	return nil
}

func (s *SNI) Scroll(delta int32, orientation string) *dbus.Error {
	return nil
}

// --- org.freedesktop.DBus.Properties for the SNI object ---

func (s *SNI) Get(iface, prop string) (dbus.Variant, *dbus.Error) {
	if v, ok := s.props()[prop]; ok {
		return v, nil
	}
	return dbus.Variant{}, unknownProperty(prop)
}

func (s *SNI) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	return s.props(), nil
}

func (s *SNI) Set(iface, prop string, value dbus.Variant) *dbus.Error {
	return propertyReadOnly(prop)
}

func (s *SNI) props() map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"Id":                 dbus.MakeVariant("dashboard"),
		"Category":           dbus.MakeVariant("ApplicationStatus"),
		"Title":              dbus.MakeVariant("Dashboard"),
		"Status":             dbus.MakeVariant("Active"),
		"WindowId":           dbus.MakeVariant(uint32(0)),
		"IconName":           dbus.MakeVariant("dashboard"),
		"OverlayIconName":    dbus.MakeVariant(""),
		"AttentionIconName":  dbus.MakeVariant(""),
		"IconPixmap":         dbus.MakeVariant([]iconPixmap{}),
		"OverlayIconPixmap":  dbus.MakeVariant([]iconPixmap{}),
		"AttentionIconPixmap": dbus.MakeVariant([]iconPixmap{}),
		"ToolTipTitle":       dbus.MakeVariant("Dashboard"),
		"ToolTipBody":        dbus.MakeVariant("Multi-service dashboard"),
		"Menu":               dbus.MakeVariant(dbus.ObjectPath(menuObj)),
		"ItemIsMenu":         dbus.MakeVariant(false),
	}
}