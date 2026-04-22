package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

// Config stores authentication and user data
type Config struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	Email        string `json:"email"`
}

var (
	configDir  = filepath.Join(xdg.ConfigHome, "groupfdn")
	configFile = filepath.Join(configDir, "config.json")
)

// Load reads the config from disk
func Load() (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// Save writes the config to disk
func (c *Config) Save() error {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// Clear removes all stored credentials
func (c *Config) Clear() error {
	c.AccessToken = ""
	c.RefreshToken = ""
	c.Email = ""
	return c.Save()
}

// IsAuthenticated returns true if access token exists
func (c *Config) IsAuthenticated() bool {
	return c.AccessToken != ""
}
