package cache

import (
	"container/list"
	"sync"
	"time"
)

type entry struct {
	key       string
	value     string
	expiresAt time.Time
}

// Cache is an in-process string-to-string cache with per-entry TTL and
// LRU eviction when maxEntries is reached.
type Cache struct {
	mu         sync.Mutex
	maxEntries int
	ttl        time.Duration
	items      map[string]*list.Element // value type: *entry
	order      *list.List               // front = MRU, back = LRU
}

func New(maxEntries int, ttl time.Duration) *Cache {
	return &Cache{
		maxEntries: maxEntries,
		ttl:        ttl,
		items:      make(map[string]*list.Element),
		order:      list.New(),
	}
}

func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return "", false
	}
	e := el.Value.(*entry)
	if time.Now().After(e.expiresAt) {
		c.order.Remove(el)
		delete(c.items, key)
		return "", false
	}
	c.order.MoveToFront(el)
	return e.value, true
}

func (c *Cache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.maxEntries <= 0 {
		return
	}
	if el, ok := c.items[key]; ok {
		e := el.Value.(*entry)
		e.value = value
		e.expiresAt = time.Now().Add(c.ttl)
		c.order.MoveToFront(el)
		return
	}
	e := &entry{key: key, value: value, expiresAt: time.Now().Add(c.ttl)}
	el := c.order.PushFront(e)
	c.items[key] = el
	if c.order.Len() > c.maxEntries {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			delete(c.items, oldest.Value.(*entry).key)
		}
	}
}

func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}
