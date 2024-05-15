package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func startPolling(config *Config, cache *MemoryCache, httpClient *http.Client, enableDebug *bool) {
	// Log when polling starts
	debugLog(enableDebug, "Polling started")

	ticker := time.NewTicker(time.Duration(config.Polling.Interval) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		for graphRef, graphApiKey := range config.Supergraphs.GraphRefs {
			debugLog(enableDebug, "Polling for graph: %s", graphRef)

			// Split the graph into GraphID and VariantID
			parts := strings.Split(graphRef, "@")
			if len(parts) != 2 {
				log.Printf("Invalid GraphRef: %s", graphRef)
				continue
			}
			graphID, variantID, err := parseGraphRef(graphRef)
			if err != nil {
				log.Printf("Failed to parse GraphRef: %s", graphRef)
				continue
			}

			// Fetch the schema for the graph
			response, err := fetchSupergraphSdl(config, httpClient, graphRef, graphApiKey)
			if err != nil {
				log.Printf("Failed to fetch schema for graph %s: %v", graphRef, err)
				continue
			}
			// Extract the schema from the response
			schema := response.Data.RouterConfig.SupergraphSdl

			// Update the cache
			cacheKey := makeCacheKey(graphID, variantID, "SupergraphSdlQuery")
			// Set the cache using the fetched schema
			debugLog(enableDebug, "Updating SDL for GraphRef %s", graphRef)
			cache.Set(cacheKey, schema, config.Cache.Duration)

			// Fetch the router license
			licenseResponse, err := fetchRouterLicense(config, httpClient, graphRef, graphApiKey)
			if err != nil {
				log.Printf("Failed to fetch router license for graph %s: %v", graphRef, err)
				continue
			}
			// Extract the license from the response
			jwt := licenseResponse.Data.RouterEntitlements.Entitlement.Jwt

			// Update the cache
			cacheKey = makeCacheKey(graphID, variantID, "LicenseQuery")
			// Set the cache using the fetched license
			debugLog(enableDebug, "Updating license for GraphRef %s", graphRef)
			cache.Set(cacheKey, jwt, config.Cache.Duration)
		}
	}
}

func fetchSupergraphSdl(config *Config, httpClient *http.Client, graphRef string, apiKey string) (*UplinkSupergraphSdlResponse, error) {
	// Prepare the request body
	requestBody, err := json.Marshal(UplinkRelayRequest{
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
		log.Printf("Error preparing request body: %v", err)
		return nil, err
	}

	// Select the next uplink URL
	selector := NewRoundRobinSelector(config.Uplink.URLs)
	uplinkURL := selector.Next()

	// Create a new request using http
	req, err := http.NewRequest("POST", uplinkURL, bytes.NewBuffer(requestBody))
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return nil, err
	}

	// Uplink Relay Headers
	req.Header.Set("apollo-client-name", "UplinkRelay")
	req.Header.Set("apollo-client-version", "1.0")
	req.Header.Set("User-Agent", "UplinkRelay/1.0")
	req.Header.Set("Content-Type", "application/json")

	// Log the request details
	log.Printf("Request method: %s", req.Method)
	log.Printf("Request URL: %s", req.URL)
	log.Printf("Request headers: %v", req.Header)
	log.Printf("Request body: %s", requestBody)

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
		log.Printf("Empty response body")
		return nil, fmt.Errorf("empty response body")
	}

	// Log the raw response body
	log.Printf("Raw response body: %s", bodyBytes)

	// Decode the response body
	var response UplinkSupergraphSdlResponse
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

func fetchRouterLicense(config *Config, httpClient *http.Client, graphRef string, apiKey string) (*UplinkLicenseResponse, error) {
	// Define the request body
	requestBody, err := json.Marshal(UplinkRelayRequest{
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
	selector := NewRoundRobinSelector(config.Uplink.URLs)
	uplinkURL := selector.Next()

	// Create a new request using http
	req, err := http.NewRequest("POST", uplinkURL, bytes.NewBuffer(requestBody))
	if err != nil {
		log.Printf("Error creating request: %v", err)
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
		log.Printf("Error on response.\n[ERROR] - %v", err)
		return nil, err
	}

	// Read the response body
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Unmarshal the response body into the LicenseQueryResponse struct
	var response UplinkLicenseResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}
