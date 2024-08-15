package pinning

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/logger"
	"testing"
)

func TestPinOfflineLicense(t *testing.T) {
	// Create a mock user config
	userConfig := config.NewDefaultConfig()

	// Create a mock logger
	logger := logger.MakeLogger(nil)

	// Create a mock system cache
	systemCache := cache.NewMemoryCache(10)

	// Set the license and graphRef for the test
	// The test JWT is entirely invalid for an actual router, but does allow us to validate the Jose logic
	license := "eyJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJURVNUIiwic3ViIjoiVEVTVCIsImF1ZCI6IlRFU1QiLCJ3YXJuQXQiOjE3MjY3NDcyMDAwLCJoYWx0QXQiOjE3Mjc5NTY4MDAwfQ.lm5WHWovWWV2q0Ipo8GCjDyTteBBmKBhQwGDP3Fsp7A"
	graphRef := "test-graph-ref"

	err := PinOfflineLicense(userConfig, logger, systemCache, license, graphRef)
	if err != nil {
		t.Errorf("Failed to pin offline license: %v", err)
	}
}
