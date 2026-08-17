package config

import (
	"os"
	"path/filepath"

	"dashboard/internal/paths"

	"gopkg.in/yaml.v3"
)

type AuthConfig struct {
	Type        string `yaml:"type"`
	UsernameEnv string `yaml:"username_env,omitempty"`
	PasswordEnv string `yaml:"password_env,omitempty"`
	TokenEnv    string `yaml:"token_env,omitempty"`
}

type ServiceEndpoint struct {
	Health      string `yaml:"health,omitempty"`
	Models      string `yaml:"models,omitempty"`
	Training    string `yaml:"training,omitempty"`
	Inference   string `yaml:"inference,omitempty"`
	Status      string `yaml:"status,omitempty"`
	Servers     string `yaml:"servers,omitempty"`
	Players     string `yaml:"players,omitempty"`
	Console     string `yaml:"console,omitempty"`
	Games       string `yaml:"games,omitempty"`
	Analytics   string `yaml:"analytics,omitempty"`
	Deployments string `yaml:"deployments,omitempty"`
	Config      string `yaml:"config,omitempty"`
}

type TerminalConfig struct {
	Enabled     bool   `yaml:"enabled,omitempty"`
	Host        string `yaml:"host,omitempty"`
	Port        int    `yaml:"port,omitempty"`
	User        string `yaml:"user,omitempty"`
	Auth        string `yaml:"auth,omitempty"`        // password | key | agent (default: agent)
	PasswordEnv string `yaml:"password_env,omitempty"` // env var name holding the SSH password
	KeyPath     string `yaml:"key_path,omitempty"`     // path to the private key file
	Dir         string `yaml:"dir,omitempty"`          // local working dir for the shell fallback
	Split       string `yaml:"split,omitempty"`        // "v" terminal below the page | "h" terminal to the right (default: "")
}

type ServiceConfig struct {
	Name           string          `yaml:"name"`
	BaseURL        string          `yaml:"base_url"`
	BackofficeURL  string          `yaml:"backoffice_url,omitempty"`
	FrontendURL    string          `yaml:"frontend_url,omitempty"`
	AdminPath      string          `yaml:"admin_path,omitempty"`
	APIPrefix      string          `yaml:"api_prefix"`
	Auth           AuthConfig      `yaml:"auth"`
	Terminal       TerminalConfig  `yaml:"terminal,omitempty"`
	TimeoutSeconds int             `yaml:"timeout_seconds"`
	ProxyEnabled   bool            `yaml:"proxy_enabled"`
	Endpoints      ServiceEndpoint `yaml:"endpoints"`
}

type ProxyConfig struct {
	Enabled          bool     `yaml:"enabled"`
	AllowedHosts     []string `yaml:"allowed_hosts"`
	TimeoutSeconds   int      `yaml:"timeout_seconds"`
	MaxBodySizeMB    int      `yaml:"max_body_size_mb"`
}

type UIConfig struct {
	Theme            string   `yaml:"theme"`
	DefaultTab       string   `yaml:"default_tab"`
	WebviewGpuPolicy string   `yaml:"webview_gpu_policy,omitempty"` // always|ondemand|never (Wails Linux)
	Tabs             []UITab  `yaml:"tabs"`
}

type UITab struct {
	ID      string `yaml:"id"`
	Label   string `yaml:"label"`
	Icon    string `yaml:"icon"`
	Enabled bool   `yaml:"enabled"`
}

type AppConfig struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Debug   bool   `yaml:"debug"`
}

type Config struct {
	App      AppConfig                `yaml:"app"`
	Services map[string]ServiceConfig `yaml:"services"`
	Proxy    ProxyConfig              `yaml:"proxy"`
	UI       UIConfig                 `yaml:"ui"`
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = findConfigFile()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			if err := cfg.Save(path); err != nil {
				return nil, err
			}
			return cfg, nil
		}
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func DefaultConfig() *Config {
	return &Config{
		App: AppConfig{
			Name:    "Dashboard",
			Version: "1.0.0",
			Debug:   true,
		},
		Services: map[string]ServiceConfig{
			"neuronet": {
				Name:          "NeuroNet",
				BaseURL:       "http://localhost:8000",
				AdminPath:     "/admin",
				APIPrefix:     "/api",
				Auth:          AuthConfig{Type: "none"},
				Terminal:      TerminalConfig{Enabled: true, Host: "localhost", Port: 22, User: "root", Auth: "agent"},
				TimeoutSeconds: 30,
				ProxyEnabled:  true,
				Endpoints: ServiceEndpoint{
					Health:    "/health",
					Models:    "/models",
					Training:  "/training",
					Inference: "/inference",
				},
			},
			"minecraft": {
				Name:          "Minecraft Network",
				BaseURL:       "http://51.75.77.248:9800",
				APIPrefix:     "/api",
				Auth:          AuthConfig{Type: "basic", UsernameEnv: "MINECRAFT_USER", PasswordEnv: "MINECRAFT_PASS"},
				Terminal:      TerminalConfig{Enabled: true, Host: "51.75.77.248", Port: 22, User: "root", Auth: "agent"},
				TimeoutSeconds: 15,
				ProxyEnabled:  true,
				Endpoints: ServiceEndpoint{
					Status:  "/status",
					Servers: "/servers",
					Players: "/players",
					Console: "/console",
				},
			},
			"slotbuilder": {
				Name:           "SlotBuilder",
				BackofficeURL:  "https://backoffice.7casinogames.com",
				FrontendURL:    "https://7casinogames.com",
				APIPrefix:      "/api",
				Auth:           AuthConfig{Type: "bearer", TokenEnv: "SLOTBUILDER_TOKEN"},
				Terminal:       TerminalConfig{Enabled: true, Host: "backoffice.7casinogames.com", Port: 22, User: "root", Auth: "agent"},
				TimeoutSeconds: 30,
				ProxyEnabled:   true,
				Endpoints: ServiceEndpoint{
					Games:       "/games",
					Analytics:   "/analytics",
					Deployments: "/deployments",
					Config:      "/config",
				},
			},
		},
		Proxy: ProxyConfig{
			Enabled:        true,
			AllowedHosts:   []string{"localhost:8000", "51.75.77.248:9800", "backoffice.7casinogames.com", "7casinogames.com"},
			TimeoutSeconds: 60,
			MaxBodySizeMB:  50,
		},
		UI: UIConfig{
			Theme: "system",
			DefaultTab: "neuronet",
			WebviewGpuPolicy: "always",
			Tabs: []UITab{
				{ID: "neuronet", Label: "NeuroNet", Icon: "brain", Enabled: true},
				{ID: "minecraft", Label: "Minecraft", Icon: "server", Enabled: true},
				{ID: "slotbuilder", Label: "SlotBuilder", Icon: "gamepad", Enabled: true},
			},
		},
	}
}

func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func findConfigFile() string {
	candidates := []string{
		filepath.Join(paths.DataDir(), "config.yaml"),
		"config.yaml",
		"config.yml",
		filepath.Join(".", "config.yaml"),
		filepath.Join(".", "config.yml"),
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exe), "config.yaml"),
			filepath.Join(filepath.Dir(exe), "config.yml"),
		)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return filepath.Join(paths.DataDir(), "config.yaml")
}

func (c *Config) GetService(key string) (ServiceConfig, bool) {
	svc, ok := c.Services[key]
	return svc, ok
}

func (c *Config) EnabledTabs() []UITab {
	var tabs []UITab
	for _, t := range c.UI.Tabs {
		if t.Enabled {
			tabs = append(tabs, t)
		}
	}
	return tabs
}