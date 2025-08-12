package webhooks

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/logger"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookHandler(t *testing.T) {
	var truePointer = true
	// Create a new test logger
	logger := logger.MakeLogger(&truePointer)

	// Create a new test cache
	cache := cache.NewMemoryCache(10)

	// Create a new test HTTP client
	httpClient := http.DefaultClient

	// Create a new test request
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{"eventType":"schema-change","eventID":"1234","changes":[{"description":"Type User added"}],"schemaURL":"https://example.com/schema","schemaURLExpiresAt":"2022-01-01T00:00:00Z","graphID":"1234","variantID":"1234@default","timestamp":"2022-01-01T00:00:00Z"}`))

	req.Header.Set("x-apollo-signature", "sha256=16dcf032fab9acbadf14ecd2ff8beed88da151aa7f0e2c145377a892db5b2945")

	// Create a new test response recorder
	w := httptest.NewRecorder()

	// Create a new test configuration
	config := &config.Config{
		Webhook: config.WebhookConfig{
			Secret: "secret",
		},
		Cache: config.CacheConfig{
			Enabled:  &truePointer,
			MaxSize:  10,
			Duration: -1,
		},
		Supergraphs: []config.SupergraphConfig{
			{
				GraphRef:  "1234@default",
				ApolloKey: "key",
			},
		},
	}

	// Call the webhook handler
	handler := WebhookHandler(config, cache, httpClient, logger)
	handler(w, req)
	// Check that the response status code is 200
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", w.Code)
	}
	// Check that the cache was updated
	if _, ok := cache.Get("1234:default:SupergraphSdlQuery"); !ok {
		t.Errorf("Expected cache key 1234:default:SupergraphSdlQuery to be set")
	}
}
