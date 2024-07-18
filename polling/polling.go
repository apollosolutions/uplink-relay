package polling

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	persistedqueries "apollosolutions/uplink-relay/persisted_queries"
	"apollosolutions/uplink-relay/proxy"
	"apollosolutions/uplink-relay/uplink"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// startPolling starts polling for updates at the specified interval.
func StartPolling(userConfig *config.Config, systemCache cache.Cache, httpClient *http.Client, logger *slog.Logger) {
	// Log when polling starts
	logger.Debug("Polling started")

	if !userConfig.Polling.Enabled {
		logger.Debug("Polling is disabled")
		return
	}

	if userConfig.Polling.Interval > 0 {
		// Create a new ticker with the polling interval
		ticker := time.NewTicker(time.Duration(userConfig.Polling.Interval) * time.Second)
		// Stop the ticker when the function returns
		defer ticker.Stop()

		// Poll for updates at the specified interval
		for range ticker.C {
			logger.Debug("Polling for updates")
			pollForUpdates(userConfig, systemCache, httpClient, logger)
		}
	}

	if len(userConfig.Polling.Expressions) > 0 {
		crons := cron.New()
		for _, expression := range userConfig.Polling.Expressions {
			_, err := cron.ParseStandard(expression)
			if err != nil {
				logger.Error("Failed to parse cron expression", "expression", expression)
				return
			}

			// Add a new cron job to poll for updates
			crons.AddFunc(expression, func() {
				pollForUpdates(userConfig, systemCache, httpClient, logger)
			})
		}
		// Start the cron schedule
		crons.Start()
	}

}

func pollForUpdates(userConfig *config.Config, systemCache cache.Cache, httpClient *http.Client, logger *slog.Logger) {
	for _, supergraphConfig := range userConfig.Supergraphs {
		// Poll for the graph
		success := false
		for i := 0; i < userConfig.Polling.RetryCount && !success; i++ {
			logger.Debug("Polling for graph", "graphRef", supergraphConfig.GraphRef)
			logger.Debug("Options enabled", "supergraph", *userConfig.Polling.Supergraph, "entitlements", *userConfig.Polling.Entitlements, "persistedQueries", *userConfig.Polling.PersistedQueries)
			// Split the graph into GraphID and VariantID
			parts := strings.Split(supergraphConfig.GraphRef, "@")
			if len(parts) != 2 {
				logger.Error("Invalid GraphRef", "graphRef", supergraphConfig.GraphRef)
				break
			}
			graphID, variantID, err := proxy.ParseGraphRef(supergraphConfig.GraphRef)
			if err != nil {
				logger.Error("Failed to parse GraphRef", "graphRef", supergraphConfig.GraphRef)
				break
			}

			// Fetch the schema for the graph
			if *userConfig.Polling.Supergraph {
				logger.Debug("Polling for supergraph", "graphRef", supergraphConfig.GraphRef)
				response, err := fetchSupergraphSdl(userConfig, httpClient, supergraphConfig.GraphRef, supergraphConfig.ApolloKey, logger)
				if err != nil {
					logger.Error("Failed to fetch schema", "graphRef", supergraphConfig.GraphRef, "err", err)
					break
				}
				// Extract the schema from the response
				schema := response.Data.RouterConfig.SupergraphSdl

				// Update the cache
				cacheKey := cache.MakeCacheKey(graphID, variantID, "SupergraphSdlQuery", map[string]interface{}{"apiKey": supergraphConfig.ApolloKey, "graph_ref": supergraphConfig.GraphRef, "ifAfterId": nil})

				// Set the cache using the fetched schema
				logger.Debug("Updating SDL for GraphRef", "graphRef", supergraphConfig.GraphRef)
				systemCache.Set(cacheKey, schema, userConfig.Cache.Duration)
			}

			// Fetch the router license
			if *userConfig.Polling.Entitlements {
				logger.Debug("Polling for router license", "graphRef", supergraphConfig.GraphRef)
				licenseResponse, err := fetchRouterLicense(userConfig, httpClient, supergraphConfig.GraphRef, supergraphConfig.ApolloKey, logger)
				if err != nil {
					logger.Error("Failed to fetch router license", "graphRef", supergraphConfig.GraphRef, "err", err)
					break
				}
				cacheEntry := proxy.JWTCacheEntry{
					Jwt:        licenseResponse.Data.RouterEntitlements.Entitlement.Jwt,
					Expiration: licenseResponse.Data.RouterEntitlements.ID,
				}
				cacheEntryBytes, err := json.Marshal(cacheEntry)
				if err != nil {
					logger.Error("Failed to marshal license", "graphRef", supergraphConfig.GraphRef, "err", err)
					break
				}
				// Update the cache
				cacheKey := cache.MakeCacheKey(graphID, variantID, "LicenseQuery", map[string]interface{}{"apiKey": supergraphConfig.ApolloKey, "graph_ref": supergraphConfig.GraphRef, "ifAfterId": nil})
				// Set the cache using the fetched license
				logger.Debug("Updating license for GraphRef", "graphRef", supergraphConfig.GraphRef, "err", err)
				systemCache.Set(cacheKey, string(cacheEntryBytes[:]), userConfig.Cache.Duration)
			}

			// Fetch the router license
			if *userConfig.Polling.PersistedQueries {
				logger.Debug("Polling for persisted query manifest", "graphRef", supergraphConfig.GraphRef)
				persistedQueryManifest, err := fetchPQManifest(userConfig, httpClient, supergraphConfig.GraphRef, supergraphConfig.ApolloKey, "", logger)
				if err != nil {
					logger.Error("Failed to fetch persisted query manifest", "graphRef", supergraphConfig.GraphRef, "err", err)
					break
				}

				pqManifest, err := json.Marshal(persistedQueryManifest)
				if err != nil {
					logger.Error("Failed to marshal PQ manifest", "graphRef", supergraphConfig.GraphRef, "err", err)
					break
				}

				// Update the cache
				cacheKey := cache.MakeCacheKey(graphID, variantID, "PersistedQueriesManifestQuery", map[string]interface{}{"apiKey": supergraphConfig.ApolloKey, "graph_ref": supergraphConfig.GraphRef, "ifAfterId": nil})

				// Set the cache using the fetched license
				logger.Debug("Updating persisted query manifest for GraphRef", "graphRef", supergraphConfig.GraphRef)
				systemCache.Set(cacheKey, string(pqManifest[:]), userConfig.Cache.Duration)
			}
			// If successful, log the success
			logger.Info("Successfully polled for graph", "graphRef", supergraphConfig.GraphRef)
			success = true
		}
		if !success {
			logger.Error("Failed to poll uplink for graph", "graphRef", supergraphConfig.GraphRef, "retries", userConfig.Polling.RetryCount)
		}
	}
}

// fetchSupergraphSdl fetches the supergraph SDL for the specified graph.
func fetchSupergraphSdl(systemConfig *config.Config, httpClient *http.Client, graphRef string, apiKey string, logger *slog.Logger) (*proxy.UplinkSupergraphSdlResponse, error) {
	// Prepare the request body
	requestBody, err := json.Marshal(proxy.UplinkRelayRequest{
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
	selector := uplink.NewRoundRobinSelector(systemConfig.Uplink.URLs)
	uplinkURL := selector.Next()

	// Create a new request using http
	req, err := http.NewRequest("POST", uplinkURL, bytes.NewBuffer(requestBody))
	if err != nil {
		logger.Error("Error creating request", "err", err)
		return nil, err
	}

	// Uplink Relay Headers
	req.Header.Set("apollo-client-name", "UplinkRelay")
	req.Header.Set("apollo-client-version", "1.0")
	req.Header.Set("User-Agent", "UplinkRelay/1.0")
	req.Header.Set("Content-Type", "application/json")

	// Send req using http Client
	resp, err := httpClient.Do(req)
	if err != nil {
		logger.Error("Error on response", "err", err)
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
	var response proxy.UplinkSupergraphSdlResponse
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
func fetchRouterLicense(userConfig *config.Config, httpClient *http.Client, graphRef string, apiKey string, logger *slog.Logger) (*proxy.UplinkLicenseResponse, error) {
	// Define the request body
	requestBody, err := json.Marshal(proxy.UplinkRelayRequest{
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
	selector := uplink.NewRoundRobinSelector(userConfig.Uplink.URLs)
	uplinkURL := selector.Next()

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

	// Read the response body
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Unmarshal the response body into the LicenseQueryResponse struct
	var response proxy.UplinkLicenseResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// fetchPQManifest fetches the persisted query (PQ) manifest for the specified graph.
func fetchPQManifest(userConfig *config.Config, httpClient *http.Client, graphRef string, apiKey string, ifAfterId string, logger *slog.Logger) (*persistedqueries.UplinkPersistedQueryResponse, error) {
	// Define the request body
	requestBody, err := json.Marshal(proxy.UplinkRelayRequest{
		Variables: map[string]interface{}{
			"apiKey":    apiKey,
			"graph_ref": graphRef,
			"ifAfterId": ifAfterId,
		},
		Query: `query PersistedQueriesManifestQuery($apiKey: String!, $graph_ref: String!, $ifAfterId: ID) {
			persistedQueries(ref: $graph_ref, apiKey: $apiKey, ifAfterId: $ifAfterId) {
				__typename
				... on PersistedQueriesResult {
				id
				minDelaySeconds
				chunks {
					id
					urls
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
		OperationName: "PersistedQueriesManifestQuery",
	})
	if err != nil {
		return nil, err
	}

	// Select the next uplink URL
	selector := uplink.NewRoundRobinSelector(userConfig.Uplink.URLs)
	uplinkURL := selector.Next()

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

	// Read the response body
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Unmarshal the response body into the LicenseQueryResponse struct
	var response persistedqueries.UplinkPersistedQueryResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}
