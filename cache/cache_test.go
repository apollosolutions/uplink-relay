package cache

import (
	"fmt"
	"testing"
)

func TestMemoryCacheGet(t *testing.T) {
	cache := NewMemoryCache(10)

	// Test case 1: Get an existing item from the cache
	cache.Set("key1", "content1", 10)
	content, found := cache.Get("key1")
	if !found {
		t.Errorf("Expected item to be found in cache")
	}
	if string(content) != "content1" {
		t.Errorf("Expected content to be 'content1', got '%s'", string(content))
	}

	// Test case 2: Get a non-existing item from the cache
	_, found = cache.Get("non_existing_key")
	if found {
		t.Errorf("Expected item to not be found in cache")
	}
}

func TestMemoryCacheSet(t *testing.T) {
	cache := NewMemoryCache(5)

	// Test case 1: Set an item with a positive duration
	err := cache.Set("key1", "content1", 10)
	if err != nil {
		t.Errorf("Expected no error, got '%s'", err.Error())
	}

	// Test case 2: Set an item with a negative duration (never expires)
	err = cache.Set("key2", "content2", -1)
	if err != nil {
		t.Errorf("Expected no error, got '%s'", err.Error())
	}

	for i := 3; i < 10; i++ {
		cache.Set(fmt.Sprintf("key%v", i), "content", 10)
	}

	// Test case 3: Cache is full, remove the oldest item
	_, found := cache.Get("key3")
	if found {
		t.Errorf("Expected item to be removed from cache")
	}

	// Test case 4: Validate the indefinite expiration time exists in the cache
	_, found = cache.Get("key2")
	if !found {
		t.Errorf("Expected item to be found in cache")
	}
}

func TestMakeCacheKey(t *testing.T) {
	// Test case 1: Generate cache key with only required arguments
	key := MakeCacheKey("graphID1", "variantID1", "operationName1")
	expectedKey := "graphID1:variantID1:operationName1"
	if key != expectedKey {
		t.Errorf("Expected key to be '%s', got '%s'", expectedKey, key)
	}

	// Test case 2: Generate cache key with extra arguments
	key = MakeCacheKey("graphID2", "variantID2", "operationName2", 123, true)
	expectedKey = "graphID2:variantID2:operationName2:6a203127b0128340878b0beadbb7cabae0ad8ae6ff0f48a054cda7cdaa87f9d5"
	if key != expectedKey {
		t.Errorf("Expected key to be '%s', got '%s'", expectedKey, key)
	}
}
