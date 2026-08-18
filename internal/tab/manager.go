package tab

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dashboard/internal/atomicwrite"
	"dashboard/internal/paths"
)

type TabManager struct {
	mu     sync.Mutex
	path   string
	tabs   []Tab
	nextID int
}

// Note is a single persistent note attached to a tab. A tab can own many notes;
// each note has its own id (unique within the tab), title, content and timestamps.
type Note struct {
	ID        int    `json:"id"`
	Title     string `json:"title,omitempty"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type Tab struct {
	ID       int                    `json:"id"`
	Title    string                 `json:"title"`
	URL      string                 `json:"url"`
	Icon     string                 `json:"icon,omitempty"`
	Notes    []Note                 `json:"notes,omitempty"`
	Settings map[string]interface{} `json:"settings,omitempty"`
}

// legacyTab mirrors the on-disk shape so notes written by older versions
// (a plain string) can be migrated into the multi-note list.
type legacyTab struct {
	ID       int                    `json:"id"`
	Title    string                 `json:"title"`
	URL      string                 `json:"url"`
	Icon     string                 `json:"icon,omitempty"`
	Notes    json.RawMessage        `json:"notes,omitempty"`
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
	var legacy []legacyTab
	if err := json.Unmarshal(data, &legacy); err != nil {
		return
	}
	migrated := false
	tm.tabs = make([]Tab, 0, len(legacy))
	for _, lt := range legacy {
		t := Tab{ID: lt.ID, Title: lt.Title, URL: lt.URL, Icon: lt.Icon, Settings: lt.Settings}
		if len(lt.Notes) > 0 {
			var list []Note
			if err := json.Unmarshal(lt.Notes, &list); err == nil {
				t.Notes = list
			} else {
				// Legacy format: notes was a plain string.
				var s string
				if err := json.Unmarshal(lt.Notes, &s); err == nil && s != "" {
					now := time.Now().UTC().Format(time.RFC3339)
					t.Notes = []Note{{ID: 1, Title: "Nota", Content: s, CreatedAt: now, UpdatedAt: now}}
				}
				migrated = true
			}
		}
		tm.tabs = append(tm.tabs, t)
		if t.ID >= tm.nextID {
			tm.nextID = t.ID + 1
		}
	}
	if migrated {
		tm.save()
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

// NoteList returns a copy of the notes attached to the tab identified by id.
func (tm *TabManager) NoteList(id int) ([]Note, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for _, t := range tm.tabs {
		if t.ID == id {
			out := make([]Note, len(t.Notes))
			copy(out, t.Notes)
			return out, true
		}
	}
	return nil, false
}

// AddNote appends a new note to the tab identified by id. A fresh id is
// assigned and both timestamps are set. Returns the updated tab.
func (tm *TabManager) AddNote(id int, note Note) (Tab, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for i, t := range tm.tabs {
		if t.ID == id {
			next := 1
			for _, n := range t.Notes {
				if n.ID >= next {
					next = n.ID + 1
				}
			}
			now := time.Now().UTC().Format(time.RFC3339)
			note.ID = next
			note.CreatedAt = now
			note.UpdatedAt = now
			t.Notes = append(t.Notes, note)
			tm.tabs[i] = t
			tm.save()
			return tm.tabs[i], true
		}
	}
	return Tab{}, false
}

// UpdateNote replaces the content of an existing note (matched by id) on the
// tab identified by id and bumps its updated timestamp. Returns the updated tab.
func (tm *TabManager) UpdateNote(id int, note Note) (Tab, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for i, t := range tm.tabs {
		if t.ID == id {
			for j, n := range t.Notes {
				if n.ID == note.ID {
					now := time.Now().UTC().Format(time.RFC3339)
					note.CreatedAt = n.CreatedAt
					note.UpdatedAt = now
					t.Notes[j] = note
					tm.tabs[i] = t
					tm.save()
					return tm.tabs[i], true
				}
			}
			return Tab{}, false
		}
	}
	return Tab{}, false
}

// DeleteNote removes the note with the given id from the tab identified by id.
// Returns the updated tab.
func (tm *TabManager) DeleteNote(id, noteID int) (Tab, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for i, t := range tm.tabs {
		if t.ID == id {
			for j, n := range t.Notes {
				if n.ID == noteID {
					t.Notes = append(t.Notes[:j], t.Notes[j+1:]...)
					tm.tabs[i] = t
					tm.save()
					return tm.tabs[i], true
				}
			}
			return Tab{}, false
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
