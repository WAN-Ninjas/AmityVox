package federation

import (
	"sync"
	"time"
)

// ttlEntry holds a cached value and its expiry time.
type ttlEntry[V any] struct {
	value  V
	expiry time.Time
}

// TTLCache is a generic, thread-safe, in-memory cache with per-entry TTL
// and a configurable maximum size. Expired entries are lazily evicted on Get;
// when full, the oldest entry (by expiry) is evicted on Set.
type TTLCache[V any] struct {
	mu      sync.Mutex
	entries map[string]ttlEntry[V]
	ttl     time.Duration
	maxSize int
}

// NewTTLCache creates a TTLCache with the given default TTL and max capacity.
func NewTTLCache[V any](ttl time.Duration, maxSize int) *TTLCache[V] {
	if maxSize <= 0 {
		maxSize = 1
	}
	return &TTLCache[V]{
		entries: make(map[string]ttlEntry[V], maxSize),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// Get returns the cached value for key. Returns (zero, false) on miss or
// if the entry has expired (lazy eviction).
func (c *TTLCache[V]) Get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[key]
	if !ok {
		var zero V
		return zero, false
	}
	if time.Now().After(e.expiry) {
		delete(c.entries, key)
		var zero V
		return zero, false
	}
	return e.value, true
}

// Set stores a value with the cache's default TTL. If the cache is at capacity
// and the key is new, the entry with the earliest expiry is evicted.
func (c *TTLCache[V]) Set(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Only evict if the key is new and we're at capacity.
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.maxSize {
		var oldestKey string
		var oldestExp time.Time
		for k, e := range c.entries {
			if oldestKey == "" || e.expiry.Before(oldestExp) {
				oldestKey = k
				oldestExp = e.expiry
			}
		}
		delete(c.entries, oldestKey)
	}

	c.entries[key] = ttlEntry[V]{
		value:  value,
		expiry: time.Now().Add(c.ttl),
	}
}

// Invalidate removes a single entry by key.
func (c *TTLCache[V]) Invalidate(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// InvalidateAll clears the entire cache.
func (c *TTLCache[V]) InvalidateAll() {
	c.mu.Lock()
	c.entries = make(map[string]ttlEntry[V], c.maxSize)
	c.mu.Unlock()
}

// Len returns the number of entries (including potentially expired ones).
func (c *TTLCache[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
