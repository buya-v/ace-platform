// Package config provides venue configuration loading for the platform-service.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ConfigLoader loads venue configuration files from the venues directory.
type ConfigLoader struct {
	venuesDir string
}

// NewConfigLoader creates a ConfigLoader that reads from venuesDir.
// If venuesDir is empty, it defaults to "./venues".
func NewConfigLoader(venuesDir string) *ConfigLoader {
	if venuesDir == "" {
		venuesDir = "./venues"
	}
	return &ConfigLoader{venuesDir: venuesDir}
}

// LoadConfig reads venues/{tenantID}/config.json and returns the parsed JSON as a
// map[string]interface{}. Returns an error if the file does not exist or is malformed.
func (cl *ConfigLoader) LoadConfig(tenantID string) (map[string]interface{}, error) {
	path := filepath.Join(cl.venuesDir, tenantID, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config not found for tenant %q", tenantID)
		}
		return nil, fmt.Errorf("reading config for tenant %q: %w", tenantID, err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config for tenant %q: %w", tenantID, err)
	}
	return cfg, nil
}
