package api

import (
	"context"
	"errors"
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
	proxy   *services.ProxyService
}

func NewDashboardAPI(manager *services.ServiceManager, proxy *services.ProxyService) *DashboardAPI {
	return &DashboardAPI{
		manager: manager,
		proxy:   proxy,
	}
}

func (d *DashboardAPI) GetDashboard(ctx context.Context) (*DashboardData, error) {
	data, err := d.manager.GetFullDashboard(ctx)
	if err != nil {
		return nil, err
	}
	return toDashboardData(data), nil
}

func (d *DashboardAPI) GetServicesStatus(ctx context.Context) ([]ServiceStatus, error) {
	statuses := d.manager.CheckAllHealth(ctx)
	result := make([]ServiceStatus, len(statuses))
	for i, s := range statuses {
		result[i] = toServiceStatus(s)
	}
	return result, nil
}

func (d *DashboardAPI) GetNeuroNetData(ctx context.Context) (*NeuroNetDashboard, error) {
	data, err := d.manager.GetNeuroNetDashboard(ctx)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return toNeuroNetDashboard(data), nil
}

func (d *DashboardAPI) GetMinecraftData(ctx context.Context) (*MinecraftDashboard, error) {
	data, err := d.manager.GetMinecraftDashboard(ctx)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return toMinecraftDashboard(data), nil
}

func (d *DashboardAPI) GetSlotBuilderData(ctx context.Context) (*SlotBuilderDashboard, error) {
	data, err := d.manager.GetSlotBuilderDashboard(ctx)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	return toSlotBuilderDashboard(data), nil
}

func (d *DashboardAPI) ProxyRequest(ctx context.Context, req ProxyRequest) (*ProxyResponse, error) {
	if d.proxy == nil {
		return nil, errors.New("proxy not enabled")
	}
	mreq := models.ProxyRequest{
		Service: req.Service,
		Path:    req.Path,
		Method:  req.Method,
		Headers: req.Headers,
		Body:    req.Body,
	}
	mresp, err := d.proxy.ProxyRequest(ctx, mreq)
	if err != nil {
		return nil, err
	}
	return toProxyResponse(mresp), nil
}

func (d *DashboardAPI) NeuroNetInference(ctx context.Context, modelID string, input map[string]interface{}) (map[string]interface{}, error) {
	return nil, errors.New("not implemented")
}

func (d *DashboardAPI) MinecraftConsoleCommand(ctx context.Context, serverID, command string) error {
	return errors.New("not implemented")
}

type TabAPI struct {
	manager *tab.TabManager
}

func NewTabAPI(manager *tab.TabManager) *TabAPI {
	return &TabAPI{manager: manager}
}

func (t *TabAPI) ListTabs(ctx context.Context) ([]TabInfo, error) {
	tabs := t.manager.List()
	log.Printf("TabAPI.ListTabs: found %d tabs", len(tabs))
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
		log.Printf("TabAPI.ListTabs: after init, found %d tabs", len(tabs))
	}
	result := make([]TabInfo, len(tabs))
	for i, tb := range tabs {
		result[i] = TabInfo{ID: tb.ID, Label: tb.Title, Icon: tb.Icon, URL: tb.URL, Settings: tb.Settings}
	}
	log.Printf("TabAPI.ListTabs: returning %d tabs", len(result))
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
	return Tab{ID: tab.ID, Title: tab.Title, URL: tab.URL, Icon: tab.Icon, Settings: tab.Settings}, nil
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
	return Tab{ID: tb.ID, Title: tb.Title, URL: tb.URL, Icon: tb.Icon, Settings: tb.Settings}, nil
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
	Settings map[string]interface{} `json:"settings,omitempty"`
}

type Tab struct {
	ID       int                    `json:"id"`
	Title    string                 `json:"title"`
	URL      string                 `json:"url"`
	Icon     string                 `json:"icon,omitempty"`
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

type DashboardData struct {
	Services    []ServiceStatus      `json:"services"`
	NeuroNet    *NeuroNetDashboard   `json:"neuronet,omitempty"`
	Minecraft   *MinecraftDashboard  `json:"minecraft,omitempty"`
	SlotBuilder *SlotBuilderDashboard `json:"slotbuilder,omitempty"`
}

type NeuroNetDashboard struct {
	Models   []NeuroNetModel        `json:"models"`
	Training []NeuroNetTrainingJob  `json:"training"`
	Health   map[string]interface{} `json:"health"`
}

type NeuroNetModel struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Status      string                 `json:"status"`
	Accuracy    float64                `json:"accuracy,omitempty"`
	LastTrained string                 `json:"last_trained,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type NeuroNetTrainingJob struct {
	ID        string             `json:"id"`
	ModelID   string             `json:"model_id"`
	Status    string             `json:"status"`
	Progress  float64            `json:"progress"`
	StartedAt string             `json:"started_at"`
	FinishedAt *string           `json:"finished_at,omitempty"`
	Metrics   map[string]float64 `json:"metrics,omitempty"`
}

type MinecraftDashboard struct {
	Servers []MinecraftServer `json:"servers"`
	Players []MinecraftPlayer `json:"players"`
	Status  map[string]interface{} `json:"status"`
}

type MinecraftServer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	Status    string `json:"status"`
	Online    int    `json:"online"`
	MaxPlayers int   `json:"max_players"`
	Version   string `json:"version"`
	MOTD      string `json:"motd"`
}

type MinecraftPlayer struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Server   string `json:"server"`
	Online   bool   `json:"online"`
	LastSeen string `json:"last_seen"`
}

type SlotBuilderDashboard struct {
	Games       []SlotBuilderGame         `json:"games"`
	Analytics   []SlotBuilderAnalytics    `json:"analytics"`
	Deployments []SlotBuilderDeployment   `json:"deployments"`
}

type SlotBuilderGame struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Version     string  `json:"version"`
	Status      string  `json:"status"`
	RTP         float64 `json:"rtp,omitempty"`
	Volatility  string  `json:"volatility,omitempty"`
	LastDeploy  string  `json:"last_deploy,omitempty"`
}

type SlotBuilderAnalytics struct {
	GameID        string  `json:"game_id"`
	Date          string  `json:"date"`
	Spins         int64   `json:"spins"`
	TotalBet      float64 `json:"total_bet"`
	TotalWin      float64 `json:"total_win"`
	RTP           float64 `json:"rtp"`
	UniquePlayers int     `json:"unique_players"`
}

type SlotBuilderDeployment struct {
	ID          string `json:"id"`
	GameID      string `json:"game_id"`
	Environment string `json:"environment"`
	Version     string `json:"version"`
	Status      string `json:"status"`
	DeployedAt  string `json:"deployed_at"`
	DeployedBy  string `json:"deployed_by"`
}

type ProxyRequest struct {
	Service string            `json:"service"`
	Path    string            `json:"path"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body,omitempty"`
}

type ProxyResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	DurationMs int64             `json:"duration_ms"`
}

// Conversion functions from internal models to API types
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

func toDashboardData(m *models.DashboardData) *DashboardData {
	if m == nil {
		return nil
	}
	services := make([]ServiceStatus, len(m.Services))
	for i, s := range m.Services {
		services[i] = toServiceStatus(s)
	}
	return &DashboardData{
		Services:    services,
		NeuroNet:    toNeuroNetDashboard(m.NeuroNet),
		Minecraft:   toMinecraftDashboard(m.Minecraft),
		SlotBuilder: toSlotBuilderDashboard(m.SlotBuilder),
	}
}

func toNeuroNetDashboard(m *models.NeuroNetDashboard) *NeuroNetDashboard {
	if m == nil {
		return nil
	}
	models := make([]NeuroNetModel, len(m.Models))
	for i, mod := range m.Models {
		models[i] = NeuroNetModel{
			ID:          mod.ID,
			Name:        mod.Name,
			Version:     mod.Version,
			Status:      mod.Status,
			Accuracy:    mod.Accuracy,
			LastTrained: mod.LastTrained.Format(time.RFC3339),
			Metadata:    mod.Metadata,
		}
	}
	training := make([]NeuroNetTrainingJob, len(m.Training))
	for i, t := range m.Training {
		var finishedAt *string
		if t.FinishedAt != nil {
			s := t.FinishedAt.Format(time.RFC3339)
			finishedAt = &s
		}
		training[i] = NeuroNetTrainingJob{
			ID:        t.ID,
			ModelID:   t.ModelID,
			Status:    t.Status,
			Progress:  t.Progress,
			StartedAt: t.StartedAt.Format(time.RFC3339),
			FinishedAt: finishedAt,
			Metrics:   t.Metrics,
		}
	}
	return &NeuroNetDashboard{
		Models:   models,
		Training: training,
		Health:   m.Health,
	}
}

func toMinecraftDashboard(m *models.MinecraftDashboard) *MinecraftDashboard {
	if m == nil {
		return nil
	}
	servers := make([]MinecraftServer, len(m.Servers))
	for i, s := range m.Servers {
		servers[i] = MinecraftServer{
			ID:         s.ID,
			Name:       s.Name,
			Address:    s.Address,
			Status:     s.Status,
			Online:     s.Online,
			MaxPlayers: s.MaxPlayers,
			Version:    s.Version,
			MOTD:       s.MOTD,
		}
	}
	players := make([]MinecraftPlayer, len(m.Players))
	for i, p := range m.Players {
		players[i] = MinecraftPlayer{
			UUID:     p.UUID,
			Name:     p.Name,
			Server:   p.Server,
			Online:   p.Online,
			LastSeen: p.LastSeen.Format(time.RFC3339),
		}
	}
	return &MinecraftDashboard{
		Servers: servers,
		Players: players,
		Status:  m.Status,
	}
}

func toSlotBuilderDashboard(m *models.SlotBuilderDashboard) *SlotBuilderDashboard {
	if m == nil {
		return nil
	}
	games := make([]SlotBuilderGame, len(m.Games))
	for i, g := range m.Games {
		games[i] = SlotBuilderGame{
			ID:         g.ID,
			Name:       g.Name,
			Version:    g.Version,
			Status:     g.Status,
			RTP:        g.RTP,
			Volatility: g.Volatility,
			LastDeploy: g.LastDeploy.Format(time.RFC3339),
		}
	}
	analytics := make([]SlotBuilderAnalytics, len(m.Analytics))
	for i, a := range m.Analytics {
		analytics[i] = SlotBuilderAnalytics{
			GameID:        a.GameID,
			Date:          a.Date.Format(time.RFC3339),
			Spins:         a.Spins,
			TotalBet:      a.TotalBet,
			TotalWin:      a.TotalWin,
			RTP:           a.RTP,
			UniquePlayers: a.UniquePlayers,
		}
	}
	deployments := make([]SlotBuilderDeployment, len(m.Deployments))
	for i, d := range m.Deployments {
		deployments[i] = SlotBuilderDeployment{
			ID:          d.ID,
			GameID:      d.GameID,
			Environment: d.Environment,
			Version:     d.Version,
			Status:      d.Status,
			DeployedAt:  d.DeployedAt.Format(time.RFC3339),
			DeployedBy:  d.DeployedBy,
		}
	}
	return &SlotBuilderDashboard{
		Games:       games,
		Analytics:   analytics,
		Deployments: deployments,
	}
}

func toProxyResponse(m *models.ProxyResponse) *ProxyResponse {
	return &ProxyResponse{
		StatusCode: m.StatusCode,
		Headers:    m.Headers,
		Body:       m.Body,
		DurationMs: m.DurationMs,
	}
}