// Package rbac wires the RBAC middleware to a read-through cache populated
// from core-service. Mirrors lib/auth/rbac.ts but resolves permissions remotely
// (this service has no permissions catalog of its own).
package rbac

import (
	"context"
	"sync"
	"time"
)

// Cache is a tiny role→permissions LRU-less map with per-entry TTL. We don't
// need an LRU because the cardinality is the number of roles in the system —
// a handful. A library would be overkill.
type Cache struct {
	ttl     time.Duration
	mu      sync.RWMutex
	entries map[string]entry
	loader  Loader
}

// Loader is the function the cache calls on miss. The coreclient implements it.
type Loader func(ctx context.Context, role string) ([]string, error)

type entry struct {
	perms     []string
	expiresAt time.Time
}

func NewCache(ttl time.Duration, loader Loader) *Cache {
	return &Cache{
		ttl:     ttl,
		entries: make(map[string]entry),
		loader:  loader,
	}
}

// Get returns the permissions for a role, loading from the loader on miss or
// expiry. Concurrent misses for the same role race the loader — acceptable
// here because loader is idempotent and cheap.
func (c *Cache) Get(ctx context.Context, role string) ([]string, error) {
	c.mu.RLock()
	if e, ok := c.entries[role]; ok && time.Now().Before(e.expiresAt) {
		c.mu.RUnlock()
		return e.perms, nil
	}
	c.mu.RUnlock()

	perms, err := c.loader(ctx, role)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.entries[role] = entry{perms: perms, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()
	return perms, nil
}

// Invalidate removes a role from the cache. Wired by the (homework)
// rbac.permissions_changed event handler.
func (c *Cache) Invalidate(role string) {
	c.mu.Lock()
	delete(c.entries, role)
	c.mu.Unlock()
}

// InvalidateAll wipes the cache. Use sparingly — preferable to fire-and-forget
// on bulk RBAC changes.
func (c *Cache) InvalidateAll() {
	c.mu.Lock()
	c.entries = make(map[string]entry)
	c.mu.Unlock()
}
