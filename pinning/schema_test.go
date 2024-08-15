package pinning

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/logger"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPinLaunchID(t *testing.T) {
	userConfig := config.NewDefaultConfig()
	userConfig.Supergraphs = []config.SupergraphConfig{
		{
			GraphRef:  "graphID@variantID",
			ApolloKey: "1234",
		},
	}

	// Create a new test server to mock the Platform API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle the request and send a response
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{"graph":{"variant":{"id":"graphID@variantID","launch":{"completedAt":"2024-08-05T19:53:30.358994000Z","build":{"result":{"__typename":"BuildSuccess","coreSchema":{"coreDocument":"sampleSchema"}}}}}}}}`))
	}))
	defer server.Close()

	userConfig.Uplink.StudioAPIURL = server.URL

	logger := logger.MakeLogger(nil)
	cache := cache.NewMemoryCache(10)

	// Set up test data
	launchID := "12345"
	graphRef := "graphID@variantID"

	// Call the function being tested
	err := PinLaunchID(userConfig, logger, cache, launchID, graphRef)

	// Check if an error occurred
	if err != nil {
		t.Errorf("PinLaunchID returned an error: %v", err)
	}
}
