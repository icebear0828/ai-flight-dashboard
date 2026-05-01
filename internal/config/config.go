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

func GetConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ai-flight-dashboard", "config.json")
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
