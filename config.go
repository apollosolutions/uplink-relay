package main

import (
	"fmt"
	"os"
	"reflect"

	"gopkg.in/yaml.v3"
)

// Config represents the application's configuration structure,
// housing Relay, Uplink, and Cache configurations.
type Config struct {
	Relay       RelayConfig        `yaml:"relay"`       // RelayConfig for incoming connections.
	Uplink      UplinkConfig       `yaml:"uplink"`      // UplinkConfig for managing uplink configuration.
	Cache       CacheConfig        `yaml:"cache"`       // CacheConfig for cache settings.
	Supergraphs []SupergraphConfig `yaml:"supergraphs"` // SupergraphConfig for supergraph settings.
	Webhook     WebhookConfig      `yaml:"webhook"`     // WebhookConfig for webhook handling.
	Polling     PollingConfig      `yaml:"polling"`     // PollingConfig for polling settings.
}

// RelayConfig defines the address the proxy server listens on.
type RelayConfig struct {
	Address string         `yaml:"address"` // Address to bind the relay server on.
	TLS     RelayTlsConfig `yaml:"tls"`     // TLS configuration for the relay server.
}

// RelayTlsConfig defines the TLS configuration for the relay server.
type RelayTlsConfig struct {
	CertFile string `yaml:"cert"` // Path to the certificate file.
	KeyFile  string `yaml:"key"`  // Path to the key file.
}

// UplinkConfig details the configuration for connecting to upstream servers.
type UplinkConfig struct {
	URLs       []string `yaml:"urls"`       // List of URLs to use as uplink targets.
	Timeout    int      `yaml:"timeout"`    // Timeout for uplink requests, in seconds.
	RetryCount int      `yaml:"retryCount"` // Number of times to retry on uplink failure.
}

// CacheConfig specifies the cache duration and max size.
type CacheConfig struct {
	Duration int `yaml:"duration"` // Duration to keep cached content, in seconds.
	MaxSize  int `yaml:"maxSize"`  // Maximum size of the cache.
}

// WebhookConfig defines the configuration for webhook handling.
type WebhookConfig struct {
	Enabled bool   `yaml:"enabled"` // Whether webhook handling is enabled.
	Path    string `yaml:"path"`    // Path to bind the webhook handler on.
	Secret  string `yaml:"secret"`  // Secret for verifying webhook requests.
}

// PollingConfig defines the configuration for polling from uplink.
type PollingConfig struct {
	Enabled  bool `yaml:"enabled"`  // Whether polling is enabled.
	Interval int  `yaml:"interval"` // Interval for polling, in seconds.
}

// SupergraphConfig defines the list of graphs to use.
type SupergraphConfig struct {
	GraphRef  string `yaml:"graphRef"`
	ApolloKey string `yaml:"apolloKey"`
}

// NewDefaultConfig creates a new default configuration.
func NewDefaultConfig() *Config {
	return &Config{
		Relay: RelayConfig{
			Address: "localhost:8080",
			TLS:     RelayTlsConfig{},
		},
		Uplink: UplinkConfig{
			URLs:       []string{"http://localhost:8081"},
			Timeout:    30,
			RetryCount: -1,
		},
		Cache: CacheConfig{
			Duration: -1,
			MaxSize:  1000,
		},
		Webhook: WebhookConfig{
			Enabled: false,
			Path:    "/webhook",
			Secret:  "",
		},
		Polling: PollingConfig{
			Enabled:  false,
			Interval: 60,
		},
	}
}

// MergeWithDefaultConfig merges the default configuration with the loaded configuration.
func MergeWithDefaultConfig(defaultConfig *Config, loadedConfig *Config, enableDebug *bool) *Config {
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

	if loadedConfig.Cache.Duration == -1 {
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

	// Log the final configuration
	debugLog(enableDebug, "Uplink Relay configuration: %+v", loadedConfig)

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

func (c *Config) Validate() error {
	// Validate Relay configuration
	if c.Relay.Address == "" {
		return fmt.Errorf("relay address cannot be empty")
	}
	// if c.Relay.TLS.CertFile == "" {
	// 	return fmt.Errorf("relay certFile cannot be empty")
	// }
	// if c.Relay.TLS.KeyFile == "" {
	// 	return fmt.Errorf("relay keyFile cannot be empty")
	// }

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
	if c.Cache.Duration <= 0 {
		return fmt.Errorf("cache duration must be positive")
	}
	if c.Cache.MaxSize <= 0 {
		return fmt.Errorf("cache maxSize must be positive")
	}

	// Validate Webhook configuration
	if c.Webhook.Enabled && c.Webhook.Path == "" {
		return fmt.Errorf("webhook path cannot be empty when webhook is enabled")
	}

	return nil
}
