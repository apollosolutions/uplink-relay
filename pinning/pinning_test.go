package pinning

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/logger"
	"apollosolutions/uplink-relay/uplink"
	"net/http"
	"testing"
	"time"

	"github.com/go-jose/go-jose/json"
)

func TestHandlePinnedEntry(t *testing.T) {
	logger := logger.MakeLogger(nil)
	systemCache := cache.NewMemoryCache(10)

	// Add a sample cache entry
	sampleEntry := cache.CacheItem{
		LastModified: time.Now().UTC().Add(time.Hour),
		Content:      []byte("sampleValue"),
		ID:           "sampleID",
		Expiration:   cache.ExpirationTime(-1),
		Hash:         "sampleHash",
	}
	sampleEntryBytes, _ := json.Marshal(sampleEntry)
	systemCache.Set(cache.MakeCacheKey("sampleGraphID@sampleVariantID", SupergraphPinned), string(sampleEntryBytes[:]), -1)

	// Call the HandlePinnedEntry function
	cacheItem, err := HandlePinnedEntry(logger, systemCache, "sampleGraphID", "sampleVariantID", uplink.SupergraphQuery, "")
	if err != nil {
		t.Errorf("HandlePinnedEntry returned an error: %v", err)
	}

	// Verify the returned cache item
	if cacheItem == nil {
		t.Error("HandlePinnedEntry returned nil cache item")
	} else {
		if cacheItem.Hash != sampleEntry.Hash {
			t.Errorf("HandlePinnedEntry returned cache item with incorrect key. Expected: %s, Got: %s", sampleEntry.Hash, cacheItem.Hash)
		}
		if string(cacheItem.Content[:]) != string(sampleEntry.Content[:]) {
			t.Errorf("HandlePinnedEntry returned cache item with incorrect value. Expected: %s, Got: %s", sampleEntry.Content, cacheItem.Content)
		}
		if cacheItem.ID != sampleEntry.ID {
			t.Errorf("HandlePinnedEntry returned cache item with incorrect ID. Expected: %s, Got: %s", sampleEntry.ID, cacheItem.ID)
		}
	}

	// Call with an ifAfterId set to ensure it returns the correct values
	cacheItem, err = HandlePinnedEntry(logger, systemCache, "sampleGraphID", "sampleVariantID", uplink.SupergraphQuery, time.Now().UTC().Add(time.Hour*2).Format("2006-01-02T15:04:05.000Z"))
	if err != nil {
		t.Errorf("HandlePinnedEntry returned an error: %v", err)
	}

	// Verify that it returned nil, as there's no more recent update to the item
	if cacheItem != nil {
		if cacheItem.Content != nil {
			t.Errorf("HandlePinnedEntry returned a cache item: %+v", cacheItem)
		}
	}

	// test the logic for PQs
	systemCache.Set(cache.MakeCacheKey("sampleGraphID@sampleVariantID", PersistedQueriesPinned), string(sampleEntryBytes[:]), -1)
	cacheItem, err = HandlePinnedEntry(logger, systemCache, "sampleGraphID", "sampleVariantID", uplink.PersistedQueriesQuery, time.Now().UTC().Add(time.Hour*2).Format("2006-01-02T15:04:05.000Z"))
	if err != nil {
		t.Errorf("HandlePinnedEntry returned an error: %v", err)
	}

	// Verify it returned an item
	if cacheItem == nil {
		t.Errorf("HandlePinnedEntry returned a cache item")
	}

	// now see if it correctly sends data based on the lastModified time
	// Add a sample cache entry
	sampleEntry = cache.CacheItem{
		LastModified: time.Now().UTC().Add(-1 * time.Hour),
		Content:      []byte("sampleValue"),
		ID:           "sampleID",
		Expiration:   cache.ExpirationTime(-1),
		Hash:         "sampleHash",
	}
	sampleEntryBytes, _ = json.Marshal(sampleEntry)
	systemCache.Set(cache.MakeCacheKey("sampleGraphID@sampleVariantID", PersistedQueriesPinned), string(sampleEntryBytes[:]), -1)

	cacheItem, err = HandlePinnedEntry(logger, systemCache, "sampleGraphID", "sampleVariantID", uplink.PersistedQueriesQuery, time.Now().UTC().Format("2006-01-02T15:04:05.000Z"))
	if err != nil {
		t.Errorf("HandlePinnedEntry returned an error: %v", err)
	}

	// Verify it returned an item
	if cacheItem != nil {
		t.Errorf("HandlePinnedEntry returned an empty cache item")
	}
}
func TestInsertPinnedCacheEntry(t *testing.T) {
	logger := logger.MakeLogger(nil)
	systemCache := cache.NewMemoryCache(10)

	// Call the insertPinnedCacheEntry function
	key := "sampleKey"
	value := "sampleValue"
	id := "sampleID"
	insertPinnedCacheEntry(logger, systemCache, key, value, id, time.Now())

	// Retrieve the cache item
	cacheItemBytes, ok := systemCache.Get(key)
	if !ok {
		t.Errorf("Failed to retrieve cache item")
	}

	// Verify the cache item
	if cacheItemBytes == nil {
		t.Error("Cache item not found")
	} else {
		var cacheItem cache.CacheItem
		json.Unmarshal([]byte(cacheItemBytes), &cacheItem)

		if string(cacheItem.Content[:]) != value {
			t.Errorf("Incorrect cache item value. Expected: %s, Got: %s", value, cacheItem.Content)
		}
		if cacheItem.ID != id {
			t.Errorf("Incorrect cache item ID. Expected: %s, Got: %s", id, cacheItem.ID)
		}
	}
}

func TestFindAPIKey(t *testing.T) {
	userConfig := &config.Config{
		Supergraphs: []config.SupergraphConfig{
			{
				GraphRef:  "graph1",
				ApolloKey: "key1",
			},
			{
				GraphRef:  "graph2",
				ApolloKey: "key2",
			},
			{
				GraphRef:  "graph3",
				ApolloKey: "key3",
			},
		},
	}

	// Call the findAPIKey function
	graphRef := "graph2"
	apiKey, err := findAPIKey(userConfig, graphRef)
	if err != nil {
		t.Errorf("findAPIKey returned an error: %v", err)
	}

	// Verify the API key
	expectedKey := "key2"
	if apiKey != expectedKey {
		t.Errorf("Incorrect API key. Expected: %s, Got: %s", expectedKey, apiKey)
	}

	// Call with an invalid graph reference
	graphRef = "graph4"
	_, err = findAPIKey(userConfig, graphRef)
	if err == nil {
		t.Errorf("Expected error when finding API key with invalid graph reference")
	}
}

func TestDefaultHeaders(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	apiKey := "sampleKey"

	// Call the defaultHeaders function
	defaultHeaders(req, apiKey)

	// Verify the headers
	authHeader := req.Header.Get("x-api-key")
	expectedAuthHeader := "sampleKey"
	if authHeader != expectedAuthHeader {
		t.Errorf("Incorrect Authorization header. Expected: %s, Got: %s", expectedAuthHeader, authHeader)
	}

	clientNameHeader := req.Header.Get("apollo-client-name")
	expectedClientNameHeader := "UplinkRelay"
	if clientNameHeader != expectedClientNameHeader {
		t.Errorf("Incorrect client name header. Expected: %s, Got: %s", expectedClientNameHeader, clientNameHeader)
	}

	contentTypeHeader := req.Header.Get("Content-Type")
	expectedContentTypeHeader := "application/json"
	if contentTypeHeader != expectedContentTypeHeader {
		t.Errorf("Incorrect Content-Type header. Expected: %s, Got: %s", expectedContentTypeHeader, contentTypeHeader)
	}
}
