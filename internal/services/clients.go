package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"dashboard/internal/config"
	"dashboard/internal/models"
)

type HTTPClient struct {
	client *http.Client
	config config.ServiceConfig
}

func NewHTTPClient(cfg config.ServiceConfig) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		},
		config: cfg,
	}
}

func (c *HTTPClient) buildURL(path string) string {
	base := c.config.BaseURL
	if c.config.BackofficeURL != "" && path != "" {
		base = c.config.BackofficeURL
	}
	u, _ := url.Parse(base)
	joined := strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(c.config.APIPrefix+"/"+path, "/")
	if idx := strings.IndexByte(joined, '?'); idx >= 0 {
		u.Path = joined[:idx]
		u.RawQuery = joined[idx+1:]
	} else {
		u.Path = joined
	}
	return u.String()
}

func (c *HTTPClient) doRequest(ctx context.Context, method, path string, body interface{}, headers map[string]string) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.buildURL(path), bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	c.addAuth(req)

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return c.client.Do(req)
}

func (c *HTTPClient) addAuth(req *http.Request) {
	switch c.config.Auth.Type {
	case "basic":
		username := os.Getenv(c.config.Auth.UsernameEnv)
		password := os.Getenv(c.config.Auth.PasswordEnv)
		if username != "" && password != "" {
			req.SetBasicAuth(username, password)
		}
	case "bearer":
		token := os.Getenv(c.config.Auth.TokenEnv)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
}

func (c *HTTPClient) Get(ctx context.Context, path string, target interface{}) error {
	resp, err := c.doRequest(ctx, "GET", path, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return c.handleResponse(resp, target)
}

func (c *HTTPClient) Post(ctx context.Context, path string, body, target interface{}) error {
	resp, err := c.doRequest(ctx, "POST", path, body, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return c.handleResponse(resp, target)
}

func (c *HTTPClient) handleResponse(resp *http.Response, target interface{}) error {
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	if target == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

type NeuroNetClient struct {
	*HTTPClient
}

func NewNeuroNetClient(cfg config.ServiceConfig) *NeuroNetClient {
	return &NeuroNetClient{HTTPClient: NewHTTPClient(cfg)}
}

func (c *NeuroNetClient) Health(ctx context.Context) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := c.Get(ctx, c.config.Endpoints.Health, &result)
	return result, err
}

func (c *NeuroNetClient) ListModels(ctx context.Context) ([]models.NeuroNetModel, error) {
	var result []models.NeuroNetModel
	err := c.Get(ctx, c.config.Endpoints.Models, &result)
	return result, err
}

func (c *NeuroNetClient) ListTrainingJobs(ctx context.Context) ([]models.NeuroNetTrainingJob, error) {
	var result []models.NeuroNetTrainingJob
	err := c.Get(ctx, c.config.Endpoints.Training, &result)
	return result, err
}

func (c *NeuroNetClient) Inference(ctx context.Context, modelID string, input map[string]interface{}) (map[string]interface{}, error) {
	path := c.config.Endpoints.Inference + "/" + modelID
	var result map[string]interface{}
	err := c.Post(ctx, path, input, &result)
	return result, err
}

type MinecraftClient struct {
	*HTTPClient
}

func NewMinecraftClient(cfg config.ServiceConfig) *MinecraftClient {
	return &MinecraftClient{HTTPClient: NewHTTPClient(cfg)}
}

func (c *MinecraftClient) Status(ctx context.Context) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := c.Get(ctx, c.config.Endpoints.Status, &result)
	return result, err
}

func (c *MinecraftClient) ListServers(ctx context.Context) ([]models.MinecraftServer, error) {
	var result []models.MinecraftServer
	err := c.Get(ctx, c.config.Endpoints.Servers, &result)
	return result, err
}

func (c *MinecraftClient) ListPlayers(ctx context.Context) ([]models.MinecraftPlayer, error) {
	var result []models.MinecraftPlayer
	err := c.Get(ctx, c.config.Endpoints.Players, &result)
	return result, err
}

func (c *MinecraftClient) SendConsoleCommand(ctx context.Context, serverID, command string) error {
	path := c.config.Endpoints.Console + "/" + serverID
	return c.Post(ctx, path, map[string]string{"command": command}, nil)
}

type SlotBuilderClient struct {
	*HTTPClient
}

func NewSlotBuilderClient(cfg config.ServiceConfig) *SlotBuilderClient {
	return &SlotBuilderClient{HTTPClient: NewHTTPClient(cfg)}
}

func (c *SlotBuilderClient) ListGames(ctx context.Context) ([]models.SlotBuilderGame, error) {
	var result []models.SlotBuilderGame
	err := c.Get(ctx, c.config.Endpoints.Games, &result)
	return result, err
}

func (c *SlotBuilderClient) GetAnalytics(ctx context.Context, gameID string, from, to time.Time) ([]models.SlotBuilderAnalytics, error) {
	path := c.config.Endpoints.Analytics + "/" + gameID
	params := url.Values{}
	params.Set("from", from.Format(time.RFC3339))
	params.Set("to", to.Format(time.RFC3339))
	
	var result []models.SlotBuilderAnalytics
	err := c.Get(ctx, path+"?"+params.Encode(), &result)
	return result, err
}

func (c *SlotBuilderClient) ListDeployments(ctx context.Context) ([]models.SlotBuilderDeployment, error) {
	var result []models.SlotBuilderDeployment
	err := c.Get(ctx, c.config.Endpoints.Deployments, &result)
	return result, err
}

func (c *SlotBuilderClient) GetConfig(ctx context.Context, gameID string) (map[string]interface{}, error) {
	path := c.config.Endpoints.Config + "/" + gameID
	var result map[string]interface{}
	err := c.Get(ctx, path, &result)
	return result, err
}