package persistedqueries

import (
	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type UplinkPersistedQueryResponse struct {
	Data struct {
		PersistedQueries UplinkPersistedQueryPersistedQueries `json:"persistedQueries"`
	} `json:"data"`
}

type UplinkPersistedQueryPersistedQueries struct {
	ID              string                      `json:"id"`
	Typename        string                      `json:"__typename"`
	MinDelaySeconds float64                     `json:"minDelaySeconds"`
	Chunks          []UplinkPersistedQueryChunk `json:"chunks,omitempty"`
}

type UplinkPersistedQueryChunk struct {
	ID   string   `json:"id"`
	URLs []string `json:"urls"`
}

/*
*

	The path follows the format of /persisted-queries/{id}?i={index} where {id} is the unique identifier of the persisted query, and {index} is the index of the chunk.
	For example, /persisted-queries/123/0 would represent the first chunk of the persisted query with ID 123.

*
*/
const pathPrefix = "/persisted-queries/"

func PersistedQueryHandler(logger *slog.Logger, client *http.Client, systemCache cache.Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger.Debug("Received request", "path", r.URL.Path)
		id := strings.Split(r.URL.Path, pathPrefix)[1]
		if id == "" {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"Invalid path format"}`, http.StatusBadRequest)
			return
		}
		logger.Debug("Received request for chunk", "path", id)

		index := r.URL.Query().Get("i")
		if index == "" {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"Invalid path format"}`, http.StatusBadRequest)
			return
		}

		logger.Debug("Received request", "id", id, "index", index)
		content, ok := systemCache.Get(makePersistedQueryCacheKey(id, index))
		if !ok {
			// Handle cache miss error
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"Manifest not found"}`, http.StatusNotFound)
			return
		}

		// Write the content to the response
		reader, err := zlib.NewReader(bytes.NewReader(content))
		if err != nil {
			http.Error(w, "Error reading content", http.StatusInternalServerError)
			return
		}
		defer reader.Close()
		io.Copy(w, reader)
	}
}

func CachePersistedQueryChunkData(config *config.Config, logger *slog.Logger, systemCache cache.Cache, chunks []UplinkPersistedQueryChunk) ([]UplinkPersistedQueryChunk, error) {
	// Validate caching is disabled, but also ignore this logic altogether if there's no public URL in the config, as it's used to advertise the cached URLs.
	if !config.Cache.Enabled || config.Relay.PublicURL == "" {
		logger.Debug("Caching disabled, skipping", "publicURL", config.Relay.PublicURL, "cacheEnabled", config.Cache.Enabled)
		return chunks, nil
	}
	if !strings.HasPrefix(config.Relay.PublicURL, "http") {
		logger.Error("Invalid public URL", "publicURL", config.Relay.PublicURL)
		return nil, fmt.Errorf("invalid publicURL: %s", config.Relay.PublicURL)
	}
	parsedUrl, err := url.Parse(config.Relay.PublicURL)
	if err != nil {
		return nil, err
	}
	for c, chunk := range chunks {
		newUrls := []string{}
		for u, chunkUrl := range chunk.URLs {
			cacheKey := makePersistedQueryCacheKey(chunk.ID, strconv.Itoa(u))

			// Fetch the content from the uplink.
			res, err := http.Get(chunkUrl)
			if err != nil {
				return nil, err
			}
			body, err := io.ReadAll(res.Body)
			if err != nil {
				return nil, err
			}

			// compress the text for reducing overall size of the cache entry
			var b bytes.Buffer
			w := zlib.NewWriter(&b)
			_, err = w.Write(body)
			if err != nil {
				return nil, err
			}
			w.Close()

			// Set the content in the cache.
			if err := systemCache.Set(cacheKey, string(b.String()), config.Cache.Duration); err != nil {
				return nil, err
			}

			protocol := parsedUrl.Scheme
			if config.Relay.TLS.KeyFile != "" || config.Relay.TLS.CertFile != "" {
				protocol = "https"
			}
			parsedUrl.Scheme = protocol
			parsedUrl = parsedUrl.JoinPath(pathPrefix)
			logger.Debug("Cached persisted query chunk", "id", chunk.ID, "urls", chunk.URLs, "chunks", chunks, "parsedUrl", parsedUrl.String())
			// Update the URL to point to the local server.
			newUrls = append(newUrls, fmt.Sprintf("%s%s?i=%d", parsedUrl.String(), chunk.ID, u))
		}
		// Update the chunk URLs to point to the local server.
		chunks[c].URLs = newUrls
		logger.Debug("Cached persisted query chunk", "id", chunk.ID, "urls", newUrls, "chunks", chunks)

	}
	return chunks, nil
}

func makePersistedQueryCacheKey(id string, index string) string {
	return fmt.Sprintf("pq:%s:%s", id, index)
}
