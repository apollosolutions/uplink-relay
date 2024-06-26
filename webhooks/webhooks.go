package webhooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/proxy"
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

func WebhookHandler(config *config.Config, systemCache cache.Cache, httpClient *http.Client, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Verify the request signature
		signatureHeader := r.Header.Get("x-apollo-signature")
		if signatureHeader == "" {
			http.Error(w, "Missing signature", http.StatusBadRequest)
			return
		}

		// Extract the signature algorithm and value
		parts := strings.SplitN(signatureHeader, "=", 2)
		if len(parts) != 2 || parts[0] != "sha256" {
			http.Error(w, "Invalid signature", http.StatusBadRequest)
			return
		}

		// Verify the signature
		secret := config.Webhook.Secret
		if secret == "" {
			http.Error(w, "Webhook secret not configured", http.StatusBadRequest)
			return
		}

		body, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		// Read the request body and compute the HMAC
		mac := hmac.New(sha256.New, []byte(secret))
		_, err = io.Copy(mac, bytes.NewReader(body))
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to read request body: %v", err), http.StatusInternalServerError)
			return
		}

		// Compare the computed HMAC with the expected HMAC
		expectedMAC := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(parts[1]), []byte(expectedMAC)) {
			http.Error(w, "Invalid signature", http.StatusBadRequest)
			return
		}

		// Parse the incoming webhook data
		var data WebhookData
		err = json.Unmarshal(body, &data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Check if the variantID is in the list of graphs from the configuration
		// webhook variantID is in the format of a GraphRef
		if !containsGraph(config.Supergraphs, data.VariantID) {
			http.Error(w, fmt.Sprintf("VariantID %s not found in the list of supergraphs", data.VariantID), http.StatusBadRequest)
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

		// Parse the GraphID and VariantID from the webhook data
		graphID, variantID, err := proxy.ParseGraphRef(data.VariantID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to parse variantID from webhook: %s", data.VariantID), http.StatusInternalServerError)
			return
		}

		if config.Cache.Enabled {
			// Create a cache key using the GraphID, VariantID
			cacheKey := cache.MakeCacheKey(graphID, variantID, "SupergraphSdlQuery")
			// Update the cache using the fetched schema
			systemCache.Set(cacheKey, schema, config.Cache.Duration)
		} else {
			logger.Info("Cache is disabled, skipping cache update for GraphID %s, VariantID %s", graphID, variantID)
		}

		// Send a response back to the webhook sender
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Webhook processed successfully")
	}
}

// Helper function to check if a configs contains variantID
func containsGraph(configs []config.SupergraphConfig, variantID string) bool {
	for _, item := range configs {
		if item.GraphRef == variantID {
			return true
		}
	}
	return false
}
