package util

import (
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/uplink"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// UplinkRelayRequest struct
type UplinkRelayRequest struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
	OperationName string                 `json:"operationName"`
}

func UplinkRequest(userConfig *config.Config, logger *slog.Logger, query string, variables map[string]interface{}, operationName string) ([]byte, error) {
	httpClient := http.DefaultClient
	httpClient.Timeout = time.Duration(userConfig.Uplink.Timeout) * time.Second

	// Select the next uplink URL
	selector := uplink.NewRoundRobinSelector(userConfig.Uplink.URLs)
	uplinkURL := selector.Next()
	body := &UplinkRelayRequest{
		Query:         query,
		Variables:     variables,
		OperationName: operationName,
	}

	requestBody, err := json.Marshal(body)
	if err != nil {
		logger.Error("Error preparing request body", "err", err)
		return nil, err
	}

	// Create a new request using http
	req, err := http.NewRequest("POST", uplinkURL, bytes.NewBuffer(requestBody))
	if err != nil {
		logger.Error("Error creating request", "err", err)
		return nil, err
	}

	// Set the request headers
	req.Header.Set("apollo-client-name", "UplinkRelay")
	req.Header.Set("apollo-client-version", "1.0")
	req.Header.Set("User-Agent", "UplinkRelay/1.0")
	req.Header.Set("Content-Type", "application/json")

	// Send the request using the http Client
	resp, err := httpClient.Do(req)
	if err != nil {
		logger.Error("Error on response", "err", err)
		return nil, err
	}

	// Check if the response status code is not 200
	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	// Read the response body
	bodyBytes, _ := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	// Check if the response body is empty
	if len(bodyBytes) == 0 {
		logger.Error("Empty response body")
		return nil, fmt.Errorf("empty response body")
	}
	return bodyBytes, nil
}
