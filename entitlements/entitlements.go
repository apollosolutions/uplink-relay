package entitlements

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/internal/util"
	"apollosolutions/uplink-relay/pinning"
	"apollosolutions/uplink-relay/uplink"
	"encoding/json"
	"log/slog"
	"time"
)

// Jwt struct
type Jwt struct {
	Jwt string `json:"jwt"`
}

type UplinkRouterEntitlements struct {
	ID              string  `json:"id"`
	Typename        string  `json:"__typename"`
	MinDelaySeconds float64 `json:"minDelaySeconds"`
	Entitlement     *Jwt    `json:"entitlement,omitempty"`
}

// UplinkLicenseResponse struct
type UplinkLicenseResponse struct {
	Data struct {
		RouterEntitlements UplinkRouterEntitlements `json:"routerEntitlements"`
	} `json:"data"`
}

// FetchRouterLicense fetches the router license for the specified graph.
func FetchRouterLicense(userConfig *config.Config, systemCache cache.Cache, logger *slog.Logger, graphRef string) error {
	supergraphConfig, err := config.FindSupergraphConfigFromGraphRef(graphRef, userConfig)
	if err != nil {
		return err
	}

	if supergraphConfig.OfflineLicense != "" {
		return pinning.PinOfflineLicense(userConfig, logger, systemCache, supergraphConfig.LaunchID, graphRef)
	}

	variables := map[string]interface{}{
		"apiKey":    supergraphConfig.ApolloKey,
		"graph_ref": graphRef,
		"ifAfterId": "",
	}

	query := `query LicenseQuery($apiKey: String!, $graph_ref: String!, $ifAfterId: ID) {
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
		}`

	operationName := "LicenseQuery"

	resp, err := util.UplinkRequest(userConfig, logger, query, variables, operationName)
	if err != nil {
		return err
	}

	// Unmarshal the response body into the LicenseQueryResponse struct
	var response UplinkLicenseResponse
	err = json.Unmarshal(resp, &response)
	if err != nil {
		return err
	}

	expiration, err := time.Parse(time.RFC3339, response.Data.RouterEntitlements.ID)
	if err != nil {
		logger.Error("Failed to parse license expiration", "graphRef", supergraphConfig.GraphRef, "err", err)
		return err
	}

	if userConfig.Cache.Enabled {
		// Cache the license
		return CacheLicense(systemCache, logger, graphRef, response.Data.RouterEntitlements.Entitlement.Jwt, expiration, userConfig.Cache.Duration, "")
	}
	return nil
}

func CacheLicense(systemCache cache.Cache, logger *slog.Logger, graphRef string, entitlementJWT string, id time.Time, duration int, ifAfterId string) error {
	cacheItem := cache.CacheItem{
		ID:           id.Format(time.RFC3339),
		Content:      []byte(entitlementJWT),
		Hash:         util.HashString(entitlementJWT),
		LastModified: time.Now(),
		Expiration:   id,
	}

	cacheBytes, err := json.Marshal(cacheItem)
	if err != nil {
		logger.Error("Failed to marshal license", "graphRef", graphRef, "err", err)
		return err
	}
	logger.Debug("Caching entitlement", "graphRef", graphRef, "iaid", ifAfterId, "id", id, "jwt", entitlementJWT)

	// Store the schema in the cache
	cacheKey := cache.MakeCacheKey(graphRef, uplink.LicenseQuery, map[string]interface{}{"graph_ref": graphRef, "ifAfterId": ifAfterId})

	if cacheItem.Content == nil {
		cache.UpdateNewest(systemCache, logger, graphRef, uplink.LicenseQuery, cacheItem)
	}

	return systemCache.Set(cacheKey, string(cacheBytes[:]), duration)
}
