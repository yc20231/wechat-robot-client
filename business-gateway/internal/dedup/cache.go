package dedup

import (
	"sync"
	"time"
)

type Cache interface {
	Seen(key string) bool
}

type MemoryCache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]time.Time
	checks  uint64
}

func NewMemoryCache(ttl time.Duration) *MemoryCache {
	return &MemoryCache{ttl: ttl, entries: make(map[string]time.Time)}
}

func (c *MemoryCache) Seen(key string) bool {
	if key == "" {
		return false
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks++
	if c.checks%256 == 0 {
		for existing, expiresAt := range c.entries {
			if !expiresAt.After(now) {
				delete(c.entries, existing)
			}
		}
	}
	if expiresAt, ok := c.entries[key]; ok && expiresAt.After(now) {
		return true
	}
	c.entries[key] = now.Add(c.ttl)
	return false
}
