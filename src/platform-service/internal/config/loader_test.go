// Package config_test exercises the venue ConfigLoader used by the platform
// control-plane to serve per-tenant onboarding configuration.
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/garudax-platform/platform-service/internal/config"
)

// writeConfig writes a config.json for tenantID under venuesDir.
func writeConfig(t *testing.T, venuesDir, tenantID, contents string) {
	t.Helper()
	dir := filepath.Join(venuesDir, tenantID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// TestNewConfigLoader_DefaultsVenuesDir verifies an empty dir falls back to
// "./venues" — a missing tenant then yields a not-found error rather than a panic.
func TestNewConfigLoader_DefaultsVenuesDir(t *testing.T) {
	cl := config.NewConfigLoader("")
	if _, err := cl.LoadConfig("definitely-not-a-real-tenant"); err == nil {
		t.Fatal("LoadConfig on default dir for unknown tenant: expected error, got nil")
	}
}

// TestLoadConfig_Success verifies a well-formed config.json is parsed and returned.
func TestLoadConfig_Success(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "ace-commodities", `{
		"tenant_id": "ace-commodities",
		"name": "ACE Commodity Exchange",
		"features": {"short_selling": false, "auctions": true}
	}`)

	cl := config.NewConfigLoader(dir)
	cfg, err := cl.LoadConfig("ace-commodities")
	if err != nil {
		t.Fatalf("LoadConfig: unexpected error: %v", err)
	}
	if cfg["tenant_id"] != "ace-commodities" {
		t.Errorf("tenant_id = %v, want ace-commodities", cfg["tenant_id"])
	}
	features, ok := cfg["features"].(map[string]interface{})
	if !ok {
		t.Fatalf("features missing or wrong type: %v", cfg["features"])
	}
	if features["auctions"] != true {
		t.Errorf("features.auctions = %v, want true", features["auctions"])
	}
}

// TestLoadConfig_NotFound verifies a missing config file returns a not-found error.
func TestLoadConfig_NotFound(t *testing.T) {
	dir := t.TempDir() // empty
	cl := config.NewConfigLoader(dir)
	if _, err := cl.LoadConfig("ghost"); err == nil {
		t.Fatal("LoadConfig for missing tenant: expected error, got nil")
	}
}

// TestLoadConfig_Malformed verifies invalid JSON returns a parse error.
func TestLoadConfig_Malformed(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "broken", `{ this is not valid json `)

	cl := config.NewConfigLoader(dir)
	if _, err := cl.LoadConfig("broken"); err == nil {
		t.Fatal("LoadConfig on malformed JSON: expected error, got nil")
	}
}
