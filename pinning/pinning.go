package pinning

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/internal/util"
	"apollosolutions/uplink-relay/uplink"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type APIResponse struct {
	Data LaunchQuery `json:"data"`
}

type PinningAPIRequest struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
	OperationName string                 `json:"operationName"`
}

const (
	SupergraphPinned       = "SupergraphPinned"
	LicensePinned          = "LicensePinned"
	PersistedQueriesPinned = "PersistedQueriesPinned"
)

var OperationMapping = map[string]string{
	uplink.SupergraphQuery:       SupergraphPinned,
	uplink.LicenseQuery:          LicensePinned,
	uplink.PersistedQueriesQuery: PersistedQueriesPinned,
}

func defaultHeaders(req *http.Request, apiKey string) *http.Request {
	req.Header.Set("apollo-client-name", "UplinkRelay")
	req.Header.Set("apollo-client-version", "1.0")
	req.Header.Set("User-Agent", "UplinkRelay/1.0")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	return req
}

func findAPIKey(userConfig *config.Config, graphRef string) (string, error) {
	for _, supergraph := range userConfig.Supergraphs {
		if supergraph.GraphRef == graphRef {
			return supergraph.ApolloKey, nil
		}
	}
	return "", fmt.Errorf("API key not found for graphRef %s", graphRef)
}

func insertPinnedCacheEntry(logger *slog.Logger, systemCache cache.Cache, key string, value string, id string, modifiedTime time.Time) {
	content := cache.CacheItem{
		LastModified: modifiedTime,
		Content:      []byte(value),
		Hash:         util.HashString(value),
		Expiration:   cache.ExpirationTime(-1),
		ID:           id,
	}

	cacheEntry, err := json.Marshal(content)
	if err != nil {
		logger.Error("Failed to create pinned cache entry", "key", key, "value", value)
		return
	}
	systemCache.Set(key, string(cacheEntry[:]), -1)
}

// handlePinnedEntry is a helper function that retrieves the pinned cache entry for the given operation name if it exists, otherwise returns true on the second param
// to indicate it is not newer than the given ifAfterId
// Return arguments are effectively: content, unchanged
func HandlePinnedEntry(logger *slog.Logger, systemCache cache.Cache, graphID, variantID, operationName string, ifAfterID string) (*cache.CacheItem, error) {
	rawEntry, ok := systemCache.Get(cache.MakeCacheKey(fmt.Sprintf("%s@%s", graphID, variantID), OperationMapping[operationName]))
	if !ok {
		logger.Debug("No pinned cache entry found", "operationName", operationName)
		return nil, nil
	}

	var entry cache.CacheItem
	err := json.Unmarshal([]byte(rawEntry), &entry)
	if err != nil {
		logger.Error("Failed to unmarshal pinned cache entry", "operationName", operationName)
		return nil, err
	}
	logger.Debug("Checking pinned cache entry", "operationName", operationName, "cacheLastModified", entry.LastModified, "ifAfterIdTime", ifAfterID)

	// If a license query, simply return the content as the logic to handle is in proxy.go:390
	if ifAfterID == "" {
		return &entry, nil
	}

	// skipping for now as the ID format is different
	if operationName == uplink.PersistedQueriesQuery {
		if entry.LastModified.After(time.Now()) {
			return &entry, nil
		}

		return nil, nil
	}

	ifAfterIDTime, err := time.Parse("2006-01-02T15:04:05Z0700", ifAfterID)
	if err != nil {
		logger.Error("Failed to parse ifAfterId time", "operationName", operationName)
		return nil, err
	}

	if operationName == uplink.LicenseQuery {
		return &entry, nil
	}

	// The entry's last modified time is newer than the ifAfterId time, return the entry in it's entirety
	// ifAfterId indicates the last time the client has seen the data, and as such, a newly modified entry indicates the client should receive the new data
	if entry.LastModified.After(ifAfterIDTime) {
		return &entry, nil
	}

	entry.Content = nil
	return &entry, nil
}
