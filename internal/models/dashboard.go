package models

import "time"

type ServiceStatus struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Healthy   bool                   `json:"healthy"`
	LastCheck time.Time              `json:"last_check"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

type NeuroNetModel struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Status      string                 `json:"status"`
	Accuracy    float64                `json:"accuracy,omitempty"`
	LastTrained time.Time              `json:"last_trained,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type NeuroNetTrainingJob struct {
	ID        string    `json:"id"`
	ModelID   string    `json:"model_id"`
	Status    string    `json:"status"`
	Progress  float64   `json:"progress"`
	StartedAt time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Metrics   map[string]float64 `json:"metrics,omitempty"`
}

type MinecraftServer struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Status   string `json:"status"`
	Online   int    `json:"online"`
	MaxPlayers int  `json:"max_players"`
	Version  string `json:"version"`
	MOTD     string `json:"motd"`
}

type MinecraftPlayer struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Server   string `json:"server"`
	Online   bool   `json:"online"`
	LastSeen time.Time `json:"last_seen"`
}

type SlotBuilderGame struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Version     string  `json:"version"`
	Status      string  `json:"status"`
	RTP         float64 `json:"rtp,omitempty"`
	Volatility  string  `json:"volatility,omitempty"`
	LastDeploy  time.Time `json:"last_deploy,omitempty"`
}

type SlotBuilderAnalytics struct {
	GameID       string    `json:"game_id"`
	Date         time.Time `json:"date"`
	Spins        int64     `json:"spins"`
	TotalBet     float64   `json:"total_bet"`
	TotalWin     float64   `json:"total_win"`
	RTP          float64   `json:"rtp"`
	UniquePlayers int      `json:"unique_players"`
}

type SlotBuilderDeployment struct {
	ID          string    `json:"id"`
	GameID      string    `json:"game_id"`
	Environment string    `json:"environment"`
	Version     string    `json:"version"`
	Status      string    `json:"status"`
	DeployedAt  time.Time `json:"deployed_at"`
	DeployedBy  string    `json:"deployed_by"`
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

type DashboardData struct {
	Services   []ServiceStatus      `json:"services"`
	NeuroNet   *NeuroNetDashboard   `json:"neuronet,omitempty"`
	Minecraft  *MinecraftDashboard  `json:"minecraft,omitempty"`
	SlotBuilder *SlotBuilderDashboard `json:"slotbuilder,omitempty"`
}

type NeuroNetDashboard struct {
	Models    []NeuroNetModel        `json:"models"`
	Training  []NeuroNetTrainingJob  `json:"training"`
	Health    map[string]interface{} `json:"health"`
}

type MinecraftDashboard struct {
	Servers []MinecraftServer `json:"servers"`
	Players []MinecraftPlayer `json:"players"`
	Status  map[string]interface{} `json:"status"`
}

type SlotBuilderDashboard struct {
	Games       []SlotBuilderGame         `json:"games"`
	Analytics   []SlotBuilderAnalytics    `json:"analytics"`
	Deployments []SlotBuilderDeployment   `json:"deployments"`
}