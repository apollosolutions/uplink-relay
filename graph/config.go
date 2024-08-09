package graph

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/graph/model"
	"apollosolutions/uplink-relay/pinning"
	"apollosolutions/uplink-relay/uplink"
	"encoding/json"
)

func (r *ResolverContext) GetConfigDetails() *model.Configuration {
	supergraphs := make([]*model.Supergraph, 0)

	for _, supergraph := range r.UserConfig.Supergraphs {
		var currentSchema *model.Schema
		supergraphCacheKey := cache.DefaultCacheKey(supergraph.GraphRef, uplink.SupergraphQuery)

		var supergraphCacheEntry cache.CacheItem
		if supergraph.LaunchID != "" {
			supergraphCacheKey = cache.MakeCacheKey(supergraph.GraphRef, pinning.SupergraphPinned)
		}

		supergrahCacheBytes, ok := r.SystemCache.Get(supergraphCacheKey)

		if ok {
			err := json.Unmarshal([]byte(supergrahCacheBytes), &supergraphCacheEntry)
			// if successful, this will set currentSchema to the schema in the cache
			if err == nil {
				if len(supergraphCacheEntry.Content) == 0 {
					continue
				}
				currentSchema = &model.Schema{
					ID:     supergraphCacheEntry.ID,
					Hash:   supergraphCacheEntry.Hash,
					Schema: string(supergraphCacheEntry.Content[:]),
				}
			} else {
				r.Logger.Error("Error unmarshalling supergraph cache entry", "graphRef", supergraph.GraphRef, "error", err)
			}
		}

		supergraphEntry := &model.Supergraph{
			GraphRef: supergraph.GraphRef,
			PersistedQueryManifestID: &model.PersistedQueryManifest{
				ID: supergraph.PersistedQueryVersion,
			},
			CurrentSchema: currentSchema,
		}

		if supergraph.LaunchID != "" {
			supergraphEntry.PinnedLaunchID = &supergraph.LaunchID
		}

		supergraphs = append(supergraphs, supergraphEntry)
	}
	return &model.Configuration{
		URL:         r.UserConfig.Relay.PublicURL,
		Supergraphs: supergraphs,
	}
}
