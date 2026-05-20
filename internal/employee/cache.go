package employee

import (
	"sync"
	"time"
)

type cacheEntry struct {
	value     interface{}
	expiresAt time.Time
}

// Cache is a thread-safe in-memory key-value store with per-entry TTL.
// A background janitor goroutine evicts expired entries every 5 minutes to
// prevent unbounded memory growth.
type Cache struct {
	mu    sync.RWMutex
	items map[string]cacheEntry
}

func newCache() *Cache {
	c := &Cache{items: make(map[string]cacheEntry)}
	go c.janitor()
	return c
}

// Get returns the stored value for key, or nil if the key is absent or expired.
func (c *Cache) Get(key string) interface{} {
	c.mu.RLock()
	it, ok := c.items[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(it.expiresAt) {
		return nil
	}
	return it.value
}

// Set stores value under key; the entry is automatically evicted after ttl elapses.
func (c *Cache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	c.items[key] = cacheEntry{value: value, expiresAt: time.Now().Add(ttl)}
	c.mu.Unlock()
}

// Delete removes key from the cache. No-op if the key is absent.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

func (c *Cache) janitor() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		c.mu.Lock()
		for k, it := range c.items {
			if now.After(it.expiresAt) {
				delete(c.items, k)
			}
		}
		c.mu.Unlock()
	}
}
