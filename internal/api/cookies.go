package api

import (
	"context"

	"dashboard/internal/cookie"
)

// CookieInfo mirrors internal/cookie.Cookie for the frontend.
type CookieInfo struct {
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	Secure   bool   `json:"secure,omitempty"`
	HttpOnly bool   `json:"http_only,omitempty"`
	Expires  string `json:"expires,omitempty"`
	Created  string `json:"created"`
}

// CookieAPI exposes the persistent cookie jar to the frontend.
type CookieAPI struct {
	store *cookie.Store
}

func NewCookieAPI(store *cookie.Store) *CookieAPI {
	return &CookieAPI{store: store}
}

// ListCookies returns cookies, optionally filtered by domain (suffix match).
func (c *CookieAPI) ListCookies(ctx context.Context, domain string) []CookieInfo {
	items := c.store.List(domain)
	out := make([]CookieInfo, len(items))
	for i, ck := range items {
		out[i] = CookieInfo(ck)
	}
	return out
}

// SetCookie stores a cookie and returns the persisted value.
func (c *CookieAPI) SetCookie(ctx context.Context, ck CookieInfo) CookieInfo {
	stored := c.store.Set(cookie.Cookie(ck))
	return CookieInfo(stored)
}

// DeleteCookie removes a single cookie. Returns true if it existed.
func (c *CookieAPI) DeleteCookie(ctx context.Context, domain, path, name string) bool {
	return c.store.Delete(domain, path, name)
}

// ClearCookies removes all cookies for a domain (empty = all). Returns count.
func (c *CookieAPI) ClearCookies(ctx context.Context, domain string) int {
	return c.store.Clear(domain)
}