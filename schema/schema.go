package schema

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/internal/util"
	"apollosolutions/uplink-relay/pinning"
	"apollosolutions/uplink-relay/uplink"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// UplinkRouterConfig struct
type UplinkRouterConfig struct {
	Typename        string  `json:"__typename"`
	ID              string  `json:"id"`
	SupergraphSdl   string  `json:"supergraphSdl,omitempty"`
	MinDelaySeconds float64 `json:"minDelaySeconds"`
}

// SupergraphSdlQueryResponse struct
type UplinkSupergraphSdlResponse struct {
	Data struct {
		RouterConfig UplinkRouterConfig `json:"routerConfig"`
	} `json:"data"`
}

func FetchSchema(userConfig *config.Config, systemCache cache.Cache, logger *slog.Logger, graphRef string) error {
	supergraphConfig, err := config.FindSupergraphConfigFromGraphRef(graphRef, userConfig)
	if err != nil {
		return err
	}

	if supergraphConfig.LaunchID != "" {
		return pinning.PinLaunchID(userConfig, logger, systemCache, supergraphConfig.LaunchID, graphRef)
	}

	variables := map[string]interface{}{
		"apiKey":    supergraphConfig.ApolloKey,
		"graph_ref": graphRef,
		"ifAfterId": "",
	}

	query := `query SupergraphSdlQuery($apiKey: String!, $graph_ref: String!, $ifAfterId: ID) {
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
			}`

	operationName := "SupergraphSdlQuery"

	resp, err := util.UplinkRequest(userConfig, logger, query, variables, operationName)
	if err != nil {
		return err
	}

	// Log the raw response body
	logger.Debug("Raw response body", "body", resp)

	// Decode the response body
	var response UplinkSupergraphSdlResponse
	decodeErr := json.Unmarshal(resp, &response)
	if decodeErr != nil {
		return fmt.Errorf("failed to decode response body: %w", decodeErr)
	}
	id, err := time.Parse(time.RFC3339, response.Data.RouterConfig.ID)
	if err != nil {
		logger.Error("Failed to parse license expiration", "graphRef", variables["graph_ref"], "err", err)
		return err
	}
	if userConfig.Cache.Enabled {
		// Cache the schema
		return CacheSchema(systemCache, logger, graphRef, response.Data.RouterConfig.SupergraphSdl, id, "", userConfig.Cache.Duration)
	}
	// Return the response
	return nil
}

func CacheSchema(systemCache cache.Cache, logger *slog.Logger, graphRef string, schema string, id time.Time, ifAfterID string, duration int) error {
	cacheItem := cache.CacheItem{
		ID:           id.Format(time.RFC3339),
		Hash:         util.HashString(schema),
		Expiration:   cache.ExpirationTime(duration),
		LastModified: time.Now(),
		Content:      []byte(schema),
	}
	cacheBytes, err := json.Marshal(cacheItem)
	if err != nil {
		return err
	}

	// Store the schema in the cache
	cacheKey := cache.MakeCacheKey(graphRef, uplink.SupergraphQuery, map[string]interface{}{"graph_ref": graphRef, "ifAfterId": ifAfterID})

	if cacheItem.Content != nil {
		cache.UpdateNewest(systemCache, logger, graphRef, uplink.SupergraphQuery, cacheItem)
	}

	logger.Debug("Caching schema", "graphRef", graphRef, "cacheKey", cacheKey)
	return systemCache.Set(cacheKey, string(cacheBytes[:]), duration)
}
