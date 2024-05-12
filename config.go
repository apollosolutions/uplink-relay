package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the application's configuration structure,
// housing Relay, Uplink, and Cache configurations.
type Config struct {
	Relay  RelayConfig  `yaml:"relay"`  // RelayConfig for incoming connections.
	Uplink UplinkConfig `yaml:"uplink"` // UplinkConfig for managing uplink configuration.
	Cache  CacheConfig  `yaml:"cache"`  // CacheConfig for cache settings.
}

// RelayConfig defines the address the proxy server listens on.
type RelayConfig struct {
	Address string `yaml:"address"` // Address to bind the relay server on.
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

// LoadConfig reads and unmarshals a YAML configuration file into a Config struct.
func LoadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}
	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling config YAML: %w", err)
	}
	return &config, nil
}

func (c *Config) Validate() error {
	// Validate Relay configuration
	if c.Relay.Address == "" {
		return fmt.Errorf("relay address cannot be empty")
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
	if c.Cache.Duration <= 0 {
		return fmt.Errorf("cache duration must be positive")
	}
	if c.Cache.MaxSize <= 0 {
		return fmt.Errorf("cache maxSize must be positive")
	}

	return nil
}
