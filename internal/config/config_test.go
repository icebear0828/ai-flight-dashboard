package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"ai-flight-dashboard/internal/config"
)

func TestLoadConfig_FileNotExist(t *testing.T) {
	// Use a temp dir so we don't read any real config
	tmpDir := t.TempDir()
	config.SetDataDir(tmpDir)
	defer config.SetDataDir("")

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig should not error when file missing, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadConfig returned nil config")
	}
	if cfg.AutoStart {
		t.Error("default AutoStart should be false")
	}
	if len(cfg.ExtraWatchDirs) != 0 {
		t.Errorf("default ExtraWatchDirs should be empty, got %v", cfg.ExtraWatchDirs)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	config.SetDataDir(tmpDir)
	defer config.SetDataDir("")

	cfg := &config.AppConfig{
		AutoStart:      true,
		ExtraWatchDirs: []string{"/tmp/logs", "/var/data"},
	}

	err := config.SaveConfig(cfg)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	configPath := filepath.Join(tmpDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

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
	config.SetDataDir(tmpDir)
	defer config.SetDataDir("")

	os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(`{invalid json`), 0644)

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig should not error on bad JSON, got: %v", err)
	}
	if cfg.AutoStart {
		t.Error("malformed JSON should result in default (false) AutoStart")
	}
}

func TestGetConfigPath(t *testing.T) {
	// Default (no SetDataDir): current directory
	config.SetDataDir("")
	path := config.GetConfigPath()
	if filepath.Base(path) != "config.json" {
		t.Errorf("expected config.json, got %s", filepath.Base(path))
	}

	// Custom dir
	config.SetDataDir("/custom/path")
	path = config.GetConfigPath()
	if path != filepath.Join("/custom/path", "config.json") {
		t.Errorf("expected /custom/path/config.json, got %s", path)
	}
	config.SetDataDir("")
}

func TestGetDataDir_Default(t *testing.T) {
	config.SetDataDir("")
	dir := config.GetDataDir()
	if dir != "." {
		t.Errorf("expected current directory '.', got %s", dir)
	}
}

func TestGetDataDir_Custom(t *testing.T) {
	config.SetDataDir("/my/data")
	defer config.SetDataDir("")
	dir := config.GetDataDir()
	if dir != "/my/data" {
		t.Errorf("expected /my/data, got %s", dir)
	}
}
