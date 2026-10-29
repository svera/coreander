package biblioreads

import (
	"sync"
	"time"
)

// DefaultCacheTTL is how long successful responses are cached by default.
const DefaultCacheTTL = 24 * time.Hour

// cacheSweepInterval sets how often expired entries are purged, so entries that are never
// requested again don't stay in memory forever.
const cacheSweepInterval = 10 * time.Minute

type cacheEntry struct {
	body      []byte
	expiresAt time.Time
}

// responseCache is an in-memory cache of raw response bodies with a fixed TTL.
type responseCache struct {
	ttl       time.Duration
	mu        sync.Mutex
	entries   map[string]cacheEntry
	lastSweep time.Time
}

func newResponseCache(ttl time.Duration) *responseCache {
	return &responseCache{
		ttl:       ttl,
		entries:   make(map[string]cacheEntry),
		lastSweep: time.Now(),
	}
}

func (c *responseCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if !time.Now().Before(entry.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return entry.body, true
}

func (c *responseCache) set(key string, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if now.Sub(c.lastSweep) >= cacheSweepInterval {
		for k, entry := range c.entries {
			if !now.Before(entry.expiresAt) {
				delete(c.entries, k)
			}
		}
		c.lastSweep = now
	}
	c.entries[key] = cacheEntry{body: body, expiresAt: now.Add(c.ttl)}
}
