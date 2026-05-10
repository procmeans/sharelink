package cache

import (
	"sync"
	"time"
)

type entry struct {
	value     string
	expiresAt time.Time
}

// Cache is an in-process string-to-string cache with per-entry TTL and a
// max-entries cap (LRU eviction added in a later task).
type Cache struct {
	mu          sync.Mutex
	maxEntries  int
	ttl         time.Duration
	items       map[string]*entry
}

func New(maxEntries int, ttl time.Duration) *Cache {
	return &Cache{
		maxEntries: maxEntries,
		ttl:        ttl,
		items:      make(map[string]*entry, maxEntries),
	}
}

func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok {
		return "", false
	}
	if time.Now().After(e.expiresAt) {
		delete(c.items, key)
		return "", false
	}
	return e.value, true
}

func (c *Cache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = &entry{value: value, expiresAt: time.Now().Add(c.ttl)}
}

func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}
