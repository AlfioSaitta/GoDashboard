package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"dashboard/internal/config"
	"dashboard/internal/cookie"
	"dashboard/internal/models"
)

type ProxyService struct {
	config    *config.Config
	proxies   map[string]*httputil.ReverseProxy
	client    *http.Client
	cookies   *cookie.Store
}

func NewProxyService(cfg *config.Config) *ProxyService {
	ps := &ProxyService{
		config: cfg,
		proxies: make(map[string]*httputil.ReverseProxy),
		cookies: cookie.New(),
	}
	ps.rebuild(cfg)
	return ps
}

// Reconfigure re-applies a (possibly updated) config: it rebuilds the reverse
// proxies and client for the new hosts/timeouts while KEEPING the persistent
// cookie store, so sessions captured so far survive a settings change.
func (ps *ProxyService) Reconfigure(cfg *config.Config) {
	ps.config = cfg
	ps.rebuild(cfg)
}

func (ps *ProxyService) rebuild(cfg *config.Config) {
	ps.client = &http.Client{
		Timeout: time.Duration(cfg.Proxy.TimeoutSeconds) * time.Second,
	}
	ps.proxies = make(map[string]*httputil.ReverseProxy)
	for _, host := range cfg.Proxy.AllowedHosts {
		target, _ := url.Parse("http://" + host)
		if strings.HasPrefix(host, "https://") || strings.HasSuffix(host, "443") {
			target.Scheme = "https"
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.Transport = &customTransport{client: ps.client}
		proxy.ModifyResponse = ps.modifyResponse
		proxy.ErrorHandler = ps.errorHandler
		ps.proxies[host] = proxy
	}
}

type customTransport struct {
	client *http.Client
}

func (t *customTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Del("Origin")
	req.Header.Del("Referer")
	return t.client.Do(req)
}

// injectCookies adds the stored jar cookies for host/path to the request's
// Cookie header (unless the caller already provided one).
func (ps *ProxyService) injectCookies(req *http.Request, host string) {
	if req.Header.Get("Cookie") != "" {
		return
	}
	val := ps.cookies.HeaderValue(host, req.URL.Path)
	if val != "" {
		req.Header.Set("Cookie", val)
	}
}

// captureCookies persists any Set-Cookie headers so future proxied requests
// (and the WebView iframe panels) can reuse the session.
func (ps *ProxyService) captureCookies(resp *http.Response, host string) {
	if len(resp.Cookies()) == 0 {
		return
	}
	for _, c := range resp.Cookies() {
		exp := ""
		if !c.Expires.IsZero() {
			exp = c.Expires.Format(time.RFC3339)
		}
		domain := c.Domain
		if domain == "" {
			domain = host
		}
		ps.cookies.Set(cookie.Cookie{
			Domain:   domain,
			Path:     c.Path,
			Name:     c.Name,
			Value:    c.Value,
			Secure:   c.Secure,
			HttpOnly: c.HttpOnly,
			Expires:  exp,
		})
	}
}

func (ps *ProxyService) modifyResponse(resp *http.Response) error {
	resp.Header.Del("X-Frame-Options")
	resp.Header.Del("Content-Security-Policy")
	resp.Header.Set("Access-Control-Allow-Origin", "*")
	resp.Header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	resp.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Cookie")

	if resp.Request != nil && resp.Request.URL != nil {
		ps.captureCookies(resp, resp.Request.URL.Host)
	}
	return nil
}

func (ps *ProxyService) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusBadGateway)
	fmt.Fprintf(w, `{"error": "proxy error: %s"}`, err.Error())
}

func (ps *ProxyService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	path := r.PathValue("path")

	svcCfg, ok := ps.config.GetService(service)
	if !ok || !svcCfg.ProxyEnabled {
		http.Error(w, "service not found or proxy disabled", http.StatusNotFound)
		return
	}

	host := ps.extractHost(svcCfg.BaseURL)
	if host == "" {
		host = ps.extractHost(svcCfg.BackofficeURL)
	}
	proxy, ok := ps.proxies[host]
	if !ok {
		http.Error(w, "proxy not configured for host", http.StatusInternalServerError)
		return
	}

	r.URL.Path = svcCfg.APIPrefix + "/" + path
	r.Host = host

	// Inject persisted jar cookies for the upstream host/path.
	ps.injectCookies(r, host)

	w.Header().Set("Access-Control-Allow-Origin", "*")
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	proxy.ServeHTTP(w, r)
}

func (ps *ProxyService) extractHost(baseURL string) string {
	u, _ := url.Parse(baseURL)
	return u.Host
}

func (ps *ProxyService) ProxyRequest(ctx context.Context, req models.ProxyRequest) (*models.ProxyResponse, error) {
	svcCfg, ok := ps.config.GetService(req.Service)
	if !ok || !svcCfg.ProxyEnabled {
		return nil, fmt.Errorf("service not found or proxy disabled")
	}
	
	start := time.Now()
	base := svcCfg.BaseURL
	if base == "" {
		base = svcCfg.BackofficeURL
	}
	targetURL := strings.TrimRight(base, "/") + svcCfg.APIPrefix + "/" + strings.TrimLeft(req.Path, "/")
	
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, targetURL, strings.NewReader(req.Body))
	if err != nil {
		return nil, err
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	// Inject persisted jar cookies for the upstream host/path.
	u, _ := url.Parse(targetURL)
	ps.injectCookies(httpReq, u.Host)

	if svcCfg.Auth.Type == "basic" {
		user := os.Getenv(svcCfg.Auth.UsernameEnv)
		pass := os.Getenv(svcCfg.Auth.PasswordEnv)
		if user != "" && pass != "" {
			httpReq.SetBasicAuth(user, pass)
		}
	} else if svcCfg.Auth.Type == "bearer" {
		token := os.Getenv(svcCfg.Auth.TokenEnv)
		if token != "" {
			httpReq.Header.Set("Authorization", "Bearer "+token)
		}
	}
	
	resp, err := ps.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Persist any Set-Cookie headers from the upstream response.
	if u, uerr := url.Parse(targetURL); uerr == nil {
		ps.captureCookies(resp, u.Host)
	}

	body, _ := io.ReadAll(resp.Body)
	
	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	
	return &models.ProxyResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       string(body),
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}