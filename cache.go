package main

import (
	"fmt"
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
// If duration is -1, the item never expires.
func (c *MemoryCache) Set(key string, content string, duration int) {
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

	expiration := time.Now().Add(time.Duration(duration) * time.Second)
	if duration == -1 {
		expiration = time.Unix(1<<63-1, 0) // Maximum possible time
	}

	c.items[key] = &CacheItem{Content: []byte(content), Expiration: expiration}
	c.currentSize++
}

// makeCacheKey generates a cache key from the provided graphID, variantID, and operationName.
func makeCacheKey(graphID, variantID, operationName string) string {
	return fmt.Sprintf("%s:%s:%s", graphID, variantID, operationName)
}
