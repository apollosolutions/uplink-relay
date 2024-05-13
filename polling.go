package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func startPolling(config *Config, cache *MemoryCache, httpClient *http.Client, enableDebug *bool) {
	ticker := time.NewTicker(time.Duration(config.Polling.Interval))
	defer ticker.Stop()

	for range ticker.C {
		for _, graph := range config.Graphs.GraphRefs {
			// Split the graph into GraphID and VariantID
			parts := strings.Split(graph, "@")
			if len(parts) != 2 {
				log.Printf("Invalid graph: %s", graph)
				continue
			}
			graphID, variantID := parts[0], parts[1]

			// Fetch the schema for the graph
			schema, err := fetchSchema(config, httpClient, graphID, variantID)
			if err != nil {
				log.Printf("Failed to fetch schema for graph %s: %v", graph, err)
				continue
			}

			// Update the cache
			cacheKey := fmt.Sprintf("%s:%s", graphID, variantID)
			ttl := config.Cache.Duration
			if ttl == 0 {
				ttl = -1 // -1 represents "indefinite"
			}
			cache.Set(cacheKey, schema, ttl)
		}
	}
}

func fetchSchema(config *Config, httpClient *http.Client, graphRef string, apiKey string) (*UplinkResponse, error) {
	// Prepare the request body
	requestBody, err := json.Marshal(map[string]string{
		"variables": fmt.Sprintf(`{"apiKey":"%s","graph_ref":"%s","ifAfterId": null}`, apiKey, graphRef),
		"query": `query SupergraphSdlQuery($apiKey: String!, $graph_ref: String!, $ifAfterId: ID) {
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
		"operationName": "SupergraphSdlQuery",
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

	// Headers
	req.Header.Set("User-Agent", "myClient")
	req.Header.Set("Content-Type", "application/json")

	// Send req using http Client
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error on response.\n[ERROR] - %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	var uplinkResponse UplinkResponse
	decodeErr := json.NewDecoder(resp.Body).Decode(&uplinkResponse)
	if decodeErr != nil {
		return nil, fmt.Errorf("failed to decode response body: %w", decodeErr)
	}

	return &uplinkResponse, nil
}
