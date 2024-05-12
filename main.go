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

	// Parse command-line flags.
	configPath := flag.String("config", "config.yml", "Path to the configuration file")
	flag.Parse()

	// Load the application configuration.
	config, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Could not load configuration: %v", err)
	}

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

	// Start the server and log its address.
	server, err := StartServer(config.Relay.Address, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleRequest(w, r, config, cache, rrSelector, httpClient)
	}))
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
