package pinning

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/internal/util"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type LaunchQuery struct {
	Graph *LaunchQueryGraph `json:"graph"`
}

type LaunchQueryGraph struct {
	Variant LaunchQueryVariantLaunch `json:"variant"`
}

type LaunchQueryVariantLaunch struct {
	ID     string `json:"id"`
	Launch struct {
		CompletedAt string `json:"completedAt"`
		Build       struct {
			Result struct {
				Typename string `json:"__typename"`
				// Only exists if __typename is "BuildSuccess"
				CoreSchema struct {
					CoreDocument string `json:"coreDocument"`
				} `json:"coreSchema"`

				// Only exists if __typename is "BuildFailure"
				ErrorMessages []struct {
					Message string `json:"message"`
				} `json:"errorMessages"`
			}
		} `json:"build"`
	} `json:"launch"`
}

func PinLaunchID(userConfig *config.Config, logger *slog.Logger, systemCache cache.Cache, launchID string, graphRef string) error {
	logger.Debug("Pinning launch ID", "launchID", launchID, "graphRef", graphRef)
	// Configure the HTTP client with a timeout.
	httpClient := &http.Client{
		Timeout: time.Duration(userConfig.Uplink.Timeout) * time.Second,
	}

	graphID, variantID, err := util.ParseGraphRef(graphRef)
	if err != nil {
		logger.Error("Failed to parse GraphRef", "graphRef", graphRef)
		return err
	}

	apiKey, err := findAPIKey(userConfig, graphRef)
	if err != nil {
		logger.Error("Failed to find API key", "graphRef", graphRef)
		return err
	}

	requestBody, err := json.Marshal(&PinningAPIRequest{
		Query: `
		query UplinkRelay_GetLaunchIDSchema($graphId: ID!, $name: String!, $launchId: ID!) {
			graph(id: $graphId) {
				variant(name: $name) {
					id
					launch(id: $launchId) {
						completedAt
						build {
						result {
							__typename
							... on BuildSuccess {
							coreSchema {
								coreDocument
							}
							}
							... on BuildFailure {
							errorMessages {
								message
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
			"graphId":  graphID,
			"name":     variantID,
			"launchId": launchID,
		},
		OperationName: "UplinkRelay_GetLaunchIDSchema",
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

	var apiResponse APIResponse
	err = json.Unmarshal(bodyBytes, &apiResponse)
	if err != nil {
		logger.Error("Error unmarshalling response", "err", err)
		return err
	}

	if apiResponse.Data.Graph == nil {
		logger.Error("Failed to get launch ID schema", "graphRef", graphRef, "launchID", launchID)
		return fmt.Errorf("failed to get launch ID schema")
	}

	if apiResponse.Data.Graph.Variant.Launch.Build.Result.Typename == "BuildFailure" {
		logger.Error("Failed to get launch ID schema", "graphRef", graphRef, "launchID", launchID, "errorMessages", apiResponse.Data.Graph.Variant.Launch.Build.Result.ErrorMessages)
		return fmt.Errorf("failed to get launch ID schema")
	}

	modifiedAt, err := time.Parse(time.RFC3339, apiResponse.Data.Graph.Variant.Launch.CompletedAt)
	if err != nil {
		logger.Error("Failed to parse completedAt", "completedAt", apiResponse.Data.Graph.Variant.Launch.CompletedAt)
		return err
	}

	// Store the core schema in the cache
	if userConfig.Cache.Enabled {
		cacheKey := cache.MakeCacheKey(graphRef, SupergraphPinned)
		insertPinnedCacheEntry(logger, systemCache, cacheKey, apiResponse.Data.Graph.Variant.Launch.Build.Result.CoreSchema.CoreDocument, apiResponse.Data.Graph.Variant.ID, modifiedAt)
	}
	// now finally update the config to the new pinned version to handle the case where the management API updated the launchID
	configs := []config.SupergraphConfig{}
	for _, s := range userConfig.Supergraphs {
		if s.GraphRef == graphRef {
			s.LaunchID = launchID
		}
		configs = append(configs, s)
	}
	userConfig.Supergraphs = configs
	return nil
}
