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

type SlotBuilderClient struct {
	*HTTPClient
}

func NewSlotBuilderClient(cfg config.ServiceConfig) *SlotBuilderClient {
	return &SlotBuilderClient{HTTPClient: NewHTTPClient(cfg)}
}

func (c *SlotBuilderClient) ListGames(ctx context.Context) ([]map[string]interface{}, error) {
	var result []map[string]interface{}
	err := c.Get(ctx, c.config.Endpoints.Games, &result)
	return result, err
}
