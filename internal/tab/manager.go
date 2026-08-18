package tab

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"dashboard/internal/atomicwrite"
	"dashboard/internal/paths"
)

type TabManager struct {
	mu     sync.Mutex
	path   string
	tabs   []Tab
	nextID int
}

type Tab struct {
	ID       int                    `json:"id"`
	Title    string                 `json:"title"`
	URL      string                 `json:"url"`
	Icon     string                 `json:"icon,omitempty"`
	Notes    string                 `json:"notes,omitempty"`
	Settings map[string]interface{} `json:"settings,omitempty"`
}

// NewTabManagerAt opens the tab store at an explicit path.
func NewTabManagerAt(path string) *TabManager {
	tm := &TabManager{path: path}
	tm.load()
	return tm
}

// NewTabManager opens the tab store using the portable data directory.
func NewTabManager() *TabManager {
	return NewTabManagerAt(filepath.Join(paths.DataDir(), "tabs.json"))
}

func (tm *TabManager) load() {
	data, err := os.ReadFile(tm.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &tm.tabs)
	for _, t := range tm.tabs {
		if t.ID >= tm.nextID {
			tm.nextID = t.ID + 1
		}
	}
}

func (tm *TabManager) save() {
	data, _ := json.Marshal(tm.tabs)
	_ = atomicwrite.Write(tm.path, data, 0o644)
}

func (tm *TabManager) List() []Tab {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	out := make([]Tab, len(tm.tabs))
	copy(out, tm.tabs)
	return out
}

// Get returns the tab with the given id.
func (tm *TabManager) Get(id int) (Tab, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for _, t := range tm.tabs {
		if t.ID == id {
			return t, true
		}
	}
	return Tab{}, false
}

func (tm *TabManager) Add(url, title string) Tab {
	return tm.AddWithIcon(url, title, "")
}

func (tm *TabManager) AddWithIcon(url, title, icon string) Tab {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if title == "" {
		title = url
	}
	tab := Tab{ID: tm.nextID, Title: title, URL: url, Icon: icon}
	tm.nextID++
	tm.tabs = append(tm.tabs, tab)
	tm.save()
	return tab
}

func (tm *TabManager) Update(id int, url, title, icon string) (Tab, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for i, t := range tm.tabs {
		if t.ID == id {
			if title == "" {
				title = t.Title
			}
			if url == "" {
				url = t.URL
			}
			tm.tabs[i] = Tab{ID: t.ID, Title: title, URL: url, Icon: icon, Notes: t.Notes, Settings: t.Settings}
			tm.save()
			return tm.tabs[i], true
		}
	}
	return Tab{}, false
}

func (tm *TabManager) Remove(id int) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for i, t := range tm.tabs {
		if t.ID == id {
			tm.tabs = append(tm.tabs[:i], tm.tabs[i+1:]...)
			tm.save()
			return true
		}
	}
	return false
}

// SetSettings replaces the per-tab display settings (zoom, toolbar, ...).
func (tm *TabManager) SetSettings(id int, settings map[string]interface{}) (Tab, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for i, t := range tm.tabs {
		if t.ID == id {
			t.Settings = settings
			tm.tabs[i] = t
			tm.save()
			return tm.tabs[i], true
		}
	}
	return Tab{}, false
}

// SetNotes sets the persistent notes of the tab identified by id.
func (tm *TabManager) SetNotes(id int, notes string) (Tab, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for i, t := range tm.tabs {
		if t.ID == id {
			t.Notes = notes
			tm.tabs[i] = t
			tm.save()
			return tm.tabs[i], true
		}
	}
	return Tab{}, false
}

// Reorder reorders the tabs to match the order of the provided ids.
// Returns false if ids is empty or does not contain every current tab id.
func (tm *TabManager) Reorder(ids []int) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if len(ids) != len(tm.tabs) || len(ids) == 0 {
		return false
	}

	seen := make(map[int]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return false
		}
		seen[id] = true
	}

	byID := make(map[int]Tab, len(tm.tabs))
	for _, t := range tm.tabs {
		if !seen[t.ID] {
			return false
		}
		byID[t.ID] = t
	}

	ordered := make([]Tab, 0, len(ids))
	for _, id := range ids {
		ordered = append(ordered, byID[id])
	}
	tm.tabs = ordered
	tm.save()
	return true
}

func (tm *TabManager) RemoveByURL(url string) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for i, t := range tm.tabs {
		if t.URL == url {
			tm.tabs = append(tm.tabs[:i], tm.tabs[i+1:]...)
			tm.save()
			return true
		}
	}
	return false
}
