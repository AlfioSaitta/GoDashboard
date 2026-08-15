package tray

import (
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	itemShowHide = int32(1)
	itemQuit     = int32(2)
)

// menu implements the DBusMenu protocol at /MenuBar for the tray item.
type menu struct {
	toggle func()
	quit   func()
	mu     sync.Mutex
}

type menuEntry struct {
	ID       int32
	Props    map[string]dbus.Variant
	Children []byte
}

type groupEntry struct {
	ID    int32
	Props map[string]dbus.Variant
}

type menuLayout struct {
	Entries []menuEntry
}

// --- helper accessors ---

func (m *menu) entries() []menuEntry {
	return []menuEntry{
		{ID: itemShowHide, Props: menuItemProps("Mostra/Nascondi")},
		{ID: itemQuit, Props: menuItemProps("Esci")},
	}
}

func menuItemProps(label string) map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"label":   dbus.MakeVariant(label),
		"enabled": dbus.MakeVariant(true),
		"type":    dbus.MakeVariant("standard"),
	}
}

// --- com.canonical.dbusmenu methods ---

func (m *menu) GetLayout(parentID, recursionDepth int32, propNames []string) (uint32, dbus.Variant, *dbus.Error) {
	return 0, dbus.MakeVariant(menuLayout{Entries: m.entries()}), nil
}

func (m *menu) GetGroupProperties(ids []int32, propNames []string) ([]groupEntry, *dbus.Error) {
	known := map[int32]map[string]dbus.Variant{}
	for _, e := range m.entries() {
		known[e.ID] = e.Props
	}
	out := []groupEntry{}
	for _, id := range ids {
		if p, ok := known[id]; ok {
			out = append(out, groupEntry{ID: id, Props: p})
		}
	}
	return out, nil
}

func (m *menu) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	for _, e := range m.entries() {
		if e.ID == id {
			if v, ok := e.Props[name]; ok {
				return v, nil
			}
		}
	}
	return dbus.Variant{}, unknownProperty(name)
}

func (m *menu) AboutToShow(id int32) (bool, *dbus.Error) {
	return true, nil
}

func (m *menu) AboutToShowGroup(ids []int32) ([]int32, *dbus.Error) {
	return []int32{}, nil
}

func (m *menu) RevokeLayout(parentID uint32) *dbus.Error {
	return nil
}

func (m *menu) PopulatePopup(id int32, timestamp uint64) (uint32, *dbus.Error) {
	return 0, nil
}

func (m *menu) Event(id int32, event string, data dbus.Variant, timestamp uint64) *dbus.Error {
	if event != "clicked" {
		return nil
	}
	switch id {
	case itemShowHide:
		if m.toggle != nil {
			go m.toggle()
		}
	case itemQuit:
		if m.quit != nil {
			go m.quit()
		}
	}
	return nil
}

// --- org.freedesktop.DBus.Properties for the menu object ---

func (m *menu) Get(iface, prop string) (dbus.Variant, *dbus.Error) {
	if v, ok := m.menuProps()[prop]; ok {
		return v, nil
	}
	return dbus.Variant{}, unknownProperty(prop)
}

func (m *menu) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	return m.menuProps(), nil
}

func (m *menu) Set(iface, prop string, value dbus.Variant) *dbus.Error {
	return propertyReadOnly(prop)
}

func (m *menu) menuProps() map[string]dbus.Variant {
	items := []groupEntry{}
	for _, e := range m.entries() {
		items = append(items, groupEntry{ID: e.ID, Props: e.Props})
	}
	return map[string]dbus.Variant{
		"Version":         dbus.MakeVariant(int32(0)),
		"TextDirection":   dbus.MakeVariant("ltr"),
		"Status":          dbus.MakeVariant("normal"),
		"IconThemePath":   dbus.MakeVariant([]string{}),
		"ItemsProperties": dbus.MakeVariant(items),
	}
}

func unknownProperty(name string) *dbus.Error {
	return &dbus.Error{Name: "org.freedesktop.DBus.Error.InvalidArgs", Body: []interface{}{"no such property: " + name}}
}

func propertyReadOnly(name string) *dbus.Error {
	return &dbus.Error{Name: "org.freedesktop.DBus.Error.PropertyReadOnly", Body: []interface{}{name + " is read-only"}}
}