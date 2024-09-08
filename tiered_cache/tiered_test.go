package tiered_cache

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/logger"
	apolloredis "apollosolutions/uplink-relay/redis"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis"
)

func TestNewTieredCache(t *testing.T) {
	// Create a mock logger
	logger := logger.MakeLogger(nil)

	// Create a test Redis server
	server := miniredis.RunT(t)
	// Create a Redis client for testing
	client := redis.NewClient(&redis.Options{
		Addr: server.Addr(),
	})

	// Create mock caches
	cache1 := cache.NewMemoryCache(100)
	cache2 := apolloredis.NewRedisCache(client)

	// Create a new TieredCache
	tc, err := NewTieredCache([]cache.Cache{cache1, cache2}, logger, 60)
	if err != nil {
		t.Errorf("Failed to create TieredCache: %v", err)
	}

	// Verify that the caches are set correctly
	if len(tc.caches) != 2 {
		t.Errorf("Expected 2 caches, got %d", len(tc.caches))
	}
	if tc.caches[0] != cache1 {
		t.Errorf("Expected cache1, got %v", tc.caches[0])
	}
	if tc.caches[1] != cache2 {
		t.Errorf("Expected cache2, got %v", tc.caches[1])
	}

	// Verify that the logger is set correctly
	if tc.logger != logger {
		t.Errorf("Expected logger, got %v", tc.logger)
	}

	// Verify that the duration is set correctly
	if tc.duration != 60 {
		t.Errorf("Expected duration 60, got %d", tc.duration)
	}
}

func TestTieredCache_Get(t *testing.T) {
	// Create a mock logger
	logger := logger.MakeLogger(nil)

	// Create a mock cache
	cache1 := cache.NewMemoryCache(100)

	// Create a new TieredCache
	tc, _ := NewTieredCache([]cache.Cache{cache1}, logger, 60)

	// Set a value in the cache
	cache1.Set("key", "value", 60)

	// Retrieve the value from the TieredCache
	content, found := tc.Get("key")

	// Verify that the value is retrieved correctly
	if !found {
		t.Errorf("Expected value to be found")
	}
	if string(content) != "value" {
		t.Errorf("Expected value 'value', got '%s'", string(content))
	}
}

func TestTieredCache_Set(t *testing.T) {
	// Create a mock logger
	logger := logger.MakeLogger(nil)

	// Create a mock cache
	cache1 := cache.NewMemoryCache(100)

	// Create a new TieredCache
	tc, _ := NewTieredCache([]cache.Cache{cache1}, logger, 60)

	// Set a value in the TieredCache
	err := tc.Set("key", "value", 60)

	// Verify that the value is set correctly
	if err != nil {
		t.Errorf("Failed to set value: %v", err)
	}
}

func TestTieredCache_DeleteWithPrefix(t *testing.T) {
	// Create a mock logger
	logger := logger.MakeLogger(nil)

	// Create a mock cache
	cache1 := cache.NewMemoryCache(100)

	// Create a new TieredCache
	tc, _ := NewTieredCache([]cache.Cache{cache1}, logger, 60)

	// Set values in the cache
	cache1.Set("key1", "value1", 60)
	cache1.Set("key2", "value2", 60)
	cache1.Set("prefix1_key", "value3", 60)
	cache1.Set("prefix2_key", "value4", 60)

	// Delete values with prefix "prefix1_"
	err := tc.DeleteWithPrefix("prefix1_")

	// Verify that the values are deleted correctly
	if err != nil {
		t.Errorf("Failed to delete values: %v", err)
	}
	if _, found := cache1.Get("prefix1_key"); found {
		t.Errorf("Expected 'prefix1_key' to be deleted")
	}
	if _, found := cache1.Get("prefix2_key"); !found {
		t.Errorf("Expected 'prefix2_key' to be present")
	}
}

func TestTieredCache_Name(t *testing.T) {
	// Create a mock logger
	logger := logger.MakeLogger(nil)

	// Create a mock cache
	cache1 := cache.NewMemoryCache(100)

	// Create a new TieredCache
	tc, _ := NewTieredCache([]cache.Cache{cache1}, logger, 60)

	// Verify the name of the TieredCache
	name := tc.Name()
	if name != "Tiered" {
		t.Errorf("Expected name 'TieredCache', got '%s'", name)
	}
}
