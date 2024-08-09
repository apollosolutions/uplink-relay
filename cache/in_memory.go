package cache

import (
	"sync"
	"time"
)

// MemoryCache provides a simple in-memory cache.
type MemoryCache struct {
	items        map[string]*CacheItem // Map of cache keys to CacheItems.
	mu           sync.RWMutex          // Read/Write mutex for thread-safe access.
	maxItems     int                   // Maximum size of the cache.
	currentItems int                   // Current size of the cache.
}

// NewMemoryCache initializes a new empty MemoryCache.
func NewMemoryCache(maxItems int) *MemoryCache {
	return &MemoryCache{items: make(map[string]*CacheItem), maxItems: maxItems}
}

// Get retrieves an item from the cache if it exists and hasn't expired.
func (c *MemoryCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, found := c.items[key]

	// If the item is not found or has expired, return a cache miss.
	// The special case of time.Unix(1<<63-1, 0) is used to indicate that an item never expires- and
	// time.Before will always return true for this case.
	if !found || timeBeforeWithIndefinite(item.Expiration, time.Now()) {
		return nil, false
	}
	return item.Content, true
}

// Set adds an item to the cache with a specified duration until expiration.
// If duration is -1, the item never expires and will never be removed, even if it is above the cache capacity.
func (c *MemoryCache) Set(key string, content string, duration int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If the cache is full, remove the oldest item.
	if c.currentItems >= c.maxItems {
		var oldestKey string
		var oldestExpiration time.Time
		for k, v := range c.items {
			if oldestKey == "" || timeBeforeWithIndefinite(v.Expiration, oldestExpiration) {
				if isIndefinite(v.Expiration) {
					continue
				}
				oldestKey = k
				oldestExpiration = v.Expiration
			}
		}
		delete(c.items, oldestKey)
		c.currentItems--
	}

	expiration := time.Now().Add(time.Duration(duration) * time.Second)
	if duration == -1 {
		expiration = IndefiniteTimestamp
	}

	c.items[key] = &CacheItem{Content: []byte(content), Expiration: expiration}
	c.currentItems++

	return nil
}

func (c *MemoryCache) DeleteWithPrefix(prefix string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for k := range c.items {
		if k[:len(prefix)] == prefix {
			delete(c.items, k)
			c.currentItems--
		}
	}

	return nil
}
