package web

import (
	"encoding/json"
	"net/http"
	"os"

	"ai-flight-dashboard/internal/config"
	"ai-flight-dashboard/internal/watcher"
)

func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig()
	if err != nil {
		http.Error(w, "Failed to load config", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

func handlePutConfig(w http.ResponseWriter, r *http.Request, wInst *watcher.Watcher) {
	var cfg config.AppConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	oldCfg, _ := config.LoadConfig()

	if err := config.SaveConfig(&cfg); err != nil {
		http.Error(w, "Failed to save config", http.StatusInternalServerError)
		return
	}

	if wInst != nil {
		if oldCfg != nil {
			for _, oldDir := range oldCfg.ExtraWatchDirs {
				found := false
				for _, newDir := range cfg.ExtraWatchDirs {
					if oldDir == newDir {
						found = true
						break
					}
				}
				if !found {
					wInst.UnwatchDir(oldDir)
				}
			}
		}

		for _, dir := range cfg.ExtraWatchDirs {
			if _, err := os.Stat(dir); err == nil {
				wInst.WatchDirRecursive(dir)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}
