package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const defaultBaseURL = "http://localhost:3000"

// Config holds the agent's saved credentials and preferences.
type Config struct {
	Name    string `json:"name"`
	Token   string `json:"token"`
	BaseURL string `json:"base_url"`
}

func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".auralens"), nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads config from disk; returns defaults if no file exists yet.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{BaseURL: defaultBaseURL}, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	return &cfg, nil
}

// Save writes config to disk (mode 0600).
func Save(cfg *Config) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0600)
}

// Clear removes the config file.
func Clear() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// IsLoggedIn returns true when both name and token are set.
func (c *Config) IsLoggedIn() bool {
	return c.Name != "" && c.Token != ""
}
