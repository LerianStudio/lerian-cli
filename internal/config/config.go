// Package config handles configuration management for the Lerian CLI.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Profile represents a single configuration profile
type Profile struct {
	APIURL   string `yaml:"api-url"`
	APIKey   string `yaml:"api-key"`
	TenantID string `yaml:"tenant-id"`
}

// Config represents the entire configuration file
type Config struct {
	CurrentProfile string             `yaml:"current-profile"`
	Profiles       map[string]Profile `yaml:"profiles"`

	// TemplatesCheckout is where lerian-terraform-foundation was cloned.
	//
	// It sits outside the profiles because it describes this machine rather than
	// an account: the same checkout drives dev, stg and prd, and which one is
	// current says nothing about where the templates live.
	TemplatesCheckout string `yaml:"templates-checkout,omitempty"`
}

// GetConfigPath returns the path to the config file
func GetConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".lerian", "config.yaml"), nil
}

// Load loads the configuration from the config file
func Load() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	// If config doesn't exist, return empty config
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &Config{
			CurrentProfile: "default",
			Profiles:       make(map[string]Profile),
		}, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// Save saves the configuration to the config file
func (c *Config) Save() error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	// Create .lerian directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetProfile returns the specified profile or the current profile if name is empty
func (c *Config) GetProfile(name string) (*Profile, error) {
	if name == "" {
		name = c.CurrentProfile
	}

	profile, ok := c.Profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile '%s' not found", name)
	}

	return &profile, nil
}

// SetProfile sets or updates a profile
func (c *Config) SetProfile(name string, profile Profile) {
	if c.Profiles == nil {
		c.Profiles = make(map[string]Profile)
	}
	c.Profiles[name] = profile
}

// DeleteProfile removes a profile
func (c *Config) DeleteProfile(name string) error {
	if name == c.CurrentProfile {
		return fmt.Errorf("cannot delete current profile")
	}

	if _, ok := c.Profiles[name]; !ok {
		return fmt.Errorf("profile '%s' not found", name)
	}

	delete(c.Profiles, name)
	return nil
}
