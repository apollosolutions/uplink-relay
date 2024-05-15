// A simple caching reverse proxy with round-robin load balancing and configurable uplink targets.

package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// main contains the main application logic.
func main() {

	// Initialize the loggerger.
	// logger := makeLogger()

	// Parse command-line flags.
	configPath := flag.String("config", "config.yml", "Path to the configuration file")
	enableDebug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	defaultConfig := NewDefaultConfig()

	// Load the application configuration.
	userConfig, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Could not load configuration: %v", err)
	}

	// Merge the default and user configurations.
	config := MergeWithDefaultConfig(defaultConfig, userConfig, enableDebug)

	// Validate the loaded configuration.
	if err := config.Validate(); err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	// Initialize caching and URL selection mechanisms.
	cache := NewMemoryCache(config.Cache.MaxSize)
	rrSelector := NewRoundRobinSelector(config.Uplink.URLs)

	// Configure the HTTP client with a timeout.
	httpClient := &http.Client{
		Timeout: time.Duration(config.Uplink.Timeout) * time.Second,
	}

	// Set up the main request handler
	RegisterHandlers("/*", relayHandler(config, cache, rrSelector, httpClient, enableDebug))

	// Set up the webhook handler if enabled
	if config.Webhook.Enabled {
		RegisterHandlers(config.Webhook.Path, webhookHandler(config, cache, httpClient, enableDebug))
	}

	// Start the polling loop if enabled
	if config.Polling.Enabled {
		go startPolling(config, cache, httpClient, enableDebug)
	}

	// Start the server and log its address.
	server, err := StartServer(config)
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	// Create a channel to listen for interrupt signals.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Wait for an interrupt signal.
	<-stop

	// Shut down the server
	ShutdownServer(server)
}
