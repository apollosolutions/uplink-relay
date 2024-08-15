package schema

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/logger"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchSchema(t *testing.T) {
	// Create a new test server to mock the Platform API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle the request and send a response
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{"routerConfig":{"__typename":"RouterConfigResult","id":"2024-08-05T19:53:29.140664000Z","supergraphSdl":"schema","minDelaySeconds":30}}}`))
	}))
	defer server.Close()

	// Create a mock userConfig, systemCache, logger, and graphRef
	userConfig := config.NewDefaultConfig()
	userConfig.Uplink.URLs = []string{server.URL}
	userConfig.Supergraphs = []config.SupergraphConfig{
		{
			GraphRef:  "example-graph@variant",
			ApolloKey: "1234",
		},
	}

	systemCache := cache.NewMemoryCache(10)
	logger := logger.MakeLogger(nil)
	graphRef := "example-graph@variant"

	// Call the FetchSchema function
	err := FetchSchema(userConfig, systemCache, logger, graphRef)

	// Check if an error occurred
	if err != nil {
		t.Errorf("FetchSchema returned an error: %v", err)
	}
}
