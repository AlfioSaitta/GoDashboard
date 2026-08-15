// Package cookie provides a persistent cookie jar for per-domain cookies
// used by the dashboard panels and the CORS-bypass proxy.
package cookie

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dashboard/internal/paths"
)

// Cookie is a single cookie, persisted in <exedir>/data/cookies.json.
type Cookie struct {
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	Secure   bool   `json:"secure,omitempty"`
	HttpOnly bool   `json:"http_only,omitempty"`
	Expires  string `json:"expires,omitempty"` // RFC3339 or empty for session cookies
	Created  string `json:"created"`
}

// Store is a thread-safe persistent cookie jar.
type Store struct {
	mu    sync.Mutex
	path  string
	items map[string]*Cookie
}

// New opens (or creates) the cookie store in the portable data directory.
func New() *Store {
	return NewAt(filepath.Join(paths.DataDir(), "cookies.json"))
}

// NewAt opens the cookie store at an explicit path.
func NewAt(path string) *Store {
	s := &Store{
		path:  path,
		items: make(map[string]*Cookie),
	}
	s.load()
	return s
}

func key(domain, path, name string) string {
	return strings.ToLower(domain) + "\x00" + path + "\x00" + name
}

func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var list []Cookie
	if err := json.Unmarshal(data, &list); err != nil {
		return
	}
	for i := range list {
		c := &list[i]
		s.items[key(c.Domain, c.Path, c.Name)] = c
	}
}

func (s *Store) save() {
	dir := filepath.Dir(s.path)
	_ = os.MkdirAll(dir, 0o755)
	out := make([]Cookie, 0, len(s.items))
	for _, c := range s.items {
		out = append(out, *c)
	}
	data, _ := json.Marshal(out)
	_ = os.WriteFile(s.path, data, 0o644)
}

// List returns all cookies, optionally filtered by domain (case-insensitive
// suffix match). An empty domain returns everything, most recently created first.
func (s *Store) List(domain string) []Cookie {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []Cookie
	needle := strings.ToLower(strings.TrimSpace(domain))
	for _, c := range s.items {
		if needle != "" && !strings.HasSuffix(strings.ToLower(c.Domain), needle) {
			continue
		}
		out = append(out, *c)
	}
	return out
}

// Set stores (or replaces) a cookie, persisting immediately.
func (s *Store) Set(c Cookie) Cookie {
	s.mu.Lock()
	defer s.mu.Unlock()

	c.Domain = strings.TrimSpace(c.Domain)
	if c.Path == "" {
		c.Path = "/"
	}
	if c.Created == "" {
		c.Created = time.Now().Format(time.RFC3339)
	}
	s.items[key(c.Domain, c.Path, c.Name)] = &c
	s.save()
	return c
}

// Delete removes a single cookie. Returns true if it existed.
func (s *Store) Delete(domain, path, name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := key(domain, path, name)
	if _, ok := s.items[k]; !ok {
		return false
	}
	delete(s.items, k)
	s.save()
	return true
}

// Clear removes all cookies for a domain (suffix match). Empty domain clears
// everything. Returns the number of cookies removed.
func (s *Store) Clear(domain string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(domain) == "" {
		n := len(s.items)
		s.items = make(map[string]*Cookie)
		s.save()
		return n
	}

	needle := strings.ToLower(strings.TrimSpace(domain))
	removed := 0
	for k, c := range s.items {
		if strings.HasSuffix(strings.ToLower(c.Domain), needle) {
			delete(s.items, k)
			removed++
		}
	}
	if removed > 0 {
		s.save()
	}
	return removed
}

// HeaderValue builds the `Cookie` header for a request to the given host and
// path, honouring path matching and expiry. Session cookies (no expiry) are
// always included.
func (s *Store) HeaderValue(host, requestPath string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var pairs []string
	for _, c := range s.items {
		if !cookieApplies(c, host, requestPath, now) {
			continue
		}
		pairs = append(pairs, c.Name+"="+c.Value)
	}
	return strings.Join(pairs, "; ")
}

func cookieApplies(c *Cookie, host, path string, now time.Time) bool {
	cHost := strings.ToLower(strings.TrimSpace(c.Domain))
	host = strings.ToLower(strings.TrimSpace(host))
	if cHost == "" {
		return false
	}
	if host != cHost && !strings.HasSuffix(host, "."+cHost) {
		return false
	}
	if !strings.HasPrefix(path, c.Path) {
		return false
	}
	if c.Expires != "" {
		if t, err := time.Parse(time.RFC3339, c.Expires); err == nil && now.After(t) {
			return false
		}
	}
	return true
}