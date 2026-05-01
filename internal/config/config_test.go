package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"ai-flight-dashboard/internal/config"
)

func TestLoadConfig_FileNotExist(t *testing.T) {
	// LoadConfig should return a zero-value AppConfig when the file doesn't exist
	// We can't easily override GetConfigPath, but we can verify the behavior
	// by checking that a non-existent path returns defaults gracefully.
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig should not error when file missing, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadConfig returned nil config")
	}
	// Default: auto_start should be false, no extra dirs
	if cfg.AutoStart {
		t.Error("default AutoStart should be false")
	}
	if len(cfg.ExtraWatchDirs) != 0 {
		t.Errorf("default ExtraWatchDirs should be empty, got %v", cfg.ExtraWatchDirs)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	// Create a temporary config directory
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".ai-flight-dashboard")
	configPath := filepath.Join(configDir, "config.json")

	// Temporarily override HOME to redirect config path
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := &config.AppConfig{
		AutoStart:      true,
		ExtraWatchDirs: []string{"/tmp/logs", "/var/data"},
	}

	err := config.SaveConfig(cfg)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	// Load it back
	loaded, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !loaded.AutoStart {
		t.Error("AutoStart should be true after save/load")
	}
	if len(loaded.ExtraWatchDirs) != 2 {
		t.Errorf("expected 2 ExtraWatchDirs, got %d", len(loaded.ExtraWatchDirs))
	}
	if loaded.ExtraWatchDirs[0] != "/tmp/logs" {
		t.Errorf("expected /tmp/logs, got %s", loaded.ExtraWatchDirs[0])
	}
}

func TestLoadConfig_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	configDir := filepath.Join(tmpDir, ".ai-flight-dashboard")
	os.MkdirAll(configDir, 0755)
	os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{invalid json`), 0644)

	// LoadConfig should gracefully return defaults on malformed JSON
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig should not error on bad JSON, got: %v", err)
	}
	if cfg.AutoStart {
		t.Error("malformed JSON should result in default (false) AutoStart")
	}
}

func TestGetConfigPath(t *testing.T) {
	path := config.GetConfigPath()
	if path == "" {
		t.Fatal("GetConfigPath returned empty string")
	}
	if filepath.Base(path) != "config.json" {
		t.Errorf("expected config.json, got %s", filepath.Base(path))
	}
	if filepath.Base(filepath.Dir(path)) != ".ai-flight-dashboard" {
		t.Errorf("expected .ai-flight-dashboard dir, got %s", filepath.Dir(path))
	}
}
