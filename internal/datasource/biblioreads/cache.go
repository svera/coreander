package biblioreads

import (
	"sync"
	"sync/atomic"
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
	requestID uint64
}

// responseCache is an in-memory cache of raw response bodies with a fixed TTL.
type responseCache struct {
	ttl       time.Duration
	mu        sync.Mutex
	entries   map[string]cacheEntry
	lastSweep time.Time
	requestID atomic.Uint64
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

func (c *responseCache) set(key string, body []byte, requestID uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	// A slower request started before a refresh must not overwrite the newer response.
	if entry, ok := c.entries[key]; ok && now.Before(entry.expiresAt) && entry.requestID > requestID {
		return
	}
	if now.Sub(c.lastSweep) >= cacheSweepInterval {
		for k, entry := range c.entries {
			if !now.Before(entry.expiresAt) {
				delete(c.entries, k)
			}
		}
		c.lastSweep = now
	}
	c.entries[key] = cacheEntry{body: body, expiresAt: now.Add(c.ttl), requestID: requestID}
}
