package pinning

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
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

type PinnedCacheEntry struct {
	LastModified string `json:"lastModified"`
	Content      string `json:"content"`
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

func insertPinnedCacheEntry(logger *slog.Logger, systemCache cache.Cache, key string, value string, modifiedTime time.Time) {
	content := PinnedCacheEntry{
		LastModified: modifiedTime.Format(time.RFC3339),
		Content:      value,
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
func HandlePinnedEntry(logger *slog.Logger, systemCache cache.Cache, graphID, variantID, operationName string, ifAfterID string) ([]byte, bool) {
	rawEntry, ok := systemCache.Get(cache.MakeCacheKey(graphID, variantID, OperationMapping[operationName]))
	if !ok {
		logger.Info("No pinned cache entry found", "operationName", operationName)
		return nil, true
	}

	var entry PinnedCacheEntry
	err := json.Unmarshal([]byte(rawEntry), &entry)
	if err != nil {
		logger.Error("Failed to unmarshal pinned cache entry", "operationName", operationName)
		return nil, true
	}
	logger.Debug("Checking pinned cache entry", "operationName", operationName, "cacheLastModified", entry.LastModified, "ifAfterIdTime", ifAfterID)

	if ifAfterID == "" {
		return []byte(entry.Content), false
	}

	cacheLastModified, err := time.Parse(time.RFC3339, entry.LastModified)
	if err != nil {
		logger.Error("Failed to parse last modified time", "operationName", operationName)
		return nil, true
	}

	// skipping for now as the ID format is different
	if operationName == uplink.PersistedQueriesQuery {
		if cacheLastModified.After(time.Now()) {
			return []byte(entry.Content), false
		}

		return nil, true
	}

	ifAfterIDTime, err := time.Parse("2006-01-02T15:04:05Z0700", ifAfterID)
	if err != nil {
		logger.Error("Failed to parse ifAfterId time", "operationName", operationName)
		return nil, true
	}
	if cacheLastModified.After(ifAfterIDTime) {
		return []byte(entry.Content), false
	}

	return nil, true
}
