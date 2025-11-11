package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all configuration for queuectl
type Config struct {
	MaxRetries            int    `json:"max_retries"`
	BackoffBase           int    `json:"backoff_base"`
	CommandTimeoutSeconds int    `json:"command_timeout_seconds"`
	WorkerCount           int    `json:"worker_count"`
	DBPath                string `json:"db_path"`
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		MaxRetries:            3,
		BackoffBase:           2,
		CommandTimeoutSeconds: 30,
		WorkerCount:           1,
		DBPath:                filepath.Join(homeDir, ".queuectl", "queuectl.db"),
	}
}

// LoadConfig loads configuration from file, or returns defaults if not found
func LoadConfig() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(homeDir, ".queuectl", "config.json")

	// If file doesn't exist, return defaults
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	// Read and parse config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := DefaultConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// SaveConfig saves configuration to file
func (c *Config) SaveConfig() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configDir := filepath.Join(homeDir, ".queuectl")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.json")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
}

// GetMaxRetries returns the configured max retries
func (c *Config) GetMaxRetries() int {
	if c.MaxRetries <= 0 {
		return 3
	}
	return c.MaxRetries
}

// GetBackoffBase returns the configured backoff base
func (c *Config) GetBackoffBase() int {
	if c.BackoffBase <= 0 {
		return 2
	}
	return c.BackoffBase
}

// GetCommandTimeout returns the configured command timeout
func (c *Config) GetCommandTimeout() int {
	if c.CommandTimeoutSeconds <= 0 {
		return 30
	}
	return c.CommandTimeoutSeconds
}
