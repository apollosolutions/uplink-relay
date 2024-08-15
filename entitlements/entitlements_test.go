package entitlements

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/logger"
)

func TestFetchRouterLicense(t *testing.T) {
	userConfig := config.NewDefaultConfig()
	userConfig.Supergraphs = []config.SupergraphConfig{
		{
			GraphRef:  "example-graph@current",
			ApolloKey: "1234",
		},
	}

	systemCache := cache.NewMemoryCache(1000)
	logger := logger.MakeLogger(nil)

	// Create a new test server to mock the Uplink API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle the request and send a response
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{"routerEntitlements":{"__typename":"RouterEntitlementsResult","id":"2024-10-03T12:00:00Z","minDelaySeconds":60,"entitlement":{"jwt":"jwt"}}}}`))
	}))
	defer server.Close()

	userConfig.Uplink.URLs = []string{server.URL}

	// Test case 1: Fetching a valid router license
	graphRef := "example-graph@current"
	err := FetchRouterLicense(userConfig, systemCache, logger, graphRef)
	if err != nil {
		t.Errorf("Failed to fetch router license: %v", err)
	}

	// Test case 2: Fetching a router license with an invalid graph reference
	invalidGraphRef := "invalid-graph"
	err = FetchRouterLicense(userConfig, systemCache, logger, invalidGraphRef)
	if err == nil {
		t.Errorf("Expected error when fetching router license with invalid graph reference")
	}

	// Test case 3: Fetching a router license with expired cache
	expiredGraphRef := "example-graph@current"
	systemCache.Set(expiredGraphRef, "expired-license", -10)
	err = FetchRouterLicense(userConfig, systemCache, logger, expiredGraphRef)
	if err != nil {
		t.Errorf("Failed to fetch router license with expired cache: %v", err)
	}

	// Test case 4: Fetching a router license with invalid user configuration
	invalidUserConfig := &config.Config{}
	err = FetchRouterLicense(invalidUserConfig, systemCache, logger, graphRef)
	if err == nil {
		t.Errorf("Expected error when fetching router license with invalid user configuration")
	}
}
