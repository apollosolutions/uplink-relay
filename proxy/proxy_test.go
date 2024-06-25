package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/logger"
	"apollosolutions/uplink-relay/uplink"
)

const licenseQuery = `{"variables":{"apiKey":"service:graph:1234","graph_ref":"graph@local","ifAfterId":null},"query":"query LicenseQuery($apiKey: String!, $graph_ref: String!, $ifAfterId: ID) {\n\n    routerEntitlements(ifAfterId: $ifAfterId, apiKey: $apiKey, ref: $graph_ref) {\n        __typename\n        ... on RouterEntitlementsResult {\n            id\n            minDelaySeconds\n            entitlement {\n                jwt\n            }\n        }\n        ... on Unchanged {\n            id\n            minDelaySeconds\n        }\n        ... on FetchError {\n            code\n            message\n        }\n    }\n}\n","operationName":"LicenseQuery"}`
const licenseResponse = `{"data":{"routerEntitlements":{"__typename":"RouterEntitlementsResult","id":"2024-08-02T12:00:00Z","minDelaySeconds":60.0,"entitlement":{"jwt":"bob"}}}}`

func TestRelayHandler(t *testing.T) {
	// Create a mock HTTP server for testing
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock the response from the uplink service
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(licenseResponse))
	}))
	defer mockServer.Close()

	// Create a mock cache
	mockCache := cache.NewMemoryCache(10)

	// Create a mock config
	mockConfig := &config.Config{
		Uplink: config.UplinkConfig{
			URLs: []string{mockServer.URL},
		},
		Cache: config.CacheConfig{
			Enabled:  true,
			Duration: 50000,
		},
	}

	// Create a mock round-robin selector
	mockRRSelector := uplink.NewRoundRobinSelector([]string{mockServer.URL})

	// Create a mock HTTP client
	mockHTTPClient := &http.Client{}

	// Create a mock logger
	pFalse := false
	mockLogger := logger.MakeLogger(&pFalse)

	// Create a new test request
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(licenseQuery))

	// Create a response recorder to capture the response
	rr := httptest.NewRecorder()

	// Call the RelayHandler function
	handler := RelayHandler(mockConfig, mockCache, mockRRSelector, mockHTTPClient, mockLogger)
	handler.ServeHTTP(rr, req)
	// Assert that the response status code is 200
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code 200, but got %d", rr.Code)
	}
	// Assert that the response body matches the expected value
	if rr.Body.String() != licenseResponse {
		t.Errorf("Expected response body '%s', but got '%s'", licenseResponse, rr.Body.String())
	}
	var key = cache.MakeCacheKey("graph", "local", "LicenseQuery")
	if key != "graph:local:LicenseQuery" {
		t.Errorf("Expected cache key 'graph:local:LicenseQuery', but got '%s'", key)
	}
	// Assert that the response body is cached
	if _, ok := mockCache.Get(key); !ok {
		t.Errorf("Expected response body to be cached, but it was not")
	}

	// Test when the request body is nil
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status code 400, but got %d", rr.Code)
	}

}

func TestHandleCacheHit(t *testing.T) {
	// Create a mock logger
	pFalse := false
	mockLogger := logger.MakeLogger(&pFalse)

	// Create a new test request
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(licenseQuery))

	// Create a response recorder to capture the response
	rr := httptest.NewRecorder()

	// Call the handleCacheHit function
	err := handleCacheHit(cache.MakeCacheKey("graph", "local", "LicenseQuery"), []byte(licenseResponse), mockLogger)(rr, req)
	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}

	// Assert that the response status code is 200
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code 200, but got %d", rr.Code)
	}

	// Recreate the test request with a nil body
	req = httptest.NewRequest(http.MethodPost, "/", nil)

	// Call the handleCacheHit again for the SupergraphQuery
	err = handleCacheHit(cache.MakeCacheKey("graph", "local", "SupergraphSdlQuery"), []byte("1234"), mockLogger)(rr, req)
	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}
}
