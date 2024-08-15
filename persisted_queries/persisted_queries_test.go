package persistedqueries

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/logger"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPersistedQueryHandler(t *testing.T) {
	pT := true
	log := logger.MakeLogger(&pT)
	mockCache := cache.NewMemoryCache(1000)
	mockConfig := config.NewDefaultConfig()
	mockConfig.Relay.PublicURL = "http://example.com/"

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"persistedQueries":{"id":"123","__typename":"","minDelaySeconds":0,"chunks":null}}}`))
	}))
	defer mockServer.Close()

	// Prefill cache with test data
	_, err := CachePersistedQueryChunkData(mockConfig, log, mockCache, []UplinkPersistedQueryChunk{{
		ID:   "123",
		URLs: []string{mockServer.URL},
	}})
	if err != nil {
		t.Fatal(err)
	}

	// Test case 1: Valid request with existing persisted query
	req1, err := http.NewRequest("GET", "/persisted-queries/123?i=0", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr1 := httptest.NewRecorder()
	handler1 := http.HandlerFunc(PersistedQueryHandler(log, http.DefaultClient, mockCache))
	handler1.ServeHTTP(rr1, req1)
	if status := rr1.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v, want %v", status, http.StatusOK)
	}
	expectedResponse1 := `{"data":{"persistedQueries":{"id":"123","__typename":"","minDelaySeconds":0,"chunks":null}}}`
	if rr1.Body.String() != expectedResponse1 {
		t.Errorf("Handler returned unexpected body: got %v, want %v", rr1.Body.String(), expectedResponse1)
	}

	// Test case 2: Invalid request with non-existent persisted query
	req2, err := http.NewRequest("GET", "/persisted-queries/456?i=0", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr2 := httptest.NewRecorder()
	handler2 := http.HandlerFunc(PersistedQueryHandler(log, http.DefaultClient, mockCache))
	handler2.ServeHTTP(rr2, req2)
	if status := rr2.Code; status != http.StatusNotFound {
		t.Errorf("Handler returned wrong status code: got %v, want %v", status, http.StatusNotFound)
	}
	expectedResponse2 := `{"error":"Manifest not found"}`
	if strings.TrimSpace(rr2.Body.String()) != expectedResponse2 {
		t.Errorf("Handler returned unexpected body: got %v, want %v", rr2.Body.String(), expectedResponse2)
	}

	// Test case 3: Invalid request with incorrect path format
	req3, err := http.NewRequest("GET", "/persisted-queries/789", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr3 := httptest.NewRecorder()
	handler3 := http.HandlerFunc(PersistedQueryHandler(log, http.DefaultClient, mockCache))
	handler3.ServeHTTP(rr3, req3)
	if status := rr3.Code; status != http.StatusBadRequest {
		t.Errorf("Handler returned wrong status code: got %v, want %v", status, http.StatusBadRequest)
	}
	expectedResponse3 := `{"error":"Invalid path format"}`
	if strings.TrimSpace(rr3.Body.String()) != expectedResponse3 {
		t.Errorf("Handler returned unexpected body: got %v, want %v", rr3.Body.String(), expectedResponse3)
	}

	// Test case 4: check if the publicURL has an existing path (e.g. example.com/pq/) whether that'll also work
	mockConfig.Relay.PublicURL = "http://example.com/pq/"
	// Prefill cache with test data
	_, err = CachePersistedQueryChunkData(mockConfig, log, mockCache, []UplinkPersistedQueryChunk{{
		ID:   "123",
		URLs: []string{mockServer.URL},
	}})
	if err != nil {
		t.Fatal(err)
	}
	req4, err := http.NewRequest("GET", "/pq/persisted-queries/123?i=0", nil)
	if err != nil {
		t.Fatal(err)
	}
	rr4 := httptest.NewRecorder()
	handler4 := http.HandlerFunc(PersistedQueryHandler(log, http.DefaultClient, mockCache))
	handler4.ServeHTTP(rr4, req4)
	if status := rr4.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v, want %v", status, http.StatusOK)
	}

	// Test case 5: check if the cache is skipped when the publicURL is empty
	mockConfig.Relay.PublicURL = ""
	// Reset cache
	mockCache = cache.NewMemoryCache(1000)
	// Attempt to prefill cache with test data
	_, err = CachePersistedQueryChunkData(mockConfig, log, mockCache, []UplinkPersistedQueryChunk{{
		ID:   "123",
		URLs: []string{mockServer.URL},
	}})

	if err != nil {
		t.Fatal(err)
	}

	req5, err := http.NewRequest("GET", "/persisted-queries/123?i=0", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr5 := httptest.NewRecorder()
	handler5 := http.HandlerFunc(PersistedQueryHandler(log, http.DefaultClient, mockCache))
	handler5.ServeHTTP(rr5, req5)
	if status := rr5.Code; status != http.StatusNotFound {
		t.Errorf("Handler returned wrong status code: got %v, want %v", status, http.StatusNotFound)
	}
	_, found := mockCache.Get("pq:123:0")
	if found {
		t.Errorf("Expected item to not be found in cache")
	}
}

func TestCachePersistedQueryChunkData(t *testing.T) {
	log := logger.MakeLogger(nil)
	mockCache := cache.NewMemoryCache(1000)
	mockConfig := config.NewDefaultConfig()
	mockConfig.Relay.PublicURL = "http://example.com"
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"format":"apollo-persisted-query-manifest","version":1,"operations":[{"id":"1234","body":"query{__typename}"}]}`))
	}))
	// Test case 1: Cache persisted query chunk data successfully
	chunks := []UplinkPersistedQueryChunk{{
		ID:   "456",
		URLs: []string{mockServer.URL},
	}}
	cachedChunks, err := CachePersistedQueryChunkData(mockConfig, log, mockCache, chunks)
	if err != nil {
		t.Fatal(err)
	}
	if len(cachedChunks) != len(chunks) {
		t.Errorf("Cached chunks length mismatch: got %v, want %v", len(cachedChunks), len(chunks))
	}
	for i, chunk := range cachedChunks {
		if chunk.ID != chunks[i].ID {
			t.Errorf("Cached chunk ID mismatch: got %v, want %v", chunk.ID, chunks[i].ID)
		}
		if len(chunk.URLs) != len(chunks[i].URLs) {
			t.Errorf("Cached chunk URLs length mismatch: got %v, want %v", len(chunk.URLs), len(chunks[i].URLs))
		}
		for j, url := range chunk.URLs {
			if url != chunks[i].URLs[j] {
				t.Errorf("Cached chunk URL mismatch: got %v, want %v", url, chunks[i].URLs[j])
			}
		}
	}

	// Test case 2: Invalid URL causes an error
	// Missing protocol
	mockConfig.Relay.PublicURL = "example.com"
	chunks = []UplinkPersistedQueryChunk{{
		ID:   "789",
		URLs: []string{mockServer.URL},
	}}
	_, err = CachePersistedQueryChunkData(mockConfig, log, mockCache, chunks)
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestMakePersistedQueryCacheKey(t *testing.T) {
	// Test case 1: Valid input
	id := "123"
	index := "0"
	expectedKey := "pq:123:0"
	result := makePersistedQueryCacheKey(id, index)
	if result != expectedKey {
		t.Errorf("Unexpected cache key: got %v, want %v", result, expectedKey)
	}

	// Test case 2: Empty input
	id = ""
	index = ""
	expectedKey = "pq::"
	result = makePersistedQueryCacheKey(id, index)
	if result != expectedKey {
		t.Errorf("Unexpected cache key: got %v, want %v", result, expectedKey)
	}

	// Test case 3: Input with special characters
	id = "abc!@#$%^&*()"
	index = "1"
	expectedKey = "pq:abc!@#$%^&*():1"
	result = makePersistedQueryCacheKey(id, index)
	if result != expectedKey {
		t.Errorf("Unexpected cache key: got %v, want %v", result, expectedKey)
	}
}
func TestFetchPQManifest(t *testing.T) {
	log := logger.MakeLogger(nil)
	mockCache := cache.NewMemoryCache(1000)
	mockConfig := config.NewDefaultConfig()
	mockConfig.Relay.PublicURL = "http://example.com"
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"persistedQueries":{"__typename":"PersistedQueriesResult","id":"c42e4925-6678-497d-a1ed-e819dc039e34:1","minDelaySeconds":60,"chunks":[{"id":"graph/1/1","urls":["https://jsonplaceholder.typicode.com/todos/1"]}]}}}`))
	}))
	defer mockServer.Close()

	mockConfig.Uplink.URLs = []string{mockServer.URL}
	mockConfig.Supergraphs = []config.SupergraphConfig{{
		GraphRef:  "graph@variant",
		ApolloKey: "1234",
	}}

	// Test case 1: Fetch PQ manifest successfully
	err := FetchPQManifest(mockConfig, mockCache, log, "graph@variant", "")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Test case 2: Fetch PQ manifest with invalid graph reference
	err = FetchPQManifest(mockConfig, mockCache, log, "", "")
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// Test case 3: Fetch PQ manifest with non-existent manifest URL
	mockConfig.Relay.PublicURL = "http://example.com/non-existent"
	err = FetchPQManifest(mockConfig, mockCache, log, "graph1", "")
	if err == nil {
		t.Error("Expected error, got nil")
	}
}
func TestDecodeID(t *testing.T) {
	// Test case 1: Valid ID and index
	id := "c42e4925-6678-497d-a1ed-e819dc039e34:1"
	expectedID := "c42e4925-6678-497d-a1ed-e819dc039e34"
	expectedIndex := 1
	resultID, resultIndex := DecodeID(id)
	if resultID != expectedID {
		t.Errorf("Unexpected ID: got %v, want %v", resultID, expectedID)
	}
	if resultIndex != expectedIndex {
		t.Errorf("Unexpected index: got %v, want %v", resultIndex, expectedIndex)
	}

	// Test case 2: Empty ID and index
	id = ""
	expectedID = ""
	expectedIndex = -1
	resultID, resultIndex = DecodeID(id)
	if resultID != expectedID {
		t.Errorf("Unexpected ID: got %v, want %v", resultID, expectedID)
	}
	if resultIndex != expectedIndex {
		t.Errorf("Unexpected index: got %v, want %v", resultIndex, expectedIndex)
	}

	// Test case 3: ID without index
	id = "c42e4925-6678-497d-a1ed-e819dc039e34"
	expectedID = ""
	expectedIndex = -1
	resultID, resultIndex = DecodeID(id)
	if resultID != expectedID {
		t.Errorf("Unexpected ID: got %v, want %v", resultID, expectedID)
	}
	if resultIndex != expectedIndex {
		t.Errorf("Unexpected index: got %v, want %v", resultIndex, expectedIndex)
	}

	// Test case 4: ID with invalid index
	id = "c42e4925-6678-497d-a1ed-e819dc039e34:abc"
	expectedID = ""
	expectedIndex = -1
	resultID, resultIndex = DecodeID(id)
	if resultID != expectedID {
		t.Errorf("Unexpected ID: got %v, want %v", resultID, expectedID)
	}
	if resultIndex != expectedIndex {
		t.Errorf("Unexpected index: got %v, want %v", resultIndex, expectedIndex)
	}
}
