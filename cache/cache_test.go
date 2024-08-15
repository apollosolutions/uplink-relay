package cache

import (
	"apollosolutions/uplink-relay/logger"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

const defaultCacheContent = "content1"

func TestMemoryCacheGet(t *testing.T) {
	cache := NewMemoryCache(10)

	// Test case 1: Get an existing item from the cache
	cache.Set("key1", defaultCacheContent, 10)
	content, found := cache.Get("key1")
	if !found {
		t.Errorf("Expected item fto be found in cache")
	}
	if string(content[:]) != defaultCacheContent {
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
	key := MakeCacheKey("graphID1@variantID1", "operationName1")
	expectedKey := "graphID1:variantID1:operationName1"
	if key != expectedKey {
		t.Errorf("Expected key to be '%s', got '%s'", expectedKey, key)
	}

	// Test case 2: Generate cache key with extra arguments
	key = MakeCacheKey("graphID2@variantID2", "operationName2", 123, true)
	expectedKey = "graphID2:variantID2:operationName2:6a203127b0128340878b0beadbb7cabae0ad8ae6ff0f48a054cda7cdaa87f9d5"
	if key != expectedKey {
		t.Errorf("Expected key to be '%s', got '%s'", expectedKey, key)
	}
}
func TestCacheDeleteWithPrefix(t *testing.T) {
	cache := NewMemoryCache(10)

	// Set some items in the cache
	cache.Set("key1", "content1", 10)
	cache.Set("key2", "content2", 10)
	cache.Set("prefix1_key1", "content3", 10)
	cache.Set("prefix1_key2", "content4", 10)
	cache.Set("prefix2_key1", "content5", 10)

	// Delete items with prefix "prefix1"
	err := cache.DeleteWithPrefix("prefix1")
	if err != nil {
		t.Errorf("Expected no error, got '%s'", err.Error())
	}

	// Check if items with prefix "prefix1" are deleted
	_, found := cache.Get("prefix1_key1")
	if found {
		t.Errorf("Expected item to be deleted from cache")
	}
	_, found = cache.Get("prefix1_key2")
	if found {
		t.Errorf("Expected item to be deleted from cache")
	}

	// Check if items with prefix "prefix2" are still present
	_, found = cache.Get("prefix2_key1")
	if !found {
		t.Errorf("Expected item to be found in cache")
	}
}

func TestUpdateNewest(t *testing.T) {
	cache := NewMemoryCache(10)

	cacheKey := DefaultCacheKey("key1", "operationName")
	// Set an initial item in the cache
	initialItem := CacheItem{
		Content:      []byte("content1"),
		Expiration:   time.Now().Add(time.Minute),
		Hash:         "hash1",
		LastModified: time.Now(),
		ID:           "id1",
	}
	initialBytes, _ := json.Marshal(initialItem)
	cache.Set(cacheKey, string(initialBytes[:]), -1)

	// Create a new cache item with a newer lastModified time
	newItem := CacheItem{
		Content:      []byte("content2"),
		Expiration:   time.Now().Add(time.Minute),
		Hash:         "hash2",
		LastModified: time.Now().Add(time.Hour),
		ID:           "id2",
	}

	newBytes, _ := json.Marshal(newItem)
	// Update the cache with the newer item
	err := UpdateNewest(cache, logger.MakeLogger(nil), "key1", "operationName", newItem)
	if err != nil {
		t.Errorf("Expected no error, got '%s'", err.Error())
	}

	// Check if the cache item is updated
	content, found := cache.Get(cacheKey)
	if !found {
		t.Errorf("Expected item to be found in cache")
	}
	if string(content) != string(newBytes) {
		t.Errorf("Expected content to be 'content2', got '%s'", string(content))
	}
}

func TestTimeBeforeWithIndefinite(t *testing.T) {
	expirationTime := time.Now().Add(time.Hour)
	compareTo := time.Now()

	// Check if expirationTime is before compareTo
	result := timeBeforeWithIndefinite(expirationTime, compareTo)
	if result {
		t.Errorf("Expected expirationTime %v to be after %v", expirationTime.Format(time.RFC3339), compareTo.Format(time.RFC3339))
	}

	// Check if expirationTime is not before compareTo
	result = timeBeforeWithIndefinite(compareTo, expirationTime)
	if !result {
		t.Errorf("Expected %v to be not before %v", compareTo.Format(time.RFC3339), expirationTime.Format(time.RFC3339))
	}
}

func TestIsIndefinite(t *testing.T) {
	expirationTime := time.Now().Add(time.Hour)

	// Check if expirationTime is not indefinite
	result := isIndefinite(expirationTime)
	if result {
		t.Errorf("Expected expirationTime to be not indefinite")
	}

	// Check if IndefiniteTimestamp is indefinite
	result = isIndefinite(IndefiniteTimestamp)
	if !result {
		t.Errorf("Expected IndefiniteTimestamp to be indefinite")
	}
}

func TestDefaultCacheKey(t *testing.T) {
	graphRef := "graphID@variantID"
	operationName := "operationName"

	// Generate the default cache key
	key := DefaultCacheKey(graphRef, operationName)
	expectedKey := "graphID:variantID:operationName:8bca5a3afa4fb1de46522e5d0cafc1a4e7d428c0d7ae728a81d089afc8777bd9"
	if key != expectedKey {
		t.Errorf("Expected key to be '%s', got '%s'", expectedKey, key)
	}
}

func TestExpirationTime(t *testing.T) {
	duration := 10

	// Calculate the expiration time
	expirationTime := ExpirationTime(-1)
	if expirationTime != IndefiniteTimestamp {
		t.Errorf("Expected expiration time to be IndefiniteTimestamp")
	}

	// this may be flaky in the future since the time is calculated in the function; this is just a simple test however and unlikely to crop up often
	expirationTime = ExpirationTime(duration)
	if time.Since(expirationTime).Seconds() > float64(duration) {
		t.Errorf("Expected expiration time to be %d seconds in the future", duration)
	}
}
