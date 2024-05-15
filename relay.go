package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// Register handlers for relay routes.
func RegisterHandlers(route string, handler http.HandlerFunc) {
	http.HandleFunc(route, handler)
}

// StartServer starts the HTTP server with the given address and handler.
func StartServer(config *Config) (*http.Server, error) {
	address := config.Relay.Address
	log.Printf("Starting Uplink Relay  🛰  at %s\n", address)
	server := &http.Server{Addr: address, Handler: http.DefaultServeMux}
	go func() {
		var err error
		if config.Relay.TLS.CertFile != "" && config.Relay.TLS.KeyFile != "" {
			err = server.ListenAndServeTLS(config.Relay.TLS.CertFile, config.Relay.TLS.KeyFile)
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()
	return server, nil
}

// Shut down the server with a context that times out after 5 seconds.
func ShutdownServer(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Uplink Relay Shutdown: %v", err)
	} else {
		log.Println("Uplink Relay shut down properly")
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

// UplinkRouterEntitlements struct
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
func debugRequestHeaders(enableDebug *bool, r *http.Request) {
	if *enableDebug {
		for name, values := range r.Header {
			for _, value := range values {
				log.Printf("Request header: %s = %s\n", name, value)
			}
		}
	}
}

// Reads and logs the request body if debug mode is enabled.
// It replaces the request body with a new buffer so it can be read again later.
func debugRequestBody(enableDebug *bool, r *http.Request) {
	if *enableDebug {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Failed to read request body: %v\n", err)
		}
		log.Printf("Request body: %s\n", bodyBytes)

		// Replace the body so it can be read again later
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}
}

// Logs the response headers if debug mode is enabled.
func debugResponseHeaders(enableDebug *bool, headers http.Header) {
	if *enableDebug {
		for name, values := range headers {
			for _, value := range values {
				log.Printf("Response header: %s = %s\n", name, value)
			}
		}
	}
}

// Reads and logs the response body if debug mode is enabled.
// It replaces the body with a new buffer so it can be read again later.
func debugResponseBody(enableDebug *bool, r *http.Response) {
	if *enableDebug {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Failed to read response body: %v\n", err)
		}
		log.Printf("Response body: %s\n", bodyBytes)

		// Replace the body so it can be read again later
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}
}

func parseGraphRef(graphRef string) (string, string, error) {
	graphParts := strings.Split(graphRef, "@")
	if len(graphParts) != 2 {
		return "", "", fmt.Errorf("invalid graph_ref: %s", graphRef)
	}
	return graphParts[0], graphParts[1], nil
}

func modifyProxiedResponse(config *Config, cache *MemoryCache, cacheKey string, uplinkRequest UplinkRelayRequest, enableDebug *bool) func(*http.Response) error {
	return func(resp *http.Response) error {
		// Debug log the response headers
		debugResponseHeaders(enableDebug, resp.Header)

		// Debug log the response body
		debugResponseBody(enableDebug, resp)

		// Get the response based on the operation name
		responseStruct, ok := uplinkRelayResponses[uplinkRequest.OperationName]
		if !ok {
			log.Printf("Unknown operation name: %s\n", uplinkRequest.OperationName)
			return nil
		}

		// Decode the response body into the response struct
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Failed to read response body: %v\n", err)
			return nil
		}

		// Unmarshal the response body into the response struct
		err = json.Unmarshal(responseBody, &responseStruct)
		if err != nil {
			log.Printf("Failed to unmarshal response body: %v\n", err)
			return nil
		}

		// Cache the response based on the operation name
		if uplinkRequest.OperationName == "SupergraphSdlQuery" {
			// Assert the type of the response
			uplinkResponse, ok := responseStruct.(*UplinkSupergraphSdlResponse)
			if !ok {
				log.Printf("Failed to assert type of response: expected *UplinkSupergraphSdlResponse, got %T", uplinkResponse)
				return nil
			}

			// Extract the schema from the UplinkResponse
			schema := uplinkResponse.Data.RouterConfig.SupergraphSdl

			// Log the UplinkResponse
			debugLog(enableDebug, "SupergraphSdlQuery response: %+v", uplinkResponse)

			// Cache the response for future requests.
			cache.Set(cacheKey, schema, config.Cache.Duration)
		} else if uplinkRequest.OperationName == "LicenseQuery" {
			// Assert the type of the response
			uplinkResponse, ok := responseStruct.(*UplinkLicenseResponse)
			if !ok {
				log.Printf("Failed to assert type of response: expected *UplinkLicenseResponse, got %T", uplinkResponse)
				return nil
			}

			// Extract the JWT from the LicenseQueryResponse
			jwt := uplinkResponse.Data.RouterEntitlements.Entitlement.Jwt

			// Log the LicenseQueryResponse
			debugLog(enableDebug, "LicenseQuery response: %+v", uplinkResponse)

			// Cache the response for future requests.
			cache.Set(cacheKey, jwt, config.Cache.Duration)

		}

		// Replace the response body with the original data
		resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))

		// Log the proxied response
		debugResponseBody(enableDebug, resp)

		return nil
	}
}

// makeProxy creates a reverse proxy to the target URL.
func makeProxy(config *Config, cache *MemoryCache, httpClient *http.Client, enableDebug *bool) func(*url.URL, string, UplinkRelayRequest) *httputil.ReverseProxy {
	return func(targetURL *url.URL, cacheKey string, uplinkRequest UplinkRelayRequest) *httputil.ReverseProxy {
		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		proxy.Transport = httpClient.Transport
		proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
			log.Printf("HTTP proxy error: %v\n", err)
		}
		proxy.ModifyResponse = modifyProxiedResponse(config, cache, cacheKey, uplinkRequest, enableDebug)
		return proxy
	}
}

func parseUrl(target string) (*url.URL, error) {
	proxyUrl, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}
	return proxyUrl, nil
}

func handleCacheHit(config *Config, cache *MemoryCache, client *http.Client, cacheKey string, content []byte, enableDebug *bool) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
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
			log.Printf("Failed to marshal response: %v\n", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Write the cached content to the response
		_, err = w.Write(responseBody)
		if err != nil {
			log.Printf("Failed to write response: %v\n", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Set the appropriate headers
		w.Header().Set("X-Relay-Cache-Hit", "true")

		// Log the response
		debugLog(enableDebug, "Cached Response: %s", responseBody)
	}
}

func handleCacheMiss(config *Config, cache *MemoryCache, httpClient *http.Client, rrSelector *RoundRobinSelector, cacheKey string, uplinkRequest UplinkRelayRequest, enableDebug *bool) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// Configure the reverse proxy for the chosen uplink.
		rrUrl := rrSelector.Next()
		uplinkUrl, uplinkUrlErr := parseUrl(rrUrl)
		if uplinkUrlErr != nil {
			log.Printf("Failed to parse URL: %v\n", uplinkUrl)
			http.Error(w, "Uplink Service Unavailable", http.StatusServiceUnavailable)
		}

		// Create a new reverse proxy to uplink
		proxy := makeProxy(config, cache, httpClient, enableDebug)(uplinkUrl, cacheKey, uplinkRequest)

		// Serve the proxied request
		proxy.ServeHTTP(w, r)
	}
}

// relayHandler handles requests to the relay endpoint.
func relayHandler(config *Config, cache *MemoryCache, rrSelector *RoundRobinSelector, httpClient *http.Client, enableDebug *bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Debug log the request
		debugLog(enableDebug, "Received request: %s %s %s", r.Method, r.URL.Path, r.Header)

		// Debug log the request heaaders
		debugRequestHeaders(enableDebug, r)

		// Debug log the request body
		debugRequestBody(enableDebug, r)

		// Parse the uplink request body
		uplinkRequest, uplinkRequestErr := parseRequest(r)
		if uplinkRequestErr != nil {
			log.Printf("Failed to parse request body: %v\n", uplinkRequestErr)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		// Parse the GraphRef from the request
		graphID, variantID, graphRefErr := parseGraphRef(uplinkRequest.Variables["graph_ref"].(string))
		if graphRefErr != nil {
			log.Println("Failed to parse GraphRef from request body")
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		// Get the operation name from the request
		operationName := uplinkRequest.OperationName

		// Make the cache key using the graphID, variantID, and operationName
		cacheKey := makeCacheKey(graphID, variantID, operationName)

		// Check if the response is cached and return it if found
		cacheContent, keyFound := cache.Get(cacheKey)

		// Handle the request based on cache hit or miss
		if keyFound {
			// Handle the cache hit
			log.Printf("Cache hit for %s\n", cacheKey)
			handleCacheHit(config, cache, httpClient, cacheKey, cacheContent, enableDebug)(w, r)
		} else {
			// Handle the cache miss
			log.Printf("Cache miss for %s\n", cacheKey)
			handleCacheMiss(config, cache, httpClient, rrSelector, cacheKey, uplinkRequest, enableDebug)(w, r)
		}

	}
}
