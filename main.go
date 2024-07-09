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

	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/logger"
	persistedqueries "apollosolutions/uplink-relay/persisted_queries"
	"apollosolutions/uplink-relay/polling"
	"apollosolutions/uplink-relay/proxy"
	apolloredis "apollosolutions/uplink-relay/redis"
	"apollosolutions/uplink-relay/uplink"
	"apollosolutions/uplink-relay/webhooks"
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
	logger := logger.MakeLogger(enableDebug)

	// Load the default configuration.
	defaultConfig := config.NewDefaultConfig()

	// Load the application configuration.
	userConfig, err := config.LoadConfig(*configPath)
	if err != nil {
		logger.Error("Could not load configuration", "err", err)
		os.Exit(1)
	}

	// Merge the default and user configurations.
	mergedConfig := config.MergeWithDefaultConfig(defaultConfig, userConfig, enableDebug, logger)

	// Validate the loaded configuration.
	if err := mergedConfig.Validate(); err != nil {
		logger.Error("Invalid configuration", "err", err)
		os.Exit(1)
	}

	// Initialize caching based on the configuration.
	var uplinkCache cache.Cache
	if mergedConfig.Redis.Enabled {
		logger.Info("Using Redis cache", "address", mergedConfig.Redis.Address)
		redisClient := redis.NewClient(&redis.Options{
			Addr:     mergedConfig.Redis.Address,
			Password: mergedConfig.Redis.Password,
			DB:       mergedConfig.Redis.Database,
		})
		redisClient.Ping()
		uplinkCache = apolloredis.NewRedisCache(redisClient)
	} else {
		uplinkCache = cache.NewMemoryCache(mergedConfig.Cache.MaxSize)
	}
	// Initialize the round-robin URL selector.
	rrSelector := uplink.NewRoundRobinSelector(mergedConfig.Uplink.URLs)

	// Configure the HTTP client with a timeout.
	httpClient := &http.Client{
		Timeout: time.Duration(mergedConfig.Uplink.Timeout) * time.Second,
	}

	// Set up the main request handler
	proxy.RegisterHandlers("/*", proxy.RelayHandler(mergedConfig, uplinkCache, rrSelector, httpClient, logger))
	proxy.RegisterHandlers("/persisted-queries/*", persistedqueries.PersistedQueryHandler(logger, httpClient, uplinkCache))
	// Set up the webhook handler if enabled
	if mergedConfig.Webhook.Enabled {
		proxy.RegisterHandlers(mergedConfig.Webhook.Path, webhooks.WebhookHandler(mergedConfig, uplinkCache, httpClient, logger))
	}

	// Start the polling loop if enabled
	if mergedConfig.Polling.Enabled {
		go polling.StartPolling(mergedConfig, uplinkCache, httpClient, logger)
	}

	// Start the server and log its address.
	server, err := proxy.StartServer(mergedConfig, logger)
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
	proxy.ShutdownServer(server, logger)
}
