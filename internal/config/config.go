// Package config provides configuration management for the application.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// Default configuration values
	DefaultHealthCheckInterval  = 2 * time.Second
	DefaultHealthCheckTimeout   = 1 * time.Second
	DefaultHealthCheckFailCount = 2
	DefaultErrorAutoClearDelay  = 3 * time.Second
	DefaultUIRefreshRate        = 100 * time.Millisecond
	DefaultLogMaxSize           = 10 // MB
	DefaultLogMaxBackups        = 3
)

// Config holds the application configuration.
type Config struct {
	// HealthCheckInterval is how often to check service health
	HealthCheckInterval time.Duration `json:"health_check_interval"`

	// HealthCheckTimeout is the timeout for each health check
	HealthCheckTimeout time.Duration `json:"health_check_timeout"`

	// HealthCheckFailCount is how many consecutive failures before marking as ERROR
	HealthCheckFailCount int `json:"health_check_fail_count"`

	// ErrorAutoClearDelay is how long to wait before clearing errors after recovery
	ErrorAutoClearDelay time.Duration `json:"error_auto_clear_delay"`

	// UIRefreshRate is how often to refresh the UI
	UIRefreshRate time.Duration `json:"ui_refresh_rate"`

	// LogMaxSize is the maximum size of log file in MB
	LogMaxSize int `json:"log_max_size"`

	// LogMaxBackups is the number of log backups to keep
	LogMaxBackups int `json:"log_max_backups"`
}

// Load loads configuration from file or returns default config.
func Load() (*Config, error) {
	cfg := &Config{
		HealthCheckInterval:  DefaultHealthCheckInterval,
		HealthCheckTimeout:   DefaultHealthCheckTimeout,
		HealthCheckFailCount: DefaultHealthCheckFailCount,
		ErrorAutoClearDelay:  DefaultErrorAutoClearDelay,
		UIRefreshRate:        DefaultUIRefreshRate,
		LogMaxSize:           DefaultLogMaxSize,
		LogMaxBackups:        DefaultLogMaxBackups,
	}

	configPath := getConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Config file doesn't exist, return defaults
		return cfg, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Unmarshal into a temporary struct to handle time.Duration
	var raw struct {
		HealthCheckInterval  int `json:"health_check_interval"` // seconds
		HealthCheckTimeout   int `json:"health_check_timeout"`  // seconds
		HealthCheckFailCount int `json:"health_check_fail_count"`
		ErrorAutoClearDelay  int `json:"error_auto_clear_delay"` // seconds
		UIRefreshRate        int `json:"ui_refresh_rate"`        // milliseconds
		LogMaxSize           int `json:"log_max_size"`
		LogMaxBackups        int `json:"log_max_backups"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply values from config file
	if raw.HealthCheckInterval > 0 {
		cfg.HealthCheckInterval = time.Duration(raw.HealthCheckInterval) * time.Second
	}
	if raw.HealthCheckTimeout > 0 {
		cfg.HealthCheckTimeout = time.Duration(raw.HealthCheckTimeout) * time.Second
	}
	if raw.HealthCheckFailCount > 0 {
		cfg.HealthCheckFailCount = raw.HealthCheckFailCount
	}
	if raw.ErrorAutoClearDelay > 0 {
		cfg.ErrorAutoClearDelay = time.Duration(raw.ErrorAutoClearDelay) * time.Second
	}
	if raw.UIRefreshRate > 0 {
		cfg.UIRefreshRate = time.Duration(raw.UIRefreshRate) * time.Millisecond
	}
	if raw.LogMaxSize > 0 {
		cfg.LogMaxSize = raw.LogMaxSize
	}
	if raw.LogMaxBackups > 0 {
		cfg.LogMaxBackups = raw.LogMaxBackups
	}

	return cfg, nil
}

// Save saves the configuration to file.
func (c *Config) Save() error {
	configPath := getConfigPath()

	// Convert to raw format for saving
	raw := struct {
		HealthCheckInterval  int `json:"health_check_interval"`
		HealthCheckTimeout   int `json:"health_check_timeout"`
		HealthCheckFailCount int `json:"health_check_fail_count"`
		ErrorAutoClearDelay  int `json:"error_auto_clear_delay"`
		UIRefreshRate        int `json:"ui_refresh_rate"`
		LogMaxSize           int `json:"log_max_size"`
		LogMaxBackups        int `json:"log_max_backups"`
	}{
		HealthCheckInterval:  int(c.HealthCheckInterval.Seconds()),
		HealthCheckTimeout:   int(c.HealthCheckTimeout.Seconds()),
		HealthCheckFailCount: c.HealthCheckFailCount,
		ErrorAutoClearDelay:  int(c.ErrorAutoClearDelay.Seconds()),
		UIRefreshRate:        int(c.UIRefreshRate.Milliseconds()),
		LogMaxSize:           c.LogMaxSize,
		LogMaxBackups:        c.LogMaxBackups,
	}

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func getConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.json"
	}
	exeDir := filepath.Dir(exe)
	return filepath.Join(exeDir, "config.json")
}
