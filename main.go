// A simple caching reverse proxy with round-robin load balancing and configurable uplink targets.

package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis"

	"apollosolutions/uplink-relay/cache"
	"apollosolutions/uplink-relay/config"
	"apollosolutions/uplink-relay/graph"
	"apollosolutions/uplink-relay/logger"
	persistedqueries "apollosolutions/uplink-relay/persisted_queries"
	"apollosolutions/uplink-relay/pinning"
	"apollosolutions/uplink-relay/polling"
	"apollosolutions/uplink-relay/proxy"
	apolloredis "apollosolutions/uplink-relay/redis"
	"apollosolutions/uplink-relay/uplink"
	"apollosolutions/uplink-relay/webhooks"

	"github.com/99designs/gqlgen/graphql/handler"
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

	// Create a channel to stop polling on SIGHUP to avoid duplicate polling.
	stopPolling := make(chan bool, 1)

	server, err := startup(mergedConfig, logger, uplinkCache, stopPolling)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	update := make(chan os.Signal, 1)
	signal.Notify(update, syscall.SIGHUP)
	go func() {
		for sig := range update {
			switch sig {
			case syscall.SIGHUP:
				logger.Info("Reloading configuration")
				proxy.ShutdownServer(server, logger)
				stopPolling <- true
				newConfig, err := config.LoadConfig(*configPath)
				if err != nil {
					logger.Error("Could not load configuration", "err", err)
					os.Exit(1)
				}
				server, err = startup(config.MergeWithDefaultConfig(defaultConfig, newConfig, enableDebug, logger), logger, uplinkCache, stopPolling)
				if err != nil {
					logger.Error(err.Error())
					os.Exit(1)
				}
			}
		}
	}()

	// Create a channel to listen for interrupt signals.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Wait for an interrupt signal.
	<-stop

	// Shut down the server
	proxy.ShutdownServer(server, logger)
}

func startup(userConfig *config.Config, logger *slog.Logger, systemCache cache.Cache, stopPolling chan bool) (*http.Server, error) {
	// Initialize the round-robin URL selector.
	rrSelector := uplink.NewRoundRobinSelector(userConfig.Uplink.URLs)

	// Configure the HTTP client with a timeout.
	httpClient := &http.Client{
		Timeout: time.Duration(userConfig.Uplink.Timeout) * time.Second,
	}

	proxy.DeregisterHandlers()
	// Set up the main request handler
	proxy.RegisterHandlers("/*", proxy.RelayHandler(userConfig, systemCache, rrSelector, httpClient, logger))
	proxy.RegisterHandlers("/persisted-queries/*", persistedqueries.PersistedQueryHandler(logger, httpClient, systemCache))
	// Set up the webhook handler if enabled
	if userConfig.Webhook.Enabled {
		proxy.RegisterHandlers(userConfig.Webhook.Path, webhooks.WebhookHandler(userConfig, systemCache, httpClient, logger))
	}

	// Start the polling loop if enabled
	if userConfig.Polling.Enabled {
		go polling.StartPolling(userConfig, systemCache, httpClient, logger, stopPolling)
	}

	for _, supergraph := range userConfig.Supergraphs {
		if supergraph.LaunchID != "" {
			logger.Debug("Pinning launch ID", "graphRef", supergraph.GraphRef, "launchID", supergraph.LaunchID)
			err := pinning.PinLaunchID(userConfig, logger, systemCache, supergraph.LaunchID, supergraph.GraphRef)
			if err != nil {
				logger.Error("Failed to pin launch ID", "graphRef", supergraph.GraphRef, "launchID", supergraph.LaunchID)
			}
		}
		if supergraph.OfflineLicense != "" {
			logger.Debug("Offline license detected", "graphRef", supergraph.GraphRef)
			err := pinning.PinOfflineLicense(userConfig, logger, systemCache, supergraph.OfflineLicense, supergraph.GraphRef)
			if err != nil {
				logger.Error("Failed to pin offline license", "graphRef", supergraph.GraphRef)
			}
		}
	}
	if userConfig.ManagementAPI.Enabled {
		logger.Info("Management API enabled", "path", userConfig.ManagementAPI.Path)
		graphqlHandler := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{Resolvers: &graph.Resolver{}}))
		logger.Info("Starting management API", "path", userConfig.ManagementAPI.Path)
		proxy.RegisterHandlers(userConfig.ManagementAPI.Path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "*")
			resolverContext := &graph.ResolverContext{
				Logger:      logger,
				SystemCache: systemCache,
				UserConfig:  userConfig,
			}
			ctx := context.WithValue(context.Background(), graph.ResolverKey, resolverContext)
			graphqlHandler.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	// Start the server and log its address.
	server, err := proxy.StartServer(userConfig, logger)
	if err != nil {
		logger.Error(err.Error())
		return nil, err
	}

	return server, nil
}
