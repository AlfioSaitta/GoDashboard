package tray

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

// TestMenuLayoutSignature guards the DBusMenu GetLayout reply format: KDE and
// libdbusmenu hosts demarshal the reply as (u(ia{sv}av)) — the layout must be
// returned as a plain struct (no variant wrapper) with children as variants of
// (ia{sv}av) entries. A variant-wrapped layout (uv) or byte children (ay)
// breaks the importer and the Plasma context menu never appears.
func TestMenuLayoutSignature(t *testing.T) {
	m := &menu{}
	root := m.rootLayout(-1)

	if got := dbus.SignatureOf(uint32(0), root).String(); got != "u(ia{sv}av)" {
		t.Fatalf("reply signature = %s, want u(ia{sv}av)", got)
	}
	if got := dbus.SignatureOf(root).String(); got != "(ia{sv}av)" {
		t.Fatalf("root signature = %s, want (ia{sv}av)", got)
	}
	if len(root.Children) == 0 {
		t.Fatalf("expected root children, got none")
	}
	for _, c := range root.Children {
		if got := dbus.SignatureOf(c.Value()).String(); got != "(ia{sv}av)" {
			t.Fatalf("child signature = %s, want (ia{sv}av)", got)
		}
	}
	if v, ok := root.Props["children-display"]; !ok || v.Value() != "submenu" {
		t.Fatalf("root must carry children-display=submenu (got %v, ok=%v)", v, ok)
	}
}