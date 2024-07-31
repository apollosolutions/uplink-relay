package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/internal/util"
	persistedqueries "apollosolutions/uplink-relay/persisted_queries"
	"apollosolutions/uplink-relay/pinning"
	"apollosolutions/uplink-relay/uplink"
)

// Register handlers for proxy routes.
func RegisterHandlers(route string, handler http.HandlerFunc) {
	http.HandleFunc(route, handler)
}

// Deregister all handlers for proxy routes (for reload purposes)
func DeregisterHandlers() {
	http.DefaultServeMux = http.NewServeMux()
}

// StartServer starts the HTTP server with the given address and handler.
func StartServer(config *config.Config, logger *slog.Logger) (*http.Server, error) {
	address := config.Relay.Address
	logger.Info("Starting Uplink Relay  🛰  ", "address", address)
	server := &http.Server{Addr: address, Handler: http.DefaultServeMux}
	go func() {
		var err error
		if config.Relay.TLS.CertFile != "" && config.Relay.TLS.KeyFile != "" {
			err = server.ListenAndServeTLS(config.Relay.TLS.CertFile, config.Relay.TLS.KeyFile)
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			logger.Error("ListenAndServe error", "err", err)
			os.Exit(1)
		}
	}()
	return server, nil
}

// Shut down the server with a context that times out after 5 seconds.
func ShutdownServer(server *http.Server, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Uplink Relay Shutdown", "err", err)
	} else {
		logger.Info("Uplink Relay shut down properly")
	}
}

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

// UplinkRelayRequest struct
type UplinkRelayRequest struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
	OperationName string                 `json:"operationName"`
}

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

// uplinkRelayResponses maps operation names to response structs.
var uplinkRelayResponses = map[string]interface{}{
	uplink.SupergraphQuery:       &UplinkSupergraphSdlResponse{},
	uplink.LicenseQuery:          &UplinkLicenseResponse{},
	uplink.PersistedQueriesQuery: &persistedqueries.UplinkPersistedQueryResponse{},
}

// parseRequest parses and validates the request.
func parseRequest(r *http.Request) (UplinkRelayRequest, error) {
	var requestBody UplinkRelayRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		err := fmt.Errorf("failed to read request body: %w", err)
		return requestBody, err
	}
	err = json.Unmarshal(body, &requestBody)
	if err != nil {
		err := fmt.Errorf("failed to unmarshal request body: %w", err)
		return requestBody, err
	}

	// Replace the body so it can be read again later
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	return requestBody, nil
}

// Logs the request headers if debug mode is enabled.
func debugRequestHeaders(logger *slog.Logger, r *http.Request) {
	for name, values := range r.Header {
		for _, value := range values {
			logger.Debug("Request header: %s = %s\n", name, value)
		}
	}
}

// Reads and logs the request body if debug mode is enabled.
// It replaces the request body with a new buffer so it can be read again later.
func debugRequestBody(logger *slog.Logger, r *http.Request) {
	if r.Body == nil {
		return
	}
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read request body", "err", err)
	}
	logger.Debug("Request body", "body", bodyBytes)

	// Replace the body so it can be read again later
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
}

// Logs the response headers if debug mode is enabled.
func debugResponseHeaders(logger *slog.Logger, headers http.Header) {
	for name, values := range headers {
		for _, value := range values {
			logger.Debug("Response header: %s = %s\n", name, value)
		}
	}
}

// Reads and logs the response body if debug mode is enabled.
// It replaces the body with a new buffer so it can be read again later.
func debugResponseBody(logger *slog.Logger, r *http.Response) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read response body", "err", err)
	}
	logger.Debug("Response Body", "body", bodyBytes)

	// Replace the body so it can be read again later
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
}

// Modifies the proxied response before it is returned to the client.
func modifyProxiedResponse(config *config.Config, systemCache cache.Cache, cacheKey string, uplinkRequest UplinkRelayRequest, logger *slog.Logger) func(*http.Response) error {
	return func(resp *http.Response) error {
		// Debug log the response headers
		debugResponseHeaders(logger, resp.Header)

		// Debug log the response body
		debugResponseBody(logger, resp)

		// Get the response based on the operation name
		responseStruct, ok := uplinkRelayResponses[uplinkRequest.OperationName]
		if !ok {
			logger.Warn("Unknown operation name", "operationName", uplinkRequest.OperationName)
			return nil
		}
		var responseBody []byte

		if resp.Header.Get("Content-Encoding") == "gzip" {
			logger.Debug("Decompressing response body")
			// Decompress the response body
			reader, err := gzip.NewReader(resp.Body)
			if err != nil {
				logger.Error("Failed to decompress response body", "err", err)
				return err
			}
			defer reader.Close()

			responseBody, err = io.ReadAll(reader)
			if err != nil {
				logger.Error("Failed to read decompressed response body", "err", err)
				return err
			}
		} else {
			// Decode the response body into the response struct
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				logger.Error("Failed to read response body", "err", err)
				return nil
			}

			responseBody = body
		}

		// Unmarshal the response body into the response struct
		err := json.Unmarshal(responseBody, &responseStruct)
		if err != nil {
			logger.Error("Failed to unmarshal response body", "err", err, "responseBody", string(responseBody[:]))
			return nil
		}
		// Cache the response based on the operation name
		if uplinkRequest.OperationName == uplink.SupergraphQuery {
			// Assert the type of the response
			uplinkResponse, ok := responseStruct.(*UplinkSupergraphSdlResponse)
			if !ok {
				logger.Error(fmt.Sprintf("Failed to assert type of response: expected *UplinkSupergraphSdlResponse, got %T", uplinkResponse))
				return nil
			}

			// Extract the schema from the UplinkResponse
			schema := uplinkResponse.Data.RouterConfig.SupergraphSdl

			// Log the UplinkResponse
			logger.Debug("SupergraphSdlQuery response", "response", uplinkResponse)

			// Cache the response for future requests.
			if config.Cache.Enabled {
				logger.Debug("Caching schema", "key", cacheKey)
				systemCache.Set(cacheKey, schema, config.Cache.Duration)
			}
		} else if uplinkRequest.OperationName == uplink.LicenseQuery {
			// Assert the type of the response
			uplinkResponse, ok := responseStruct.(*UplinkLicenseResponse)
			if !ok {
				logger.Error(fmt.Sprintf("Failed to assert type of response: expected *UplinkLicenseResponse, got %T", uplinkResponse))
				return nil
			}
			// Log the LicenseQueryResponse
			logger.Debug("LicenseQuery response", "response", uplinkResponse)

			jwt := ""
			if uplinkResponse.Data.RouterEntitlements.Entitlement != nil {
				jwt = uplinkResponse.Data.RouterEntitlements.Entitlement.Jwt
			}
			expiration, err := time.Parse(time.RFC3339, uplinkResponse.Data.RouterEntitlements.ID)
			if err != nil {
				logger.Error("Failed to parse license expiration", "graphRef", uplinkRequest.Variables["graph_ref"], "err", err)
				return err
			}
			// Cache the response for future requests, if caching is enabled
			if config.Cache.Enabled {
				logger.Debug("Caching JWT", "key", cacheKey)
				cacheEntry := cache.CacheItem{
					Content:    []byte(jwt),
					Expiration: expiration,
				}
				cacheEntryBytes, err := json.Marshal(cacheEntry)
				if err != nil {
					logger.Error("Failed to marshal license", "err", err)
					return err
				}
				systemCache.Set(cacheKey, string(cacheEntryBytes[:]), config.Cache.Duration)
			}

		} else if uplinkRequest.OperationName == uplink.PersistedQueriesQuery {
			// Assert the type of the response
			uplinkResponse, ok := responseStruct.(*persistedqueries.UplinkPersistedQueryResponse)
			if !ok {
				logger.Error(fmt.Sprintf("Failed to assert type of response: expected *UplinkPersistedQueryResponse, got %T", uplinkResponse))
				return nil
			}

			// Log the PersistedQueryResponse
			logger.Debug("PersistedQuery response", "response", uplinkResponse)

			// Cache the response for future requests, if caching is enabled
			if config.Cache.Enabled {
				logger.Debug("Caching PersistedQuery", "key", cacheKey)
				chunks, err := persistedqueries.CachePersistedQueryChunkData(config, logger, systemCache, uplinkResponse.Data.PersistedQueries.Chunks)
				if err != nil {
					logger.Error("Failed to cache PersistedQuery chunks", "err", err)
					return err
				}
				uplinkResponse.Data.PersistedQueries.Chunks = chunks

				// Marshal the response struct
				responseBody, err = json.Marshal(uplinkResponse)
				if err != nil {
					logger.Error("Failed to marshal cache entry", "err", err)
				}

				// Set the Content-Length header to the length of the response body as we've modified it to include the persisted query chunks hosted on the relay
				resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(responseBody)))

				// Cache the response
				err = systemCache.Set(cacheKey, string(responseBody[:]), config.Cache.Duration)
				if err != nil {
					logger.Error("Failed to cache response", "err", err)
				}
			}
		} else {
			logger.Warn("Unknown operation name", "operationName", uplinkRequest.OperationName)
		}

		// Replace the response body with the original data
		resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))

		// Log the proxied response
		debugResponseBody(logger, resp)

		// Reset the response struct to avoid caching the response across requests
		// The cache function will handle caching the response
		responseStruct = nil

		return nil
	}
}

// Creates a reverse proxy to the target URL.
func makeProxy(config *config.Config, cache cache.Cache, httpClient *http.Client, logger *slog.Logger) func(*url.URL, string, UplinkRelayRequest) *httputil.ReverseProxy {
	return func(targetURL *url.URL, cacheKey string, uplinkRequest UplinkRelayRequest) *httputil.ReverseProxy {
		proxy := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.Out.URL = targetURL
				pr.Out.Host = targetURL.Host
				pr.Out.Header = pr.In.Header
			},
		}
		proxy.Transport = httpClient.Transport
		proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
			logger.Error("HTTP proxy error", "err", err)
		}
		proxy.ModifyResponse = modifyProxiedResponse(config, cache, cacheKey, uplinkRequest, logger)
		return proxy
	}
}

// Parses the target URL.
func parseUrl(target string) (*url.URL, error) {
	proxyUrl, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}
	return proxyUrl, nil
}

// Handles a cache hit by returning the cached response.
func handleCacheHit(cacheKey string, content []byte, logger *slog.Logger, cacheDuration time.Duration, ifAfterId string) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var response interface{}

		// Format the response body based on operation name
		if strings.Contains(cacheKey, uplink.SupergraphQuery) {
			typename := "RouterConfigResult"
			if len(content) == 0 {
				typename = "Unchanged"
			}
			timestamp := time.Now().UTC().Round(cacheDuration).Format(time.RFC3339)

			response = &UplinkSupergraphSdlResponse{
				Data: struct {
					RouterConfig UplinkRouterConfig `json:"routerConfig"`
				}{
					RouterConfig: UplinkRouterConfig{
						ID:              timestamp,
						Typename:        typename,
						SupergraphSdl:   string(content),
						MinDelaySeconds: 30,
					},
				},
			}
		} else if strings.Contains(cacheKey, uplink.LicenseQuery) {
			typename := "RouterEntitlementsResult"
			cacheResponse := cache.CacheItem{}
			err := json.Unmarshal(content, &cacheResponse)
			if err != nil {
				logger.Error("Failed to unmarshal JWT cache entry", "err", err, "cacheKey", cacheKey, "content", string(content))
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return nil
			}

			jwtEntitlement := &Jwt{Jwt: string(cacheResponse.Content[:])}
			if len(cacheResponse.Content) == 0 {
				typename = "Unchanged"
				jwtEntitlement = nil
			}

			if ifAfterId != "" {
				ifAfterTime, err := time.Parse(time.RFC3339, ifAfterId)
				if err != nil {
					logger.Error("Failed to parse ifAfterId", "err", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return nil
				}
				if cacheResponse.Expiration.Before(ifAfterTime) {
					typename = "Unchanged"
					jwtEntitlement = nil
				}
			}

			response = &UplinkLicenseResponse{
				Data: struct {
					RouterEntitlements UplinkRouterEntitlements `json:"routerEntitlements"`
				}{
					RouterEntitlements: UplinkRouterEntitlements{
						ID:              cacheResponse.Expiration.Format(time.RFC3339),
						Typename:        typename,
						MinDelaySeconds: 60,
						Entitlement:     jwtEntitlement,
					},
				},
			}
		} else if strings.Contains(cacheKey, uplink.PersistedQueriesQuery) {
			var cachedResponse persistedqueries.UplinkPersistedQueryResponse
			err := json.Unmarshal(content, &cachedResponse)
			if err != nil {
				logger.Error("Failed to unmarshal PersistedQuery chunks", "err", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return nil
			}
			response = cachedResponse
		}

		// Convert the response to JSON
		responseBody, err := json.Marshal(response)
		if err != nil {
			logger.Error("Failed to marshal response", "err", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return nil
		}
		// Set the appropriate headers
		w.Header().Add("X-Cache-Hit", "true")

		// Write the cached content to the response
		_, err = w.Write(responseBody)
		if err != nil {
			logger.Error("Failed to write response", "err", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return nil
		}

		// Log the response
		logger.Debug("Cached Response", "response", responseBody)

		return nil
	}
}

// Handles a cache miss by proxying the request to the uplink service.
func handleCacheMiss(config *config.Config, cache cache.Cache, httpClient *http.Client, rrSelector *uplink.RoundRobinSelector, cacheKey string, uplinkRequest UplinkRelayRequest, logger *slog.Logger) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		// Configure the reverse proxy for the chosen uplink.
		rrUrl := rrSelector.Next()
		uplinkUrl, uplinkUrlErr := parseUrl(rrUrl)
		if uplinkUrlErr != nil {
			logger.Error("Failed to parse URL", "url", uplinkUrl)
			http.Error(w, "Uplink Service Unavailable", http.StatusServiceUnavailable)
			return uplinkUrlErr
		}

		// Create a new reverse proxy to uplink
		proxy := makeProxy(config, cache, httpClient, logger)(uplinkUrl, cacheKey, uplinkRequest)

		// Serve the proxied request
		proxy.ServeHTTP(w, r)

		return nil
	}
}

// Handles requests to the relay endpoint.
func RelayHandler(userConfig *config.Config, currentCache cache.Cache, rrSelector *uplink.RoundRobinSelector, httpClient *http.Client, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Debug log the request
		logger.Debug("Received request", "method", r.Method, "path", r.URL.Path, "header", r.Header)

		// Debug log the request heaaders
		debugRequestHeaders(logger, r)

		// Debug log the request body
		debugRequestBody(logger, r)

		// Parse the uplink request body
		uplinkRequest, uplinkRequestErr := parseRequest(r)
		if uplinkRequestErr != nil {
			logger.Error("Failed to parse request body", "err", uplinkRequestErr)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		if uplinkRequest.Variables["graph_ref"] == nil {
			logger.Error("Missing graph_ref in request body")
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		// Parse the GraphRef from the request
		graphID, variantID, graphRefErr := util.ParseGraphRef(uplinkRequest.Variables["graph_ref"].(string))
		if graphRefErr != nil {
			logger.Error("Failed to parse GraphRef from request body")
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		// Get the operation name from the request
		operationName := uplinkRequest.OperationName

		// Remove the api key from cache calculation to avoid uplink-relay having a different key making polling not work
		delete(uplinkRequest.Variables, "apiKey")

		// Make the cache key using the graphID, variantID, and operationName
		cacheKey := cache.MakeCacheKey(graphID, variantID, operationName, uplinkRequest.Variables)
		// If cache is enabled, attempt to retrieve the response from the cache
		if userConfig.Cache.Enabled {
			// Check if the response is cached and return it if found
			if cacheContent, keyFound := currentCache.Get(cacheKey); keyFound {
				// Handle the cache hit
				logger.Debug("Cache hit", "key", cacheKey, "operationName", operationName)
				handleCacheHit(cacheKey, cacheContent, logger, time.Duration(userConfig.Cache.Duration)*time.Second, uplinkRequest.Variables["ifAfterId"].(string))(w, r)
				return
			}

			supergraphConfig, err := config.FindSupergraphConfigFromGraphRef(uplinkRequest.Variables["graph_ref"].(string), userConfig)
			if err != nil {
				logger.Error("Failed to find supergraph config", "err", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// Set it to an empty string if it is nil to avoid panics
			if uplinkRequest.Variables["ifAfterId"] == nil {
				uplinkRequest.Variables["ifAfterId"] = ""
			}

			if operationName == uplink.SupergraphQuery && supergraphConfig.LaunchID != "" {
				s, _ := pinning.HandlePinnedEntry(logger, currentCache, graphID, variantID, operationName, uplinkRequest.Variables["ifAfterId"].(string))
				handleCacheHit(cacheKey, s, logger, time.Duration(userConfig.Cache.Duration)*time.Second, uplinkRequest.Variables["ifAfterId"].(string))(w, r)
				return
			} else if operationName == uplink.LicenseQuery && supergraphConfig.OfflineLicense != "" {
				s, _ := pinning.HandlePinnedEntry(logger, currentCache, graphID, variantID, operationName, uplinkRequest.Variables["ifAfterId"].(string))
				handleCacheHit(cacheKey, s, logger, time.Duration(userConfig.Cache.Duration)*time.Second, uplinkRequest.Variables["ifAfterId"].(string))(w, r)
				return
			} else if operationName == uplink.PersistedQueriesQuery && supergraphConfig.PersistedQueryVersion != "" {
				s, _ := pinning.HandlePinnedEntry(logger, currentCache, graphID, variantID, operationName, uplinkRequest.Variables["ifAfterId"].(string))
				handleCacheHit(cacheKey, s, logger, time.Duration(userConfig.Cache.Duration)*time.Second, uplinkRequest.Variables["ifAfterId"].(string))(w, r)
				return
			}
		}

		// If the response is not cached, proxy the request to the uplink service
		// and cache the response for future requests
		logger.Debug("Cache miss", "key", cacheKey)

		success := false
		for attempt := 0; attempt <= userConfig.Uplink.RetryCount && !success; attempt++ {
			err := handleCacheMiss(userConfig, currentCache, httpClient, rrSelector, cacheKey, uplinkRequest, logger)(w, r)
			if err != nil {
				logger.Error("Request to uplink failed", "attempt", attempt, "err", err)
				if attempt == userConfig.Uplink.RetryCount {
					logger.Error("Failed to proxy request", "attempts", userConfig.Uplink.RetryCount, "err", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
				logger.Warn("Retrying request", "operationName", operationName)
			} else {
				logger.Info("Successfully proxied request", "cacheKey", cacheKey)
				success = true
				break
			}
		}
		if !success {
			logger.Error("Failed to proxy request", "operationName", operationName)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
}

func HandlePinnedEntry(operationName string, systemCache cache.Cache, logger *slog.Logger) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		switch operationName {

		default:
			break
		}
		return nil
	}
}
