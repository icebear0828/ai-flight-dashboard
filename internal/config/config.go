package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type AppConfig struct {
	AutoStart      bool     `json:"auto_start"`
	ExtraWatchDirs []string `json:"extra_watch_dirs"`
}

// customDir allows overriding the data directory at runtime.
var customDir string

// SetDataDir overrides the default ~/.ai-flight-dashboard data directory.
func SetDataDir(dir string) {
	customDir = dir
}

// GetDataDir returns the application data directory.
func GetDataDir() string {
	if customDir != "" {
		return customDir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ai-flight-dashboard")
}

func GetConfigPath() string {
	return filepath.Join(GetDataDir(), "config.json")
}

func LoadConfig() (*AppConfig, error) {
	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		return &AppConfig{}, nil
	}
	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &AppConfig{}, nil
	}
	return &cfg, nil
}

func SaveConfig(cfg *AppConfig) error {
	path := GetConfigPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
