package pinning

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	persistedqueries "apollosolutions/uplink-relay/persisted_queries"
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type PersistedQueryQueryResponse struct {
	Data PersistedQueryQueryData `json:"data"`
}

type PersistedQueryQueryData struct {
	Variant PersistedQueryQueryVariant `json:"variant"`
}

type PersistedQueryQueryVariant struct {
	Typename string `json:"__typename"`
	// Only exists if __typename is "GraphVariant"
	PersistedQueryQueryList *PersistedQueryQueryList `json:"persistedQueryList"`

	// Only exists if __typename is "InvalidRefFormat" or "Error"
	Message string `json:"message"`
}

type PersistedQueryQueryList struct {
	Builds PersistedQueryQueryBuilds `json:"builds"`
}

type PersistedQueryQueryBuilds struct {
	Edges []PersistedQueryQueryEdge `json:"edges"`
}

type PersistedQueryQueryEdge struct {
	Node PersistedQueryQueryNode `json:"node"`
}

type PersistedQueryQueryNode struct {
	ID             string                               `json:"id"`
	ManifestChunks *[]PersistedQueryQueryManifestChunks `json:"manifestChunks"`
}

type PersistedQueryQueryManifestChunks struct {
	ID   string `json:"id"`
	JSON string `json:"json"`
}

func PinPersistedQueries(userConfig *config.Config, logger *slog.Logger, systemCache cache.Cache, graphRef string, persistedQueryVersion string) error {
	logger.Debug("Pinning PQ version", "version", persistedQueryVersion, "graphRef", graphRef)
	// Configure the HTTP client with a timeout.
	httpClient := &http.Client{
		Timeout: time.Duration(userConfig.Uplink.Timeout) * time.Second,
	}

	apiKey, err := findAPIKey(userConfig, graphRef)
	if err != nil {
		logger.Error("Failed to find API key", "graphRef", graphRef)
		return err
	}

	requestBody, err := json.Marshal(&PinningAPIRequest{
		Query: `
		query UplinkRelay_PinPersistedQueries($ref: ID!){
			variant(ref: $ref) {
				__typename
				... on InvalidRefFormat {
					message
				}
				... on Error {
					message
				}
				... on GraphVariant {
					persistedQueryList {
						builds {
							edges {
								node {
									id
									manifestChunks {
										id
										json
									}
								}
							}
						}
					}
				}
			}
		}
		`,
		Variables: map[string]interface{}{
			"ref": graphRef,
		},
		OperationName: "UplinkRelay_PinPersistedQueries",
	})
	if err != nil {
		logger.Error("Error preparing request body", "err", err)
		return err
	}

	req, err := http.NewRequest("POST", userConfig.Uplink.StudioAPIURL, bytes.NewBuffer(requestBody))
	if err != nil {
		logger.Error("Error creating request", "err", err)
		return err
	}
	req = defaultHeaders(req, apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		logger.Error("Error sending request", "err", err)
		return err
	}
	defer resp.Body.Close()
	// Read the response body
	bodyBytes, _ := io.ReadAll(resp.Body)

	var apiResponse PersistedQueryQueryResponse
	err = json.Unmarshal(bodyBytes, &apiResponse)
	if err != nil {
		logger.Error("Error unmarshalling response", "err", err)
		return err
	}

	if apiResponse.Data.Variant.Typename != "GraphVariant" {
		logger.Error("Failed to get persisted query list", "graphRef", graphRef, "version", persistedQueryVersion, "message", apiResponse.Data.Variant.Message)
		return fmt.Errorf(apiResponse.Data.Variant.Message)
	}
	// Find the matching edge for the persisted query version
	node, err := findMatchingNode(apiResponse.Data.Variant.PersistedQueryQueryList.Builds.Edges, persistedQueryVersion)
	if err != nil {
		logger.Error("Failed to find persisted query version", "graphRef", graphRef, "version", persistedQueryVersion)
		return err
	}

	if userConfig.Cache.Enabled {
		// Insert the pinned cache entry
		chunks, err := cachePinnedChunks(userConfig, logger, systemCache, node)
		if err != nil {
			logger.Error("Failed to cache pinned chunks", "graphRef", graphRef, "version", persistedQueryVersion)
			return err
		}
		logger.Debug("Cached pinned chunks", "graphRef", graphRef, "version", persistedQueryVersion)

		// Insert the persisted query version into the cache
		fakeResponse := persistedqueries.UplinkPersistedQueryResponse{
			Data: struct {
				PersistedQueries persistedqueries.UplinkPersistedQueryPersistedQueries "json:\"persistedQueries\""
			}{
				PersistedQueries: persistedqueries.UplinkPersistedQueryPersistedQueries{
					Typename:        "PersistedQueriesResult",
					ID:              node.ID,
					MinDelaySeconds: 60,
					Chunks:          chunks,
				},
			},
		}

		respBytes, err := json.Marshal(fakeResponse)
		if err != nil {
			logger.Error("Failed to marshal fake response", "graphRef", graphRef, "version", persistedQueryVersion)
			return err
		}
		logger.Debug("Caching persisted query version", "graphRef", graphRef, "version", persistedQueryVersion, "response", fakeResponse)
		insertPinnedCacheEntry(logger, systemCache, cache.MakeCacheKey(graphRef, PersistedQueriesPinned), string(respBytes[:]), node.ID, time.Now())
	}

	// now finally update the config to the new pinned version to handle the case where the management API updated the PQ ID
	configs := []config.SupergraphConfig{}
	for _, s := range userConfig.Supergraphs {
		if s.GraphRef == graphRef {
			s.PersistedQueryVersion = persistedQueryVersion
		}
		configs = append(configs, s)
	}
	userConfig.Supergraphs = configs
	return nil
}

func findMatchingNode(edges []PersistedQueryQueryEdge, persistedQueryVersion string) (*PersistedQueryQueryNode, error) {
	for _, edge := range edges {
		if edge.Node.ID == persistedQueryVersion {
			return &edge.Node, nil
		}
	}
	return nil, fmt.Errorf("failed to find matching edge for persisted query version %s", persistedQueryVersion)
}

func cachePinnedChunks(userConfig *config.Config, logger *slog.Logger, systemCache cache.Cache, node *PersistedQueryQueryNode) ([]persistedqueries.UplinkPersistedQueryChunk, error) {
	if userConfig.Relay.PublicURL == "" {
		logger.Error("Public URL not set")
		return nil, fmt.Errorf("public URL not set")
	}
	publicURL, err := url.Parse(userConfig.Relay.PublicURL)
	if err != nil {
		return nil, err
	}
	publicURL = publicURL.JoinPath("/persisted-queries")

	chunks := make([]persistedqueries.UplinkPersistedQueryChunk, len(*node.ManifestChunks))

	for index, chunk := range *node.ManifestChunks {
		// insertPinnedCacheEntry(logger, systemCache, chunk.ID, chunk.JSON, chunk.ID, time.Now())
		cacheKey := persistedqueries.MakePersistedQueryCacheKey(chunk.ID, strconv.Itoa(index))
		var b bytes.Buffer
		w := zlib.NewWriter(&b)
		_, err = w.Write([]byte(chunk.JSON))
		if err != nil {
			return nil, err
		}
		w.Close()

		err := systemCache.Set(cacheKey, b.String(), -1)
		if err != nil {
			logger.Error("Failed to cache persisted query chunk", "id", chunk.ID)
			return nil, err
		}
		chunkURL := publicURL.JoinPath(fmt.Sprintf("/%s", chunk.ID))
		q := chunkURL.Query()
		q.Add("i", strconv.Itoa(index))
		chunkURL.RawQuery = q.Encode()
		chunks[index] = persistedqueries.UplinkPersistedQueryChunk{
			ID:   chunk.ID,
			URLs: []string{chunkURL.String()},
		}
		logger.Debug("Cached persisted query chunk", "id", chunk.ID)
	}
	return chunks, nil
}
