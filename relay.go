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

// UplinkResponse struct
type UplinkResponse struct {
	Data struct {
		RouterConfig struct {
			Typename        string  `json:"__typename"`
			ID              string  `json:"id"`
			SupergraphSdl   string  `json:"supergraphSdl"`
			MinDelaySeconds float64 `json:"minDelaySeconds"`
		} `json:"routerConfig"`
	} `json:"data"`
}

// SupergraphSdlQuery struct
type SupergraphSdlQuery struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

// relayHandler handles requests to the relay endpoint.
func relayHandler(config *Config, cache *MemoryCache, rrSelector *RoundRobinSelector, httpClient *http.Client, enableDebug *bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		debugLog(enableDebug, "Received request: %s %s", r.Method, r.URL.Path)

		// Debug log the request headers
		for name, values := range r.Header {
			for _, value := range values {
				debugLog(enableDebug, "Request headers: %s = %s", name, value)
			}
		}

		// Read and log the request body
		var buf bytes.Buffer
		tee := io.TeeReader(r.Body, &buf)
		body, err := io.ReadAll(tee)
		if err != nil {
			log.Printf("Failed to read request body: %v\n", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Debug log the request body
		debugLog(enableDebug, "Request body: %s", buf.String())

		// Replace the request body with the read data.
		r.Body = io.NopCloser(bytes.NewReader(body))

		// Parse the request body to get the graph_ref
		var requestBody SupergraphSdlQuery
		err = json.Unmarshal(body, &requestBody)
		if err != nil {
			log.Printf("Failed to parse request body: %v\n", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		graphRef, ok := requestBody.Variables["graph_ref"].(string)
		if !ok {
			log.Println("Failed to parse graph_ref from request body")
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		// Use the graph_ref as the cache key
		graphParts := strings.Split(graphRef, "@")
		graphID, variantID := graphParts[0], graphParts[1]
		cacheKey := fmt.Sprintf("%s:%s", graphID, variantID)
		content, found := cache.Get(cacheKey)
		if found {
			log.Printf("Cache hit for %s\n", cacheKey)
			cacheResponse := fmt.Sprintf(`{"data":{"routerConfig":{"__typename":"RouterConfigResult","id":"UplinkRelay","supergraphSdl":"%s","minDelaySeconds":0}}}`, string(content))
			w.Write([]byte(cacheResponse))
			return
		}

		// Attempt to retrieve content from the uplink, retrying as configured.
		var lastError error
		for attempts := 0; attempts < config.Uplink.RetryCount; attempts++ {
			targetURL := rrSelector.Next()
			if targetURL == "" {
				log.Println("No upstream URL is available.")
				break
			}

			// Configure the reverse proxy for the chosen uplink.
			proxyURL, err := url.Parse(targetURL)
			if err != nil {
				log.Printf("Failed to parse target URL %s: %v\n", targetURL, err)
				lastError = err
				continue
			}
			proxy := httputil.NewSingleHostReverseProxy(proxyURL)
			proxy.Transport = httpClient.Transport
			proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
				log.Printf("HTTP proxy error: %v\n", err)
				lastError = err
			}
			proxy.ModifyResponse = func(resp *http.Response) error {
				// Log the response headers
				for name, values := range resp.Header {
					for _, value := range values {
						debugLog(enableDebug, "Proxied response headers: %s = %s", name, value)
					}
				}

				// Read and log the response body
				var buf bytes.Buffer
				tee := io.TeeReader(resp.Body, &buf)
				body, err := io.ReadAll(tee)
				if err != nil {
					return fmt.Errorf("failed to read response body: %w", err)
				}
				defer resp.Body.Close()

				// Decode the response body into the UplinkResponse struct
				var response UplinkResponse
				decodeErr := json.NewDecoder(bytes.NewReader(body)).Decode(&response)
				if decodeErr != nil {
					return fmt.Errorf("failed to decode response body: %w", decodeErr)
				}
				// Extract the schema from the UplinkResponse
				schema := response.Data.RouterConfig.SupergraphSdl

				// Log the UplinkResponse
				debugLog(enableDebug, "UplinkResponse: %+v", response)

				// Cache the response for future requests.
				cache.Set(cacheKey, schema, config.Cache.Duration)
				resp.Body = io.NopCloser(bytes.NewReader(body))

				// Log the proxied response
				debugLog(enableDebug, "Proxied response: %s", buf.String())

				lastError = nil // Reset the last error on successful response modification.
				return nil
			}

			proxy.ServeHTTP(w, r) // Serve the proxied request.

			if lastError == nil {
				return // Successful request handling; exit the handler.
			}
		}

		// If all retries failed, report a service unavailable error.
		if lastError != nil {
			log.Printf("Failed after retries, last error: %v\n", lastError)
			http.Error(w, "Uplink Service Unavailable", http.StatusServiceUnavailable)
		} else {
			http.Error(w, "Uplink Service Unavailable", http.StatusServiceUnavailable)
		}
	}
}
