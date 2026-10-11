package httpapi

import (
	"context"
	"sync"
	"time"

	"github.com/lamooof/clickshield/internal/domain"
	"github.com/lamooof/clickshield/internal/store"
)

// projectCache keeps public key lookups, including unknown keys, out of
// PostgreSQL: every tracker request carries the key, and a flood of random
// keys must not reach the database.
type projectCache struct {
	mu      sync.Mutex
	load    func(context.Context, string) (domain.Project, error)
	ttl     time.Duration
	entries map[string]projectEntry
}

type projectEntry struct {
	project domain.Project
	found   bool
	expires time.Time
}

const maxProjectCacheEntries = 10000

func newProjectCache(st *store.Store, ttl time.Duration) *projectCache {
	c := &projectCache{ttl: ttl, entries: map[string]projectEntry{}}
	if st != nil {
		c.load = st.ProjectByKey
	}
	return c
}

// Get returns the project for a public key; found is false for unknown keys.
// Database errors are returned and not cached.
func (c *projectCache) Get(ctx context.Context, key string) (domain.Project, bool, error) {
	now := time.Now()
	c.mu.Lock()
	e, ok := c.entries[key]
	c.mu.Unlock()
	if ok && now.Before(e.expires) {
		return e.project, e.found, nil
	}
	p, err := c.load(ctx, key)
	if err != nil && !store.IsNotFound(err) {
		return domain.Project{}, false, err
	}
	e = projectEntry{project: p, found: err == nil, expires: now.Add(c.ttl)}
	c.mu.Lock()
	if len(c.entries) >= maxProjectCacheEntries {
		c.entries = map[string]projectEntry{}
	}
	c.entries[key] = e
	c.mu.Unlock()
	return e.project, e.found, nil
}

// Reset drops every cached key after a project is created or changed, so new
// origins and modes apply at once.
func (c *projectCache) Reset() {
	c.mu.Lock()
	c.entries = map[string]projectEntry{}
	c.mu.Unlock()
}
