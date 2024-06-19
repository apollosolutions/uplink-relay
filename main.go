// A simple caching reverse proxy with round-robin load balancing and configurable uplink targets.

package main

import (
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis"

	Cache "apollosolutions/uplink-relay/cache"
	Config "apollosolutions/uplink-relay/config"
	Logger "apollosolutions/uplink-relay/logger"
	Polling "apollosolutions/uplink-relay/polling"
	Proxy "apollosolutions/uplink-relay/proxy"
	Redis "apollosolutions/uplink-relay/redis"
	Uplink "apollosolutions/uplink-relay/uplink"
	Webhooks "apollosolutions/uplink-relay/webhooks"
)

var (
	// Parse command-line flags.
	configPath  = flag.String("config", "config.yml", "Path to the configuration file")
	enableDebug = flag.Bool("debug", false, "Enable debug logging")
)

// init parses the command-line flags.
func init() {
	flag.Parse()
}

// main contains the main application logic.
func main() {

	// Initialize the logger.
	logger := Logger.MakeLogger(enableDebug)

	// Load the default configuration.
	defaultConfig := Config.NewDefaultConfig()

	// Load the application configuration.
	userConfig, err := Config.LoadConfig(*configPath)
	if err != nil {
		logger.Error("Could not load configuration: %v", err)
		os.Exit(1)
	}

	// Merge the default and user configurations.
	config := Config.MergeWithDefaultConfig(defaultConfig, userConfig, enableDebug, logger)

	// Validate the loaded configuration.
	if err := config.Validate(); err != nil {
		logger.Error("Invalid configuration: %v", err)
		os.Exit(1)
	}

	// Initialize caching based on the configuration.
	var cache Cache.Cache
	if config.Redis.Enabled {
		redisClient := redis.NewClient(&redis.Options{
			Addr:     config.Redis.Address,
			Password: config.Redis.Password,
			DB:       config.Redis.Database,
		})
		cache = Redis.NewRedisCache(redisClient)
	} else {
		cache = Cache.NewMemoryCache(config.Cache.MaxSize)
	}
	// Initialize the round-robin URL selector.
	rrSelector := Uplink.NewRoundRobinSelector(config.Uplink.URLs)

	// Configure the HTTP client with a timeout.
	httpClient := &http.Client{
		Timeout: time.Duration(config.Uplink.Timeout) * time.Second,
	}

	// Set up the main request handler
	Proxy.RegisterHandlers("/*", Proxy.RelayHandler(config, cache, rrSelector, httpClient, logger))

	// Set up the webhook handler if enabled
	if config.Webhook.Enabled {
		Proxy.RegisterHandlers(config.Webhook.Path, Webhooks.WebhookHandler(config, cache, httpClient, logger))
	}

	// Start the polling loop if enabled
	if config.Polling.Enabled {
		go Polling.StartPolling(config, cache, httpClient, logger)
	}

	// Start the server and log its address.
	server, err := Proxy.StartServer(config, logger)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	// Create a channel to listen for interrupt signals.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Wait for an interrupt signal.
	<-stop

	// Shut down the server
	Proxy.ShutdownServer(server, logger)
}
