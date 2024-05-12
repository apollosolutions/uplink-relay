package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// StartServer starts the HTTP server with the given address and handler.
func StartServer(address string, handler http.HandlerFunc) (*http.Server, error) {
	log.Printf("Starting Uplink Relay 🛰 at %s\n", address)
	server := &http.Server{Addr: address, Handler: handler}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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

func handleRequest(w http.ResponseWriter, r *http.Request, config *Config, cache *MemoryCache, rrSelector *RoundRobinSelector, httpClient *http.Client) {
	cacheKey := fmt.Sprintf("%s-%s", r.URL.Path, r.Header.Get("Accept-Encoding"))
	content, found := cache.Get(cacheKey)
	if found {
		w.Write(content)
		log.Printf("Cache hit for %s\n", cacheKey)
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
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read response body: %w", err)
			}
			defer resp.Body.Close()

			// Cache the response for future requests.
			cache.Set(cacheKey, body, config.Cache.Duration)
			resp.Body = io.NopCloser(bytes.NewReader(body))

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
