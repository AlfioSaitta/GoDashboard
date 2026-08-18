package api

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"dashboard/internal/models"
	"dashboard/internal/services"
	"dashboard/internal/tab"
)

type DashboardAPI struct {
	manager *services.ServiceManager
}

func NewDashboardAPI(manager *services.ServiceManager) *DashboardAPI {
	return &DashboardAPI{manager: manager}
}

func (d *DashboardAPI) GetServicesStatus(ctx context.Context) []ServiceStatus {
	statuses := d.manager.CheckAllHealth(ctx)
	result := make([]ServiceStatus, len(statuses))
	for i, s := range statuses {
		result[i] = toServiceStatus(s)
	}
	return result
}

type TabAPI struct {
	manager *tab.TabManager
}

func NewTabAPI(manager *tab.TabManager) *TabAPI {
	return &TabAPI{manager: manager}
}

func (t *TabAPI) ListTabs(ctx context.Context) ([]TabInfo, error) {
	tabs := t.manager.List()
	if len(tabs) == 0 {
		// Initialize with default tabs
		defaults := []tab.Tab{
			{ID: 1, Title: "NeuroNet", URL: "neuronet", Icon: "brain"},
			{ID: 2, Title: "Minecraft", URL: "minecraft", Icon: "server"},
			{ID: 3, Title: "SlotBuilder", URL: "slotbuilder", Icon: "gamepad"},
		}
		for _, d := range defaults {
			t.manager.AddWithIcon(d.URL, d.Title, d.Icon)
		}
		tabs = t.manager.List()
		log.Printf("TabAPI.ListTabs: seeded %d default tabs", len(tabs))
	}
	result := make([]TabInfo, len(tabs))
	for i, tb := range tabs {
		result[i] = TabInfo{ID: tb.ID, Label: tb.Title, Icon: tb.Icon, URL: tb.URL, Notes: tb.Notes, Settings: tb.Settings}
	}
	return result, nil
}

// UpdateTab updates the label, url and icon of the tab identified by id.
// The id is passed as a string (matching RemoveTab) and parsed as an integer.
func (t *TabAPI) UpdateTab(ctx context.Context, id string, config map[string]interface{}) (Tab, error) {
	intID, err := strconv.Atoi(id)
	if err != nil {
		return Tab{}, fmt.Errorf("invalid tab id %q", id)
	}
	tab, ok := t.manager.Update(intID, cfgString(config, "url"), cfgString(config, "label"), cfgString(config, "icon"))
	if !ok {
		return Tab{}, fmt.Errorf("tab %d not found", intID)
	}
	log.Printf("TabAPI.UpdateTab: updated tab %d", intID)
	return Tab{ID: tab.ID, Title: tab.Title, URL: tab.URL, Icon: tab.Icon, Notes: tab.Notes, Settings: tab.Settings}, nil
}

// UpdateTabSettings replaces the per-tab display settings (zoom, toolbar, ...).
func (t *TabAPI) UpdateTabSettings(ctx context.Context, id string, settings map[string]interface{}) (Tab, error) {
	intID, err := strconv.Atoi(id)
	if err != nil {
		return Tab{}, fmt.Errorf("invalid tab id %q", id)
	}
	if settings == nil {
		settings = map[string]interface{}{}
	}
	tb, ok := t.manager.SetSettings(intID, settings)
	if !ok {
		return Tab{}, fmt.Errorf("tab %d not found", intID)
	}
	log.Printf("TabAPI.UpdateTabSettings: tab %d settings updated", intID)
	return Tab{ID: tb.ID, Title: tb.Title, URL: tb.URL, Icon: tb.Icon, Notes: tb.Notes, Settings: tb.Settings}, nil
}

// GetNotes returns the persistent notes of the tab identified by id.
func (t *TabAPI) GetNotes(ctx context.Context, id string) (string, error) {
	intID, err := strconv.Atoi(id)
	if err != nil {
		return "", fmt.Errorf("invalid tab id %q", id)
	}
	tb, ok := t.manager.Get(intID)
	if !ok {
		return "", fmt.Errorf("tab %d not found", intID)
	}
	return tb.Notes, nil
}

// SaveNotes sets the persistent notes of the tab identified by id.
func (t *TabAPI) SaveNotes(ctx context.Context, id string, notes string) (Tab, error) {
	intID, err := strconv.Atoi(id)
	if err != nil {
		return Tab{}, fmt.Errorf("invalid tab id %q", id)
	}
	tb, ok := t.manager.SetNotes(intID, notes)
	if !ok {
		return Tab{}, fmt.Errorf("tab %d not found", intID)
	}
	log.Printf("TabAPI.SaveNotes: tab %d notes saved (%d bytes)", intID, len(notes))
	return Tab{ID: tb.ID, Title: tb.Title, URL: tb.URL, Icon: tb.Icon, Notes: tb.Notes, Settings: tb.Settings}, nil
}

// ReorderTabs reorders tabs to match the order of the provided ids.
func (t *TabAPI) ReorderTabs(ctx context.Context, ids []int) error {
	if !t.manager.Reorder(ids) {
		return fmt.Errorf("reorder failed: ids must contain every tab exactly once")
	}
	log.Printf("TabAPI.ReorderTabs: reordered %d tabs", len(ids))
	return nil
}

func cfgString(config map[string]interface{}, key string) string {
	if v, ok := config[key].(string); ok {
		return v
	}
	return ""
}

// Exported types for Wails binding
type TabInfo struct {
	ID       int                    `json:"id"`
	Label    string                 `json:"label"`
	Icon     string                 `json:"icon"`
	URL      string                 `json:"url"`
	Notes    string                 `json:"notes,omitempty"`
	Settings map[string]interface{} `json:"settings,omitempty"`
}

type Tab struct {
	ID       int                    `json:"id"`
	Title    string                 `json:"title"`
	URL      string                 `json:"url"`
	Icon     string                 `json:"icon,omitempty"`
	Notes    string                 `json:"notes,omitempty"`
	Settings map[string]interface{} `json:"settings,omitempty"`
}

type ServiceStatus struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Healthy   bool                   `json:"healthy"`
	LastCheck string                 `json:"last_check"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

func toServiceStatus(m models.ServiceStatus) ServiceStatus {
	return ServiceStatus{
		ID:        m.ID,
		Name:      m.Name,
		Healthy:   m.Healthy,
		LastCheck: m.LastCheck.Format(time.RFC3339),
		Details:   m.Details,
		Error:     m.Error,
	}
}
