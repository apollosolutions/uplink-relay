package graph

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/graph/model"
	persistedqueries "apollosolutions/uplink-relay/persisted_queries"
	"apollosolutions/uplink-relay/pinning"
	"apollosolutions/uplink-relay/uplink"
	"bytes"
	"compress/zlib"
	"encoding/json"
	"io"
	"strconv"
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
			GraphRef:               supergraph.GraphRef,
			PersistedQueryManifest: nil,
			CurrentSchema:          currentSchema,
		}

		if supergraph.LaunchID != "" {
			supergraphEntry.PinnedLaunchID = &supergraph.LaunchID
		}

		if supergraph.PersistedQueryVersion != "" {
			supergraphEntry.PinnedPersistedQueryManifestID = &supergraph.PersistedQueryVersion
		}

		persistedQueryCacheKey := cache.MakeCacheKey(supergraph.GraphRef, uplink.PersistedQueriesQuery, map[string]interface{}{"graph_ref": supergraph.GraphRef, "ifAfterId": ""})

		if supergraph.PersistedQueryVersion != "" {
			persistedQueryCacheKey = cache.MakeCacheKey(supergraph.GraphRef, pinning.PersistedQueriesPinned)
		}

		persistedQueryCacheBytes, ok := r.SystemCache.Get(persistedQueryCacheKey)
		if ok {
			var persistedQueryManifestCacheItem *cache.CacheItem
			err := json.Unmarshal([]byte(persistedQueryCacheBytes), &persistedQueryManifestCacheItem)
			if err != nil {
				return nil
			}

			var persistedQueryManifest *persistedqueries.UplinkPersistedQueryResponse
			err = json.Unmarshal([]byte(persistedQueryManifestCacheItem.Content), &persistedQueryManifest)
			if err != nil {
				r.Logger.Error("Error unmarshalling persisted query manifest", "graphRef", supergraph.GraphRef, "error", err)
				return nil
			}

			chunks := make([]string, 0)
			for index, chunk := range persistedQueryManifest.Data.PersistedQueries.Chunks {
				pqBytes, ok := r.SystemCache.Get(persistedqueries.MakePersistedQueryCacheKey(chunk.ID, strconv.Itoa(index)))
				if ok {
					reader, err := zlib.NewReader(bytes.NewReader(pqBytes))
					if err != nil {
						r.Logger.Error("Error creating zlib reader", "error", err)
						return nil
					}
					defer reader.Close()
					b, err := io.ReadAll(reader)
					if err == nil {
						chunks = append(chunks, string(b[:]))
					}
				}
			}

			supergraphEntry.PersistedQueryManifest = &model.PersistedQueryManifest{
				ID:                   persistedQueryManifestCacheItem.ID,
				Hash:                 persistedQueryManifestCacheItem.Hash,
				PersistedQueryChunks: chunks,
			}
		}

		supergraphs = append(supergraphs, supergraphEntry)
	}
	return &model.Configuration{
		URL:         r.UserConfig.Relay.PublicURL,
		Supergraphs: supergraphs,
	}
}
