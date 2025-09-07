// Package config provides configuration management for the pfSense container controller
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
)

// Config represents the main configuration structure
type Config struct {
	Endpoints []EndpointConfig `toml:"endpoints"`
	Global    GlobalConfig     `toml:"global"`
}

// GlobalConfig contains global controller settings
type GlobalConfig struct {
	LogLevel          string   `toml:"log_level"`
	PollInterval      duration `toml:"poll_interval"`
	RetryDelay        duration `toml:"retry_delay"`
	RetryAttempts     int      `toml:"retry_attempts"`
	HealthPort        int      `toml:"health_port"`
	TraefikCompatMode bool     `toml:"traefik_compat_mode"`
}

// EndpointConfig represents a pfSense endpoint configuration
type EndpointConfig struct {
	Name           string   `toml:"name"`
	URL            string   `toml:"url"`
	APIKey         string   `toml:"api_key"`
	RequestTimeout duration `toml:"request_timeout"`
	InsecureTLS    bool     `toml:"insecure_tls"`
}

// duration is a custom type to handle TOML duration parsing
type duration struct {
	time.Duration
}

func (d *duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

// LoadConfig loads configuration from file and applies environment overrides
func LoadConfig(configPath string) (*Config, error) {
	config := &Config{
		Global: GlobalConfig{
			PollInterval:      duration{30 * time.Second},
			RetryAttempts:     3,
			RetryDelay:        duration{5 * time.Second},
			LogLevel:          "info",
			HealthPort:        8080,
			TraefikCompatMode: false,
		},
	}

	// Load from TOML file if it exists
	if _, err := os.Stat(configPath); err == nil {
		if _, err := toml.DecodeFile(configPath, config); err != nil {
			return nil, fmt.Errorf("failed to decode config file: %w", err)
		}
	}

	// Apply environment variable overrides
	if pollInterval := os.Getenv("PFSENSE_POLL_INTERVAL"); pollInterval != "" {
		if d, err := time.ParseDuration(pollInterval); err == nil {
			config.Global.PollInterval.Duration = d
		}
	}

	if logLevel := os.Getenv("PFSENSE_LOG_LEVEL"); logLevel != "" {
		config.Global.LogLevel = logLevel
	}

	if healthPort := os.Getenv("PFSENSE_HEALTH_PORT"); healthPort != "" {
		if port := parseInt(healthPort, config.Global.HealthPort); port > 0 {
			config.Global.HealthPort = port
		}
	}

	if traefikMode := os.Getenv("PFSENSE_TRAEFIK_COMPAT_MODE"); traefikMode != "" {
		config.Global.TraefikCompatMode = parseBool(traefikMode, false)
	}

	// Load endpoints from environment if no endpoints defined in config
	if len(config.Endpoints) == 0 {
		if url := os.Getenv("PFSENSE_URL"); url != "" {
			endpoint := EndpointConfig{
				Name:           "default",
				URL:            url,
				APIKey:         os.Getenv("PFSENSE_API_KEY"),
				InsecureTLS:    parseBool(os.Getenv("PFSENSE_INSECURE_TLS"), false),
				RequestTimeout: duration{30 * time.Second},
			}
			config.Endpoints = append(config.Endpoints, endpoint)
		}
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// Validate ensures the configuration is valid
func (c *Config) Validate() error {
	if len(c.Endpoints) == 0 {
		return fmt.Errorf("at least one endpoint must be configured")
	}

	for i, endpoint := range c.Endpoints {
		if endpoint.Name == "" {
			return fmt.Errorf("endpoint %d: name is required", i)
		}
		if endpoint.URL == "" {
			return fmt.Errorf("endpoint %s: URL is required", endpoint.Name)
		}
		if endpoint.APIKey == "" {
			return fmt.Errorf("endpoint %s: API key is required", endpoint.Name)
		}
	}

	if c.Global.PollInterval.Duration <= 0 {
		return fmt.Errorf("poll_interval must be positive")
	}

	if c.Global.RetryAttempts < 0 {
		return fmt.Errorf("retry_attempts must be non-negative")
	}

	return nil
}

// GetEndpoint returns an endpoint by name, or nil if not found
func (c *Config) GetEndpoint(name string) *EndpointConfig {
	for _, endpoint := range c.Endpoints {
		if endpoint.Name == name {
			return &endpoint
		}
	}
	return nil
}

// GetDefaultEndpoint returns the first endpoint (used as default)
func (c *Config) GetDefaultEndpoint() *EndpointConfig {
	if len(c.Endpoints) > 0 {
		return &c.Endpoints[0]
	}
	return nil
}

func parseInt(s string, defaultValue int) int {
	var result int
	if _, err := fmt.Sscanf(s, "%d", &result); err != nil {
		return defaultValue
	}
	return result
}

// parseBool parses a string to boolean with a default value
func parseBool(s string, defaultValue bool) bool {
	if s == "" {
		return defaultValue
	}
	if result, err := strconv.ParseBool(s); err == nil {
		return result
	}
	return defaultValue
}
