package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type SchemaChange struct {
	Description string `json:"description"`
}

type WebhookData struct {
	EventType          string         `json:"eventType"`
	EventID            string         `json:"eventID"`
	Changes            []SchemaChange `json:"changes"`
	SchemaURL          string         `json:"schemaURL"`
	SchemaURLExpiresAt time.Time      `json:"schemaURLExpiresAt"`
	GraphID            string         `json:"graphID"`
	VariantID          string         `json:"variantID"`
	Timestamp          time.Time      `json:"timestamp"`
}

func webhookHandler(config *Config, cache *MemoryCache, httpClient *http.Client, enableDebug *bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the incoming webhook data
		var data WebhookData
		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Check if the GraphRef is in the list of graphs from the configuration
		graph := fmt.Sprintf("%s@%s", data.GraphID, data.VariantID)
		if !contains(config.Graphs.GraphRefs, graph) {
			http.Error(w, "Graph not in the list of graphs", http.StatusBadRequest)
			return
		}

		// Fetch the schema using the SchemaURL from the webhook data
		resp, err := httpClient.Get(data.SchemaURL)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to fetch schema: %v", err), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		// Parse the fetched schema
		response, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to read schema: %v", err), http.StatusInternalServerError)
			return
		}
		// Convert the schema to a string
		schema := string(response)

		if config.Webhook.Cache {
			// Create a cache key using the GraphID, VariantID
			cacheKey := fmt.Sprintf("%s:%s", data.GraphID, data.VariantID)

			// Set the cache TTL based on the Cache duration from the configuration
			ttl := time.Duration(config.Cache.Duration) * time.Second
			if ttl == 0 {
				ttl = -1 // Cache indefinitely if duration is 0
			}

			// Update the cache using the fetched schema
			cache.Set(cacheKey, schema, int(ttl.Seconds()))
		}

		// Send a response back to the webhook sender
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Webhook processed successfully")
	}
}

// Helper function to check if a slice contains a string
func contains(slice []string, str string) bool {
	for _, v := range slice {
		if v == str {
			return true
		}
	}
	return false
}
