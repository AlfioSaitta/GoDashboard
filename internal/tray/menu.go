package tray

import (
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	itemShowHide = int32(1)
	itemQuit     = int32(2)
	sepTabs      = int32(10)
	sepQuit      = int32(11)
	firstTabItem = int32(100)
)

// tabItem is a single tab entry shown in the tray menu.
type tabItem struct {
	ID   int
	Name string
}

// menuLayout is a single node of the DBusMenu GetLayout response. The D-Bus
// signature is (ia{sv}av): unique id, properties dict, child nodes (as
// variants containing another (ia{sv}av) struct). The root node has id 0 and
// its children are the top-level menu entries.
type menuLayout struct {
	ID       int32
	Props    map[string]dbus.Variant
	Children []dbus.Variant
}

// groupEntry is an (id, properties) pair used by GetGroupProperties and
// ItemsProperties.
type groupEntry struct {
	ID    int32
	Props map[string]dbus.Variant
}

// menu implements the DBusMenu protocol at /MenuBar for the tray item.
type menu struct {
	toggle  func()
	showTab func(int)
	quit    func()
	mu      sync.RWMutex
	tabs    []tabItem
	rev     uint32
}

type menuEntry struct {
	ID    int32
	Props map[string]dbus.Variant
}

// --- revision ---

func (m *menu) revision() uint32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rev
}

// --- helper accessors ---

func (m *menu) entries() []menuEntry {
	m.mu.RLock()
	tabs := make([]tabItem, len(m.tabs))
	copy(tabs, m.tabs)
	m.mu.RUnlock()

	entries := []menuEntry{
		{ID: itemShowHide, Props: menuItemProps("Mostra/Nascondi")},
	}
	if len(tabs) > 0 {
		entries = append(entries, menuEntry{ID: sepTabs, Props: separatorProps()})
		for i, t := range tabs {
			entries = append(entries, menuEntry{ID: firstTabItem + int32(i), Props: menuItemProps(t.Name)})
		}
		entries = append(entries, menuEntry{ID: sepQuit, Props: separatorProps()})
	}
	entries = append(entries, menuEntry{ID: itemQuit, Props: menuItemProps("Esci")})
	return entries
}

// rootLayout builds the GetLayout response root node. The root carries the
// "children-display" property required by hosts (libdbusmenu/KDE) to render the
// entries as a submenu; recursionDepth==0 asks for the root without children.
func (m *menu) rootLayout(recursionDepth int32) menuLayout {
	root := menuLayout{
		ID: 0,
		Props: map[string]dbus.Variant{
			"children-display": dbus.MakeVariant("submenu"),
		},
	}
	if recursionDepth == 0 {
		return root
	}
	for _, e := range m.entries() {
		root.Children = append(root.Children, dbus.MakeVariant(menuLayout{ID: e.ID, Props: e.Props}))
	}
	return root
}

func menuItemProps(label string) map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"label":   dbus.MakeVariant(label),
		"enabled": dbus.MakeVariant(true),
		"type":    dbus.MakeVariant("standard"),
	}
}

func separatorProps() map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"label":   dbus.MakeVariant(""),
		"enabled": dbus.MakeVariant(false),
		"type":    dbus.MakeVariant("separator"),
	}
}

// SetTabs replaces the tab list shown in the tray menu and bumps the layout
// revision so hosts refetch it.
func (m *menu) SetTabs(tabs []tabItem) {
	m.mu.Lock()
	m.tabs = append([]tabItem(nil), tabs...)
	m.rev++
	m.mu.Unlock()
}

// tabForID maps a D-Bus menu item id back to the tab it represents.
func (m *menu) tabForID(id int32) (tabItem, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	idx := int(id - firstTabItem)
	if idx < 0 || idx >= len(m.tabs) {
		return tabItem{}, false
	}
	return m.tabs[idx], true
}

// --- com.canonical.dbusmenu methods ---

// GetLayout returns the root menu node. The D-Bus out signature MUST be
// (u(ia{sv}av)): revision + the layout struct WITHOUT a variant wrapper.
// KDE/Qt importers (KF DBusMenuImporter, Qt QDBusMenuAdaptor) demarshal the
// reply as QDBusPendingReply<uint, DBusMenuLayoutItem>; returning the layout
// inside a dbus.Variant (uv) breaks their demarshalling and the tray menu
// silently never appears on Plasma.
func (m *menu) GetLayout(parentID, recursionDepth int32, propNames []string) (uint32, menuLayout, *dbus.Error) {
	if parentID != 0 {
		return m.revision(), menuLayout{ID: parentID}, nil
	}
	return m.revision(), m.rootLayout(recursionDepth), nil
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
	switch {
	case id == itemShowHide:
		if m.toggle != nil {
			go m.toggle()
		}
	case id == itemQuit:
		if m.quit != nil {
			go m.quit()
		}
	case id >= firstTabItem:
		if t, ok := m.tabForID(id); ok && m.showTab != nil {
			go m.showTab(t.ID)
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
		items = append(items, groupEntry(e))
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
