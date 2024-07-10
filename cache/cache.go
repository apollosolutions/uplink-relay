package cache

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// Cache represents a simple cache interface.
type Cache interface {
	Get(key string) ([]byte, bool)                      // Get retrieves an item from the cache if it exists and hasn't expired.
	Set(key string, content string, duration int) error // Set adds an item to the cache with a specified duration until expiration.
}

// CacheItem represents a single cached item.
type CacheItem struct {
	Content    []byte    // Byte content of the cached item.
	Expiration time.Time // Expiration time of the cached item.
}

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
		expiration = time.Unix(1<<63-1, 0) // Maximum possible time
	}

	c.items[key] = &CacheItem{Content: []byte(content), Expiration: expiration}
	c.currentItems++

	return nil
}

// makeCacheKey generates a cache key from the provided graphID, variantID, and operationName.
func MakeCacheKey(graphID, variantID, operationName string, extraArgs ...interface{}) string {
	// Append any extra arguments to the cache key.
	if len(extraArgs) > 0 {
		hash := sha256.Sum256([]byte(fmt.Sprint(extraArgs...)))
		return fmt.Sprintf("%s:%s:%s:%x", graphID, variantID, operationName, hash)
	}

	return fmt.Sprintf("%s:%s:%s", graphID, variantID, operationName)
}

func timeBeforeWithIndefinite(expirationTime time.Time, compareTo time.Time) bool {
	return expirationTime.Before(compareTo) && !isIndefinite(expirationTime)
}

func isIndefinite(expirationTime time.Time) bool {
	return expirationTime == time.Unix(1<<63-1, 0)
}
