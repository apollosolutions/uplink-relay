package util

import (
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/logger"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUplinkRequest(t *testing.T) {
	testConfig := config.NewDefaultConfig()

	// Create a new test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle the request and send a response
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "Test response"}`))
	}))
	defer server.Close()

	testConfig.Uplink.URLs = []string{server.URL}

	// Set the test server URL in the config
	testConfig.Uplink.URLs = []string{server.URL}
	// Create a sample logger
	logger := logger.MakeLogger(nil)

	// Define sample input values
	query := "query Test {__typename}"
	variables := map[string]interface{}{
		"limit": 10,
	}
	operationName := "Test"

	// Call the UplinkRequest function
	response, err := UplinkRequest(testConfig, logger, query, variables, operationName)

	// Check if there was an error
	if err != nil {
		t.Errorf("UplinkRequest returned an error: %v", err)
	}

	// Check if the response is not empty
	if len(response) == 0 {
		t.Errorf("UplinkRequest returned an empty response")
	}
}
