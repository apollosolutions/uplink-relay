package redis

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis"
)

func TestRedisCacheGet(t *testing.T) {
	// Create a test Redis server
	server := miniredis.RunT(t)

	// Create a Redis client for testing
	client := redis.NewClient(&redis.Options{
		Addr: server.Addr(),
	})

	// Create a RedisCache instance
	cache := NewRedisCache(client)

	// Set a test key-value pair in Redis
	err := client.Set("test_key", "test_value", 0).Err()
	if err != nil {
		t.Fatalf("Failed to set test data in Redis: %v", err)
	}

	// Test Get method
	content, found := cache.Get("test_key")
	if !found {
		t.Errorf("Expected key 'test_key' to be found in Redis cache")
	}

	expectedContent := "test_value"
	if string(content) != expectedContent {
		t.Errorf("Expected content '%s', got '%s'", expectedContent, string(content))
	}
}

func TestRedisCacheSet(t *testing.T) {
	// Create a test Redis server
	server := miniredis.RunT(t)

	// Create a Redis client for testing
	client := redis.NewClient(&redis.Options{
		Addr: server.Addr(),
	})

	// Create a RedisCache instance
	cache := NewRedisCache(client)

	// Test Set method
	err := cache.Set("test_key", "test_value", 0)
	if err != nil {
		t.Fatalf("Failed to set test data in Redis: %v", err)
	}

	// Check if the key-value pair is set in Redis
	content, err := client.Get("test_key").Result()
	if err != nil {
		t.Fatalf("Failed to get test data from Redis: %v", err)
	}

	expectedContent := "test_value"
	if content != expectedContent {
		t.Errorf("Expected content '%s', got '%s'", expectedContent, content)
	}
}
