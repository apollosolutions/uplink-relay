package polling

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/entitlements"
	"apollosolutions/uplink-relay/internal/util"
	persistedqueries "apollosolutions/uplink-relay/persisted_queries"
	"apollosolutions/uplink-relay/schema"
	"apollosolutions/uplink-relay/uplink"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// startPolling starts polling for updates at the specified interval.
func StartPolling(userConfig *config.Config, systemCache cache.Cache, httpClient *http.Client, logger *slog.Logger, stopPolling chan bool) {
	// Log when polling starts
	logger.Info("Polling started")
	if !userConfig.Polling.Enabled {
		logger.Debug("Polling is disabled")
		return
	}

	// immediately poll for updates
	pollForUpdates(userConfig, systemCache, httpClient, logger)

	if userConfig.Polling.Interval > 0 {
		// Create a new ticker with the polling interval
		ticker := time.NewTicker(time.Duration(userConfig.Polling.Interval) * time.Second)
		// Stop the ticker when the function returns
		defer ticker.Stop()

		for {
			select {
			case <-stopPolling:
				logger.Debug("Polling stopped")
				// Stop the ticker as it'll be restarted on the next call to StartPolling
				ticker.Stop()
				return
			case <-ticker.C:
				pollForUpdates(userConfig, systemCache, httpClient, logger)
			}
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

		for range stopPolling {
			logger.Debug("Polling stopped")
			crons.Stop()
			return
		}
	}

}

func pollForUpdates(userConfig *config.Config, systemCache cache.Cache, httpClient *http.Client, logger *slog.Logger) {
	if !userConfig.Polling.Enabled {
		logger.Debug("Polling is disabled for graph")
		return
	}

	if !*userConfig.Polling.Supergraph && !*userConfig.Polling.Entitlements && !*userConfig.Polling.PersistedQueries {
		logger.Warn("Polling is disabled for all artifacts")
		return
	}

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

			// Fetch the schema for the graph if enabled and the launch ID is not set as launchID implies a static schema
			if *userConfig.Polling.Supergraph && supergraphConfig.LaunchID == "" {
				logger.Debug("Polling for supergraph", "graphRef", supergraphConfig.GraphRef)
				err := schema.FetchSchema(userConfig, systemCache, logger, supergraphConfig.GraphRef)
				if err != nil {
					logger.Error("Failed to fetch schema", "graphRef", supergraphConfig.GraphRef, "err", err)
					break
				}
			}

			// Fetch the router license if enabled and the offline license is not set
			if *userConfig.Polling.Entitlements && supergraphConfig.OfflineLicense == "" {
				logger.Debug("Polling for router license", "graphRef", supergraphConfig.GraphRef)
				err := entitlements.FetchRouterLicense(userConfig, systemCache, logger, supergraphConfig.GraphRef)
				if err != nil {
					logger.Error("Failed to fetch router license", "graphRef", supergraphConfig.GraphRef, "err", err)
					break
				}
			}

			// Fetch the persisted queries manifest if enabled and the persisted query version is not set
			if *userConfig.Polling.PersistedQueries && supergraphConfig.PersistedQueryVersion == "" {
				logger.Debug("Polling for persisted query manifest", "graphRef", supergraphConfig.GraphRef)
				persistedQueryManifest, err := FetchPQManifest(userConfig, httpClient, supergraphConfig.GraphRef, supergraphConfig.ApolloKey, "", logger)
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
				cacheKey := cache.MakeCacheKey(supergraphConfig.GraphRef, uplink.PersistedQueriesQuery, map[string]interface{}{"graph_ref": supergraphConfig.GraphRef, "ifAfterId": ""})

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

// FetchPQManifest fetches the persisted query (PQ) manifest for the specified graph.
func FetchPQManifest(userConfig *config.Config, httpClient *http.Client, graphRef string, apiKey string, ifAfterId string, logger *slog.Logger) (*persistedqueries.UplinkPersistedQueryResponse, error) {
	// Define the request body
	requestBody, err := json.Marshal(util.UplinkRelayRequest{
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
