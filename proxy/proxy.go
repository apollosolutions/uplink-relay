package proxy

import (
	"bytes"
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

	Cache "apollosolutions/uplink-relay/cache"
	Config "apollosolutions/uplink-relay/config"
	Uplink "apollosolutions/uplink-relay/uplink"
)

// Register handlers for proxy routes.
func RegisterHandlers(route string, handler http.HandlerFunc) {
	http.HandleFunc(route, handler)
}

// StartServer starts the HTTP server with the given address and handler.
func StartServer(config *Config.Config, logger *slog.Logger) (*http.Server, error) {
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
			logger.Error("ListenAndServe(): %v", err)
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
		logger.Error("Uplink Relay Shutdown: %v", err)
	} else {
		logger.Info("Uplink Relay shut down properly")
	}
}

// UplinkRouterConfig struct
type UplinkRouterConfig struct {
	Typename        string  `json:"__typename"`
	ID              string  `json:"id"`
	SupergraphSdl   string  `json:"supergraphSdl"`
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
	Entitlement     Jwt     `json:"entitlement"`
}

// UplinkLicenseResponse struct
type UplinkLicenseResponse struct {
	Data struct {
		RouterEntitlements UplinkRouterEntitlements `json:"routerEntitlements"`
	} `json:"data"`
}

// uplinkRelayResponses maps operation names to response structs.
var uplinkRelayResponses = map[string]interface{}{
	"SupergraphSdlQuery": &UplinkSupergraphSdlResponse{},
	"LicenseQuery":       &UplinkLicenseResponse{},
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
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("Failed to read request body", "err", err)
	}
	logger.Info("Request body", "body", bodyBytes)

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
		logger.Error("Failed to read response body: %v\n", err)
	}
	logger.Info("Response Body", "body", bodyBytes)

	// Replace the body so it can be read again later
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
}

// Parses the graph_ref into graphID and variantID.
func ParseGraphRef(graphRef string) (string, string, error) {
	graphParts := strings.Split(graphRef, "@")
	if len(graphParts) != 2 {
		return "", "", fmt.Errorf("invalid graph_ref: %s", graphRef)
	}
	return graphParts[0], graphParts[1], nil
}

// Modifies the proxied response before it is returned to the client.
func modifyProxiedResponse(config *Config.Config, cache Cache.Cache, cacheKey string, uplinkRequest UplinkRelayRequest, logger *slog.Logger) func(*http.Response) error {
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

		// Decode the response body into the response struct
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Error("Failed to read response body: %v\n", err)
			return nil
		}

		// Unmarshal the response body into the response struct
		err = json.Unmarshal(responseBody, &responseStruct)
		if err != nil {
			logger.Error("Failed to unmarshal response body: %v\n", err)
			return nil
		}

		// Cache the response based on the operation name
		if uplinkRequest.OperationName == "SupergraphSdlQuery" {
			// Assert the type of the response
			uplinkResponse, ok := responseStruct.(*UplinkSupergraphSdlResponse)
			if !ok {
				logger.Error("Failed to assert type of response: expected *UplinkSupergraphSdlResponse, got %T", uplinkResponse)
				return nil
			}

			// Extract the schema from the UplinkResponse
			schema := uplinkResponse.Data.RouterConfig.SupergraphSdl

			// Log the UplinkResponse
			logger.Debug("SupergraphSdlQuery response", "response", uplinkResponse)

			// Cache the response for future requests.
			cache.Set(cacheKey, schema, config.Cache.Duration)
		} else if uplinkRequest.OperationName == "LicenseQuery" {
			// Assert the type of the response
			uplinkResponse, ok := responseStruct.(*UplinkLicenseResponse)
			if !ok {
				logger.Error("Failed to assert type of response: expected *UplinkLicenseResponse, got %T", uplinkResponse)
				return nil
			}

			// Extract the JWT from the LicenseQueryResponse
			jwt := uplinkResponse.Data.RouterEntitlements.Entitlement.Jwt

			// Log the LicenseQueryResponse
			logger.Debug("LicenseQuery response", "response", uplinkResponse)

			// Cache the response for future requests, if caching is enabled
			if config.Cache.Enabled {
				logger.Debug("Caching JWT", "key", cacheKey)
				cache.Set(cacheKey, jwt, config.Cache.Duration)
			}

		}

		// Replace the response body with the original data
		resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))

		// Log the proxied response
		debugResponseBody(logger, resp)

		return nil
	}
}

// Creates a reverse proxy to the target URL.
func makeProxy(config *Config.Config, cache Cache.Cache, httpClient *http.Client, logger *slog.Logger) func(*url.URL, string, UplinkRelayRequest) *httputil.ReverseProxy {
	return func(targetURL *url.URL, cacheKey string, uplinkRequest UplinkRelayRequest) *httputil.ReverseProxy {
		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		proxy.Transport = httpClient.Transport
		proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
			logger.Error("HTTP proxy error: %v\n", err)
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
func handleCacheHit(_ *Config.Config, _ Cache.Cache, _ *http.Client, cacheKey string, content []byte, logger *slog.Logger) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var response interface{}

		// Format the response body based on operation name
		if strings.Contains(cacheKey, "SupergraphSdlQuery") {
			response = &UplinkSupergraphSdlResponse{
				Data: struct {
					RouterConfig UplinkRouterConfig `json:"routerConfig"`
				}{
					RouterConfig: UplinkRouterConfig{
						ID:              time.Now().UTC().String(),
						Typename:        "RouterConfigResult",
						SupergraphSdl:   string(content),
						MinDelaySeconds: 30,
					},
				},
			}
		} else if strings.Contains(cacheKey, "LicenseQuery") {
			response = &UplinkLicenseResponse{
				Data: struct {
					RouterEntitlements UplinkRouterEntitlements `json:"routerEntitlements"`
				}{
					RouterEntitlements: UplinkRouterEntitlements{
						ID:              time.Now().UTC().String(),
						Typename:        "RouterEntitlementsResult",
						MinDelaySeconds: 60,
						Entitlement:     Jwt{Jwt: string(content)},
					},
				},
			}
		}

		// Convert the response to JSON
		responseBody, err := json.Marshal(response)
		if err != nil {
			logger.Error("Failed to marshal response: %v\n", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return nil
		}

		// Write the cached content to the response
		_, err = w.Write(responseBody)
		if err != nil {
			logger.Error("Failed to write response: %v\n", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return nil
		}

		// Set the appropriate headers
		w.Header().Set("X-Cache-Hit", "true")

		// Log the response
		logger.Debug("Cached Response", "response", responseBody)

		return nil
	}
}

// Handles a cache miss by proxying the request to the uplink service.
func handleCacheMiss(config *Config.Config, cache Cache.Cache, httpClient *http.Client, rrSelector *Uplink.RoundRobinSelector, cacheKey string, uplinkRequest UplinkRelayRequest, logger *slog.Logger) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		// Configure the reverse proxy for the chosen uplink.
		rrUrl := rrSelector.Next()
		uplinkUrl, uplinkUrlErr := parseUrl(rrUrl)
		if uplinkUrlErr != nil {
			logger.Error("Failed to parse URL", "url", uplinkUrl)
			http.Error(w, "Uplink Service Unavailable", http.StatusServiceUnavailable)
		}

		// Create a new reverse proxy to uplink
		proxy := makeProxy(config, cache, httpClient, logger)(uplinkUrl, cacheKey, uplinkRequest)

		// Serve the proxied request
		proxy.ServeHTTP(w, r)

		return nil
	}
}

// Handles requests to the relay endpoint.
func RelayHandler(config *Config.Config, cache Cache.Cache, rrSelector *Uplink.RoundRobinSelector, httpClient *http.Client, logger *slog.Logger) http.HandlerFunc {
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
			logger.Error("Failed to parse request body: %v\n", uplinkRequestErr)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		// Parse the GraphRef from the request
		graphID, variantID, graphRefErr := ParseGraphRef(uplinkRequest.Variables["graph_ref"].(string))
		if graphRefErr != nil {
			logger.Error("Failed to parse GraphRef from request body")
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		// Get the operation name from the request
		operationName := uplinkRequest.OperationName

		// Make the cache key using the graphID, variantID, and operationName
		cacheKey := Cache.MakeCacheKey(graphID, variantID, operationName)

		// If cache is enabled, attempt to retrieve the response from the cache
		if config.Cache.Enabled {

			// Check if the response is cached and return it if found
			if cacheContent, keyFound := cache.Get(cacheKey); keyFound {
				// Handle the cache hit
				logger.Info("Cache hit", "key", cacheKey)
				handleCacheHit(config, cache, httpClient, cacheKey, cacheContent, logger)(w, r)
				return
			}

		}

		// If the response is not cached, proxy the request to the uplink service
		// and cache the response for future requests
		logger.Info("Cache miss", "key", cacheKey)

		success := false
		for attempt := 0; attempt <= config.Uplink.RetryCount && !success; attempt++ {
			err := handleCacheMiss(config, cache, httpClient, rrSelector, cacheKey, uplinkRequest, logger)(w, r)
			if err != nil {
				logger.Error("Request to uplink failed", "attempt", attempt, "err", err)
				if attempt == config.Uplink.RetryCount {
					logger.Error("Failed to proxy request", "attempts", config.Uplink.RetryCount, "err", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
				logger.Info("Retrying...")
			} else {
				logger.Info("Successfully proxied request", "cacheKey", cacheKey)
				success = true
				break
			}
		}

	}
}
