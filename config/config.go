package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"reflect"
	"slices"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

// Config represents the application's configuration structure,
// housing Relay, Uplink, and Cache configurations.
type Config struct {
	Relay         RelayConfig         `yaml:"relay"`         // RelayConfig for incoming connections.
	Uplink        UplinkConfig        `yaml:"uplink"`        // UplinkConfig for managing uplink configuration.
	Cache         CacheConfig         `yaml:"cache"`         // CacheConfig for cache settings.
	Redis         RedisConfig         `yaml:"redis"`         // RedisConfig for using redis as cache.
	Supergraphs   []SupergraphConfig  `yaml:"supergraphs"`   // SupergraphConfig for supergraph settings.
	Webhook       WebhookConfig       `yaml:"webhook"`       // WebhookConfig for webhook handling.
	Polling       PollingConfig       `yaml:"polling"`       // PollingConfig for polling settings.
	ManagementAPI ManagementAPIConfig `yaml:"managementAPI"` // ManagementAPIConfig for management API settings.
}

// RelayConfig defines the address the proxy server listens on.
type RelayConfig struct {
	Address   string         `yaml:"address"`   // Address to bind the relay server on.
	TLS       RelayTlsConfig `yaml:"tls"`       // TLS configuration for the relay server.
	PublicURL string         `yaml:"publicURL"` // Public URL for the relay server.
}

// RelayTlsConfig defines the TLS configuration for the relay server.
type RelayTlsConfig struct {
	CertFile string `yaml:"cert"` // Path to the certificate file.
	KeyFile  string `yaml:"key"`  // Path to the key file.
}

// UplinkConfig details the configuration for connecting to upstream servers.
type UplinkConfig struct {
	URLs         []string `yaml:"urls"`         // List of URLs to use as uplink targets.
	Timeout      int      `yaml:"timeout"`      // Timeout for uplink requests, in seconds.
	RetryCount   int      `yaml:"retryCount"`   // Number of times to retry on uplink failure.
	StudioAPIURL string   `yaml:"studioAPIURL"` // URL for the Studio API.
}

// CacheConfig specifies the cache duration and max size.
type CacheConfig struct {
	Enabled  bool `yaml:"enabled"`  // Whether caching is enabled.
	Duration int  `yaml:"duration"` // Duration to keep cached content, in seconds.
	MaxSize  int  `yaml:"maxSize"`  // Maximum size of the cache.
}

// RedisConfig defines the configuration for connecting to a Redis cache.
type RedisConfig struct {
	Enabled  bool   `yaml:"enabled"`  // Whether Redis caching is enabled.
	Address  string `yaml:"address"`  // Address of the Redis server.
	Password string `yaml:"password"` // Password for Redis authentication.
	Database int    `yaml:"database"` // Database to use in the Redis server.
}

// WebhookConfig defines the configuration for webhook handling.
type WebhookConfig struct {
	Enabled bool   `yaml:"enabled"` // Whether webhook handling is enabled.
	Path    string `yaml:"path"`    // Path to bind the webhook handler on.
	Secret  string `yaml:"secret"`  // Secret for verifying webhook requests.
}

// PollingConfig defines the configuration for polling from uplink.
type PollingConfig struct {
	Enabled          bool     `yaml:"enabled"`          // Whether polling is enabled.
	Interval         int      `yaml:"interval"`         // Interval for polling, in seconds. Can only use either `interval` or `cronExpression`.
	Expressions      []string `yaml:"cronExpressions"`  // Cron expression to use for polling. Can only use either `interval` or `cronExpression`.
	RetryCount       int      `yaml:"retryCount"`       // Number of times to retry on polling failure.
	Entitlements     *bool    `yaml:"entitlements"`     // Whether to poll for entitlements.
	Supergraph       *bool    `yaml:"supergraph"`       // Whether to poll for supergraph.
	PersistedQueries *bool    `yaml:"persistedQueries"` // Whether to poll for persisted queries.
}

// SupergraphConfig defines the list of graphs to use.
type SupergraphConfig struct {
	GraphRef              string `yaml:"graphRef"`
	ApolloKey             string `yaml:"apolloKey"`
	LaunchID              string `yaml:"launchID"`
	PersistedQueryVersion string `yaml:"persistedQueryVersion"`
	OfflineLicense        string `yaml:"offlineLicense"`
}

type ManagementAPIConfig struct {
	Enabled bool   `yaml:"enabled"` // Whether the management API is enabled.
	Path    string `yaml:"path"`    // Path to bind the management API handler on.
	Secret  string `yaml:"secret"`  // Secret for verifying management API requests.
}

var currentConfig *Config

// NewDefaultConfig creates a new default configuration.
func NewDefaultConfig() *Config {
	pTrue := true
	pFalse := false
	currentConfig = &Config{
		Relay: RelayConfig{
			Address: "localhost:8080",
			TLS:     RelayTlsConfig{},
		},
		Uplink: UplinkConfig{
			URLs:         []string{"http://localhost:8081"},
			Timeout:      30,
			RetryCount:   -1,
			StudioAPIURL: "https://graphql.api.apollographql.com/api/graphql",
		},
		Cache: CacheConfig{
			Enabled:  true,
			Duration: -1,
			MaxSize:  1000,
		},
		Webhook: WebhookConfig{
			Enabled: false,
			Path:    "/webhook",
			Secret:  "",
		},
		Polling: PollingConfig{
			Enabled:          false,
			PersistedQueries: &pFalse,
			Entitlements:     &pTrue,
			Supergraph:       &pTrue,
		},
		ManagementAPI: ManagementAPIConfig{
			Enabled: false,
			Path:    "/graphql",
			Secret:  "",
		},
	}

	return currentConfig
}

type keyType string

const ConfigKey keyType = "config"

// MergeWithDefaultConfig merges the default configuration with the loaded configuration.
func MergeWithDefaultConfig(defaultConfig *Config, loadedConfig *Config, enableDebug *bool, logger *slog.Logger) *Config {
	if loadedConfig.Relay.Address == "" {
		loadedConfig.Relay.Address = defaultConfig.Relay.Address
	}

	if len(loadedConfig.Uplink.URLs) == 0 {
		loadedConfig.Uplink.URLs = defaultConfig.Uplink.URLs
	}

	if loadedConfig.Uplink.Timeout == 0 {
		loadedConfig.Uplink.Timeout = defaultConfig.Uplink.Timeout
	}

	if loadedConfig.Uplink.RetryCount == -1 {
		loadedConfig.Uplink.RetryCount = defaultConfig.Uplink.RetryCount
	}

	if loadedConfig.Cache.Duration == 0 {
		loadedConfig.Cache.Duration = defaultConfig.Cache.Duration
	}

	if loadedConfig.Cache.MaxSize == 0 {
		loadedConfig.Cache.MaxSize = defaultConfig.Cache.MaxSize
	}

	if len(loadedConfig.Supergraphs) == 0 {
		loadedConfig.Supergraphs = defaultConfig.Supergraphs
	}

	if loadedConfig.Webhook.Path == "" {
		loadedConfig.Webhook.Path = defaultConfig.Webhook.Path
	}

	if loadedConfig.Polling.Interval == 0 {
		loadedConfig.Polling.Interval = defaultConfig.Polling.Interval
	}

	if loadedConfig.Polling.Entitlements == nil {
		loadedConfig.Polling.Entitlements = defaultConfig.Polling.Entitlements
	}

	if loadedConfig.Polling.Supergraph == nil {
		loadedConfig.Polling.Supergraph = defaultConfig.Polling.Supergraph
	}

	if loadedConfig.Polling.PersistedQueries == nil {
		loadedConfig.Polling.PersistedQueries = defaultConfig.Polling.PersistedQueries
	}

	if loadedConfig.ManagementAPI.Path == "" {
		loadedConfig.ManagementAPI.Path = defaultConfig.ManagementAPI.Path
	}

	if loadedConfig.Uplink.StudioAPIURL == "" {
		loadedConfig.Uplink.StudioAPIURL = defaultConfig.Uplink.StudioAPIURL
	}

	// Log the final configuration
	logger.Debug("Uplink Relay configuration: %+v", "config", loadedConfig)

	currentConfig = loadedConfig
	return loadedConfig
}

// LoadConfig reads and unmarshals a YAML configuration file into a Config struct.
func LoadConfig(configPath string) (*Config, error) {
	configFile, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer configFile.Close()

	decoder := yaml.NewDecoder(configFile)

	var config Config
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}

	expandEnvInStruct(reflect.ValueOf(&config))

	return &config, nil
}

func FindSupergraphConfigFromGraphRef(graphRef string, userConfig *Config) (*SupergraphConfig, error) {
	for _, supergraph := range userConfig.Supergraphs {
		if supergraph.GraphRef == graphRef {
			return &supergraph, nil
		}
	}
	return nil, fmt.Errorf("supergraph not found for graphRef: %s", graphRef)
}

// expandEnvInStruct expands environment variables in a struct.
// It recursively traverses the struct and expands environment variables in string fields.
// It also expands environment variables in map keys.
//
// We use this to expand environment variables in the configuration file.
func expandEnvInStruct(v reflect.Value) {
	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			return
		}
		v = v.Elem()
		expandEnvInStruct(v)
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			expandEnvInStruct(v.Index(i))
		}
	case reflect.Map:
		newMap := reflect.MakeMap(v.Type())
		for _, key := range v.MapKeys() {
			val := v.MapIndex(key)
			newKey := key
			if key.Kind() == reflect.String {
				newKey = reflect.ValueOf(os.ExpandEnv(key.String()))
			}
			if val.Kind() == reflect.String {
				val = reflect.ValueOf(os.ExpandEnv(val.String()))
			} else {
				expandEnvInStruct(val)
			}
			newMap.SetMapIndex(newKey, val)
		}
		v.Set(newMap)
	case reflect.String:
		if v.CanSet() {
			v.SetString(os.ExpandEnv(v.String()))
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			switch field.Kind() {
			case reflect.Ptr:
				if !field.IsNil() {
					expandEnvInStruct(field)
				}
			default:
				expandEnvInStruct(field)
			}
		}
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	// Validate Relay configuration
	if c.Relay.Address == "" {
		return fmt.Errorf("relay address cannot be empty")
	}

	if c.Relay.PublicURL != "" {
		allowedProtocols := []string{"http", "https"}
		parsedUrl, err := url.Parse(c.Relay.PublicURL)
		if err != nil {
			return fmt.Errorf("invalid publicURL: %s", err)
		}

		if parsedUrl == nil || parsedUrl.Scheme == "" || parsedUrl.Host == "" {
			return fmt.Errorf("invalid publicURL: %s", c.Relay.PublicURL)
		}

		if !slices.Contains(allowedProtocols, parsedUrl.Scheme) {
			return fmt.Errorf(`invalid publicURL scheme "%s"; must be one of "http" or "https"`, parsedUrl.Scheme)
		}

	}
	// Validate Uplink configuration
	if len(c.Uplink.URLs) == 0 {
		return fmt.Errorf("uplink URLs cannot be empty")
	}
	if c.Uplink.Timeout < 0 {
		return fmt.Errorf("uplink timeout cannot be negative")
	}
	if c.Uplink.RetryCount < 1 {
		return fmt.Errorf("uplink retryCount must be at least 1")
	}

	// Validate Cache configuration
	if c.Cache.Duration <= 0 && c.Cache.Duration != -1 {
		return fmt.Errorf("cache duration must be positive")
	}
	if c.Cache.MaxSize <= 0 {
		return fmt.Errorf("cache maxSize must be positive")
	}

	// Validate Webhook configuration
	if c.Webhook.Enabled && c.Webhook.Path == "" {
		return fmt.Errorf("webhook path cannot be empty when webhook is enabled")
	}

	// Validate Polling configuration
	if c.Polling.Enabled {
		if len(c.Polling.Expressions) > 0 {
			if c.Polling.Interval > 0 {
				return fmt.Errorf("cannot use both interval and cronExpressions for polling")
			}
			for _, expression := range c.Polling.Expressions {
				if _, err := cron.ParseStandard(expression); err != nil {
					return fmt.Errorf("invalid cron expression: %s", err)
				}
			}
		} else {
			if c.Polling.Interval <= 0 {
				return fmt.Errorf("polling interval must be positive")
			}

		}
	}

	return nil
}
