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
func TestRedisCacheDeleteWithPrefix(t *testing.T) {
	// Create a test Redis server
	server := miniredis.RunT(t)

	// Create a Redis client for testing
	client := redis.NewClient(&redis.Options{
		Addr: server.Addr(),
	})

	// Create a RedisCache instance
	cache := NewRedisCache(client)

	// Set test key-value pairs in Redis
	err := client.Set("test_key_1", "test_value_1", 0).Err()
	if err != nil {
		t.Fatalf("Failed to set test data in Redis: %v", err)
	}

	err = client.Set("test_key_2", "test_value_2", 0).Err()
	if err != nil {
		t.Fatalf("Failed to set test data in Redis: %v", err)
	}

	err = client.Set("other_key", "other_value", 0).Err()
	if err != nil {
		t.Fatalf("Failed to set test data in Redis: %v", err)
	}

	// Test DeleteWithPrefix method
	err = cache.DeleteWithPrefix("test_key")
	if err != nil {
		t.Fatalf("Failed to delete keys with prefix: %v", err)
	}

	// Check if the keys with prefix are deleted from Redis
	_, err = client.Get("test_key_1").Result()
	if err != redis.Nil {
		t.Errorf("Expected key 'test_key_1' to be deleted from Redis cache")
	}

	_, err = client.Get("test_key_2").Result()
	if err != redis.Nil {
		t.Errorf("Expected key 'test_key_2' to be deleted from Redis cache")
	}

	// Check if other keys are still present in Redis
	_, err = client.Get("other_key").Result()
	if err != nil {
		t.Errorf("Expected key 'other_key' to be present in Redis cache")
	}
}
