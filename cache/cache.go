package cache

import (
	"apollosolutions/uplink-relay/internal/util"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// Cache represents a simple cache interface.
type Cache interface {
	Get(key string) ([]byte, bool)                      // Get retrieves an item from the cache if it exists and hasn't expired.
	Set(key string, content string, duration int) error // Set adds an item to the cache with a specified duration until expiration.
	DeleteWithPrefix(prefix string) error
	Name() string
}

type keyType string

const CacheKey keyType = "cache"

const CurrentStatusKey string = "current"

var IndefiniteTimestamp = time.Unix(0, 0)

// CacheItem represents a single cached item.
type CacheItem struct {
	Content      []byte    `json:"content"`      // Byte content of the cached item.
	Expiration   time.Time `json:"expiration"`   // Expiration time of the cached item for in-memory use.
	Hash         string    `json:"hash"`         // sha256 hash of the cached item.
	LastModified time.Time `json:"lastModified"` // Last modified time of the cached item.
	ID           string    `json:"id"`           // ID of the cached item.
}

// CurrentCacheMetadata represents the current cache metadata. It points to the various cache keys to more easily retrieve the schema, for example. These will only point to the latest cache key with actual data- that is, those that aren't Unchanged.
type CurrentCacheMetadata struct {
	LastModified      time.Time `json:"lastModified"`      // Last modified time of the cache.
	SupergraphKey     string    `json:"supergraphKey"`     // Supergraph key of the cache.
	PersistedQueryKey string    `json:"persistedQueryKey"` // Persisted query key of the cache.
	EntitlementKey    string    `json:"entitlementKey"`    // Entitlement key of the cache.
}

// makeCacheKey generates a cache key from the provided graphID, variantID, and operationName.
func MakeCacheKey(graphRef, operationName string, extraArgs ...interface{}) string {
	prefix := MakeCachePrefix(graphRef, operationName)
	// Append any extra arguments to the cache key.
	if len(extraArgs) > 0 {
		return fmt.Sprintf("%s:%s", prefix, util.HashString(fmt.Sprint(extraArgs...)))
	}

	return prefix
}

func MakeCachePrefix(graphRef string, operationName string) string {
	graphID, variantID, err := util.ParseGraphRef(graphRef)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s:%s:%s", graphID, variantID, operationName)
}

// UpdateNewest updates the base cache entry with the passed item if it is newer.
// This means that new routers won't have outdated artifacts, since the entry will always be the one without arguments; in environments with duration == -1, this leads to an outdated entry
// from uplink-relay's POV, so this ensures that the entry is always the latest
func UpdateNewest(systemCache Cache, logger *slog.Logger, graphRef string, operationName string, passedItem CacheItem) error {
	if passedItem.Content == nil || len(passedItem.Content) == 0 {
		return nil
	}

	cacheKey := DefaultCacheKey(graphRef, operationName)
	var firstEntry CacheItem

	// Get the first entry
	entry, ok := systemCache.Get(cacheKey)
	if ok {
		// Unmarshal the first entry
		if err := json.Unmarshal([]byte(entry), &firstEntry); err != nil {
			logger.Error("Error unmarshalling cache entry", "cacheKey", cacheKey)
			return err
		}
	} else {
		firstEntry = CacheItem{
			Content:      nil,
			Expiration:   IndefiniteTimestamp,
			Hash:         "",
			LastModified: IndefiniteTimestamp,
			ID:           "",
		}
	}

	// If the passed item is newer than the first entry, update the first entry (aka the one without arguments)
	if firstEntry.LastModified.Before(passedItem.LastModified) && firstEntry.Hash != passedItem.Hash {
		cacheBytes, err := json.Marshal(passedItem)
		if err != nil {
			return err
		}
		// default args to set the first entry
		return systemCache.Set(cacheKey, string(cacheBytes[:]), -1)
	}
	return nil
}

func timeBeforeWithIndefinite(expirationTime time.Time, compareTo time.Time) bool {
	return expirationTime.Before(compareTo) && !isIndefinite(expirationTime)
}

func isIndefinite(expirationTime time.Time) bool {
	return expirationTime == IndefiniteTimestamp
}

func DefaultCacheKey(graphRef string, operationName string) string {
	return MakeCacheKey(graphRef, operationName, map[string]interface{}{"graph_ref": graphRef, "ifAfterId": ""})
}

func ExpirationTime(duration int) time.Time {
	if duration == -1 {
		return IndefiniteTimestamp
	}
	return time.Now().Add(time.Duration(duration) * time.Second)
}
