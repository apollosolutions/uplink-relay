package main

import (
	"sync"
	"time"
)

// CacheItem represents a single cached item.
type CacheItem struct {
	Content    []byte    // Byte content of the cached item.
	Expiration time.Time // Expiration time of the cached item.
}

// MemoryCache provides a simple in-memory cache.
type MemoryCache struct {
	items       map[string]*CacheItem // Map of cache keys to CacheItems.
	mu          sync.RWMutex          // Read/Write mutex for thread-safe access.
	maxSize     int                   // Maximum size of the cache.
	currentSize int                   // Current size of the cache.
}

// NewMemoryCache initializes a new empty MemoryCache.
func NewMemoryCache(maxSize int) *MemoryCache {
	return &MemoryCache{items: make(map[string]*CacheItem), maxSize: maxSize}
}

// Get retrieves an item from the cache if it exists and hasn't expired.
func (c *MemoryCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	item, found := c.items[key]
	if !found || item.Expiration.Before(time.Now()) {
		return nil, false
	}
	return item.Content, true
}

// Set adds an item to the cache with a specified duration until expiration.
func (c *MemoryCache) Set(key string, content []byte, duration int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If the cache is full, remove the oldest item.
	if c.currentSize >= c.maxSize {
		var oldestKey string
		var oldestExpiration time.Time
		for k, v := range c.items {
			if oldestKey == "" || v.Expiration.Before(oldestExpiration) {
				oldestKey = k
				oldestExpiration = v.Expiration
			}
		}
		delete(c.items, oldestKey)
		c.currentSize--
	}

	c.items[key] = &CacheItem{Content: content, Expiration: time.Now().Add(time.Duration(duration) * time.Second)}
	c.currentSize++
}
