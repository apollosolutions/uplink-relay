package polling

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"time"

	Cache "apollosolutions/uplink-relay/cache"
	Config "apollosolutions/uplink-relay/config"
	Proxy "apollosolutions/uplink-relay/proxy"
	Uplink "apollosolutions/uplink-relay/uplink"
)

// startPolling starts polling for updates at the specified interval.
func StartPolling(config *Config.Config, cache Cache.Cache, httpClient *http.Client, logger *slog.Logger) {
	// Log when polling starts
	logger.Debug("Polling started")

	// Create a new ticker with the polling interval
	ticker := time.NewTicker(time.Duration(config.Polling.Interval) * time.Second)
	// Stop the ticker when the function returns
	defer ticker.Stop()

	// Poll for updates at the specified interval
	for range ticker.C {
		for _, supergraphConfig := range config.Supergraphs {
			// Poll for the graph
			success := false
			for i := 0; i < config.Polling.RetryCount && !success; i++ {
				logger.Debug("Polling for graph", "graphRef", supergraphConfig.GraphRef)

				// Split the graph into GraphID and VariantID
				parts := strings.Split(supergraphConfig.GraphRef, "@")
				if len(parts) != 2 {
					logger.Error("Invalid GraphRef", "graphRef", supergraphConfig.GraphRef)
					break
				}
				graphID, variantID, err := Proxy.ParseGraphRef(supergraphConfig.GraphRef)
				if err != nil {
					logger.Error("Failed to parse GraphRef", "graphRef", supergraphConfig.GraphRef)
					break
				}

				// Fetch the schema for the graph
				response, err := fetchSupergraphSdl(config, httpClient, supergraphConfig.GraphRef, supergraphConfig.ApolloKey, logger)
				if err != nil {
					logger.Error("Failed to fetch schema for graph", "graphRef", supergraphConfig.GraphRef, "err", err)
					break
				}
				// Extract the schema from the response
				schema := response.Data.RouterConfig.SupergraphSdl

				// Update the cache
				cacheKey := Cache.MakeCacheKey(graphID, variantID, "SupergraphSdlQuery")
				// Set the cache using the fetched schema
				logger.Debug("Updating SDL for GraphRef", "graphRef", supergraphConfig.GraphRef)
				cache.Set(cacheKey, schema, config.Cache.Duration)

				// Fetch the router license
				licenseResponse, err := fetchRouterLicense(config, httpClient, supergraphConfig.GraphRef, supergraphConfig.ApolloKey, logger)
				if err != nil {
					logger.Error("Failed to fetch router license for graph %s: %v", supergraphConfig.GraphRef, err)
					break
				}
				// Extract the license from the response
				jwt := licenseResponse.Data.RouterEntitlements.Entitlement.Jwt

				// Update the cache
				cacheKey = Cache.MakeCacheKey(graphID, variantID, "LicenseQuery")
				// Set the cache using the fetched license
				logger.Debug("Updating license for GraphRef", "graphRef", supergraphConfig.GraphRef)
				cache.Set(cacheKey, jwt, config.Cache.Duration)

				// If successful, log the success
				logger.Info("Successfully polled for graph", "graphRef", supergraphConfig.GraphRef)
				success = true
			}
			if !success {
				logger.Error("Failed to poll uplink for graph", "graphRef", supergraphConfig.GraphRef, "retries", config.Polling.RetryCount)
			}
		}
	}
}

// fetchSupergraphSdl fetches the supergraph SDL for the specified graph.
func fetchSupergraphSdl(config *Config.Config, httpClient *http.Client, graphRef string, apiKey string, logger *slog.Logger) (*Proxy.UplinkSupergraphSdlResponse, error) {
	// Prepare the request body
	requestBody, err := json.Marshal(Proxy.UplinkRelayRequest{
		Variables: map[string]interface{}{
			"apiKey":    apiKey,
			"graph_ref": graphRef,
			"ifAfterId": nil,
		},
		Query: `query SupergraphSdlQuery($apiKey: String!, $graph_ref: String!, $ifAfterId: ID) {
					routerConfig(ref: $graph_ref, apiKey: $apiKey, ifAfterId: $ifAfterId) {
							__typename
							... on RouterConfigResult {
									id
									supergraphSdl: supergraphSDL
									minDelaySeconds
							}
							... on Unchanged {
									id
									minDelaySeconds
							}
							... on FetchError {
									code
									message
							}
					}
			}`,
		OperationName: "SupergraphSdlQuery",
	})
	if err != nil {
		logger.Error("Error preparing request body", "err", err)
		return nil, err
	}

	// Select the next uplink URL
	selector := Uplink.NewRoundRobinSelector(config.Uplink.URLs)
	uplinkURL := selector.Next()

	// Create a new request using http
	req, err := http.NewRequest("POST", uplinkURL, bytes.NewBuffer(requestBody))
	if err != nil {
		logger.Error("Error creating request: %v", err)
		return nil, err
	}

	// Uplink Relay Headers
	req.Header.Set("apollo-client-name", "UplinkRelay")
	req.Header.Set("apollo-client-version", "1.0")
	req.Header.Set("User-Agent", "UplinkRelay/1.0")
	req.Header.Set("Content-Type", "application/json")

	// Log the request details
	logger.Info("Request method", "method", req.Method)
	logger.Info("Request URL", "url", req.URL)
	logger.Info("Request headers", "header", req.Header)
	logger.Info("Request body", "body", requestBody)

	// Send req using http Client
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("Error on response.\n[ERROR] - %v", err)
		return nil, err
	}

	// Read the response body
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Check if the response body is empty
	if len(bodyBytes) == 0 {
		logger.Error("Empty response body")
		return nil, fmt.Errorf("empty response body")
	}

	// Log the raw response body
	logger.Debug("Raw response body", "body", bodyBytes)

	// Decode the response body
	var response Proxy.UplinkSupergraphSdlResponse
	decodeErr := json.Unmarshal(bodyBytes, &response)
	if decodeErr != nil {
		return nil, fmt.Errorf("failed to decode response body: %w", decodeErr)
	}
	// Use bytes.NewBuffer to create a new reader, since resp.Body has been read
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Check if the response status code is not 200
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	// Return the response
	return &response, nil
}

// fetchRouterLicense fetches the router license for the specified graph.
func fetchRouterLicense(config *Config.Config, httpClient *http.Client, graphRef string, apiKey string, logger *slog.Logger) (*Proxy.UplinkLicenseResponse, error) {
	// Define the request body
	requestBody, err := json.Marshal(Proxy.UplinkRelayRequest{
		Variables: map[string]interface{}{
			"apiKey":    apiKey,
			"graph_ref": graphRef,
		},
		Query: `query LicenseQuery($apiKey: String!, $graph_ref: String!, $ifAfterId: ID) {
			routerEntitlements(ifAfterId: $ifAfterId, apiKey: $apiKey, ref: $graph_ref) {
					__typename
					... on RouterEntitlementsResult {
							id
							minDelaySeconds
							entitlement {
									jwt
							}
					}
					... on Unchanged {
							id
							minDelaySeconds
					}
					... on FetchError {
							code
							message
					}
			}
		}`,
		OperationName: "LicenseQuery",
	})
	if err != nil {
		return nil, err
	}

	// Select the next uplink URL
	selector := Uplink.NewRoundRobinSelector(config.Uplink.URLs)
	uplinkURL := selector.Next()

	// Create a new request using http
	req, err := http.NewRequest("POST", uplinkURL, bytes.NewBuffer(requestBody))
	if err != nil {
		logger.Error("Error creating request: %v", err)
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
		logger.Error("Error on response.\n[ERROR] - %v", err)
		return nil, err
	}

	// Read the response body
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Unmarshal the response body into the LicenseQueryResponse struct
	var response Proxy.UplinkLicenseResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}
