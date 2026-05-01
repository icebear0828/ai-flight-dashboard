package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/config"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/lan"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/watcher"
)

func NewHandler(database *db.DB, calc *calculator.Calculator, wInst *watcher.Watcher, lanInst *lan.LAN, token string, distBinFS embed.FS) http.Handler {
	mux := http.NewServeMux()

	authMiddleware := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if token != "" {
				authHeader := r.Header.Get("Authorization")
				if authHeader != "Bearer "+token {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
			}
			next(w, r)
		}
	}

	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		handleStats(w, r, database, calc, wInst)
	})

	mux.HandleFunc("/api/cache-savings", func(w http.ResponseWriter, r *http.Request) {
		handleCacheSavings(w, r, database, calc)
	})

	mux.HandleFunc("/api/pricing", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handleGetPricing(w, r, calc)
		} else if r.Method == http.MethodPut || r.Method == http.MethodPost {
			authMiddleware(func(w http.ResponseWriter, r *http.Request) {
				handlePutPricing(w, r, calc)
			})(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handleGetConfig(w, r)
		} else if r.Method == http.MethodPut {
			authMiddleware(func(w http.ResponseWriter, r *http.Request) {
				handlePutConfig(w, r, wInst)
			})(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/track", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if wInst != nil && wInst.IsPaused() {
			w.WriteHeader(http.StatusOK) // Acknowledge but ignore
			return
		}

		var payload model.TrackPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		cost, err := calc.CalculateCost(payload.Usage.Model, payload.Usage.InputTokens, payload.Usage.CachedTokens, payload.Usage.CacheCreationTokens, payload.Usage.OutputTokens)
		if err != nil {
			// Ignore cost calculation errors (e.g. unknown model) but proceed with inserting usage with 0 cost
			cost = 0
		}
		if err := database.InsertUsage(payload.Usage, cost, payload.DeviceID); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))

	mux.HandleFunc("/api/device-alias", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			DeviceID    string `json:"device_id"`
			DisplayName string `json:"display_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DeviceID == "" {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		if err := database.SetDeviceAlias(req.DeviceID, req.DisplayName); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	mux.HandleFunc("/api/lan/scan", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var peers []string
		if lanInst != nil {
			peers = lanInst.GetActivePeers()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"peers": peers,
		})
	}))

	mux.HandleFunc("/api/lan/join", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Acknowledge join
		w.WriteHeader(http.StatusOK)
	}))

	mux.HandleFunc("/api/pause", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if wInst != nil {
			wInst.SetPaused(!wInst.IsPaused())
		}
		
		isPaused := false
		if wInst != nil {
			isPaused = wInst.IsPaused()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"is_paused": isPaused})
	}))

	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		filename := r.URL.Path[len("/download/"):]
		if filename == "dashboard" || filename == "" {
			exePath, err := os.Executable()
			if err == nil {
				w.Header().Set("Content-Disposition", "attachment; filename=dashboard")
				http.ServeFile(w, r, exePath)
				return
			}
		}

		data, err := distBinFS.ReadFile("dist-bin/" + filename)
		if err != nil {
			exePath, err2 := os.Executable()
			if err2 == nil {
				w.Header().Set("Content-Disposition", "attachment; filename=dashboard")
				http.ServeFile(w, r, exePath)
				return
			}
			http.Error(w, "Binary not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Disposition", "attachment; filename="+filename)
		w.Write(data)
	})

	mux.HandleFunc("/install.sh", func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if host == "" {
			host = "localhost:19100"
		}
		script := fmt.Sprintf("#!/bin/bash\n"+
			"OS=$(uname -s | tr '[:upper:]' '[:lower:]')\n"+
			"ARCH=$(uname -m)\n"+
			"if [ \"$ARCH\" = \"x86_64\" ]; then ARCH=\"amd64\"; fi\n"+
			"if [ \"$ARCH\" = \"aarch64\" ]; then ARCH=\"arm64\"; fi\n"+
			"echo \"📡 Downloading AI Flight Dashboard ($OS-$ARCH) from %s...\"\n"+
			"curl -o dashboard http://%s/download/dashboard-$OS-$ARCH\n"+
			"chmod +x dashboard\n"+
			"echo \"✅ Download complete! Starting LAN mode...\"\n"+
			"./dashboard --lan\n", host, host)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(script))
	})

	// Serve static files from the "dist" directory embedded in the binary
	staticFS, err := fs.Sub(StaticFiles, "dist")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	return mux
}

func handleStats(w http.ResponseWriter, r *http.Request, database *db.DB, calc *calculator.Calculator, wInst *watcher.Watcher) {
	now := time.Now().UTC()
	deviceID := r.URL.Query().Get("device")

	windows := []struct {
		label string
		dur   time.Duration
	}{
		{"1h", 1 * time.Hour},
		{"24h", 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
		{"3mo", 90 * 24 * time.Hour},
		{"6mo", 180 * 24 * time.Hour},
		{"1y", 365 * 24 * time.Hour},
	}

	var periods []model.PeriodCost
	for _, win := range windows {
		cost, inTok, caTok, caWTok, outTok, _ := database.QueryPeriodStatsSince(now.Add(-win.dur), deviceID)
		periods = append(periods, model.PeriodCost{Label: win.label, Cost: cost, InputTokens: inTok, CachedTokens: caTok, CacheCreationTokens: caWTok, OutputTokens: outTok})
	}
	total, tIn, tCa, tCaW, tOut, _ := database.QueryPeriodStatsAll(deviceID)
	periods = append(periods, model.PeriodCost{Label: "ALL", Cost: total, InputTokens: tIn, CachedTokens: tCa, CacheCreationTokens: tCaW, OutputTokens: tOut})

	// Get all-time stats grouped by model
	stats, _ := database.QueryStatsSince(time.Time{}, deviceID)

	// Group by source
	sourceMap := make(map[string]*model.SourceStats)
	for _, s := range stats {
		src, ok := sourceMap[s.Source]
		if !ok {
			src = &model.SourceStats{Name: s.Source}
			sourceMap[s.Source] = src
		}
		price, _ := calc.GetModelPrice(s.Model)
		src.Models = append(src.Models, model.ModelStats{
			Model:               s.Model,
			Events:              s.Events,
			InputTokens:         s.InputTokens,
			CachedTokens:        s.CachedTokens,
			CacheCreationTokens: s.CacheCreationTokens,
			OutputTokens:        s.OutputTokens,
			TotalCost:           s.TotalCost,
			InputPricePerM:      price.InputPricePerM,
			CachedPricePerM:     price.CachedPricePerM,
			OutputPricePerM:     price.OutputPricePerM,
		})
		src.TotalInput += s.InputTokens
		src.TotalCached += s.CachedTokens
		src.TotalCacheCreation += s.CacheCreationTokens
		src.TotalOutput += s.OutputTokens
		src.TotalCost += s.TotalCost
		src.TotalEvents += s.Events
	}

	var sources []model.SourceStats
	for _, s := range sourceMap {
		sources = append(sources, *s)
	}

	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Name < sources[j].Name
	})

	devices, _ := database.QueryDevices()
	aliases, _ := database.GetDeviceAliases()

	var deviceInfos []model.DeviceInfo
	for _, id := range devices {
		name := id
		if alias, ok := aliases[id]; ok && alias != "" {
			name = alias
		}
		deviceInfos = append(deviceInfos, model.DeviceInfo{ID: id, DisplayName: name})
	}

	projects, _ := database.QueryProjectStatsSince(time.Time{}, deviceID)

	isPaused := false
	if wInst != nil {
		isPaused = wInst.IsPaused()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(model.StatsResponse{
		Periods:  periods,
		Sources:  sources,
		Devices:  deviceInfos,
		Projects: projects,
		IsPaused: isPaused,
	})
}

func handleCacheSavings(w http.ResponseWriter, r *http.Request, database *db.DB, calc *calculator.Calculator) {
	deviceID := r.URL.Query().Get("device")

	// Query all records (no time filter = all time)
	records, err := database.QueryUsageRecords(time.Time{}, deviceID)
	if err != nil {
		http.Error(w, "query error", http.StatusInternalServerError)
		return
	}

	var actualTotal, hypoTotal float64
	var totalInput, totalCached int

	for _, rec := range records {
		actual, _ := calc.CalculateCost(rec.Model, rec.InputTokens, rec.CachedTokens, rec.CacheCreationTokens, rec.OutputTokens)
		hypo, _ := calc.CalculateCostNoCaching(rec.Model, rec.InputTokens, rec.CachedTokens, rec.CacheCreationTokens, rec.OutputTokens)
		actualTotal += actual
		hypoTotal += hypo
		totalInput += rec.InputTokens
		totalCached += rec.CachedTokens
	}

	saved := hypoTotal - actualTotal
	savedPct := 0.0
	if hypoTotal > 0 {
		savedPct = (saved / hypoTotal) * 100
	}
	hitRate := 0.0
	if totalInput > 0 {
		hitRate = (float64(totalCached) / float64(totalInput)) * 100
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(model.CacheSavingsResponse{
		ActualCost:       actualTotal,
		HypotheticalCost: hypoTotal,
		Saved:            saved,
		SavedPercent:     savedPct,
		CacheHitRate:     hitRate,
	})
}

type PricingEntry struct {
	Model                  string  `json:"model"`
	InputPricePerM         float64 `json:"input_price_per_m"`
	CachedPricePerM        float64 `json:"cached_price_per_m"`
	CacheCreationPricePerM float64 `json:"cache_creation_price_per_m"`
	OutputPricePerM        float64 `json:"output_price_per_m"`
}

func handleGetPricing(w http.ResponseWriter, r *http.Request, calc *calculator.Calculator) {
	models := calc.ListModels()
	sort.Strings(models)
	var entries []PricingEntry
	for _, m := range models {
		price, _ := calc.GetModelPrice(m)
		entries = append(entries, PricingEntry{
			Model:                  m,
			InputPricePerM:         price.InputPricePerM,
			CachedPricePerM:        price.CachedPricePerM,
			CacheCreationPricePerM: price.CacheCreationPricePerM,
			OutputPricePerM:        price.OutputPricePerM,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func handlePutPricing(w http.ResponseWriter, r *http.Request, calc *calculator.Calculator) {
	var entries []PricingEntry
	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	customPrices := make(map[string]calculator.ModelPrice)
	for _, e := range entries {
		if e.InputPricePerM < 0 { e.InputPricePerM = 0 }
		if e.CachedPricePerM < 0 { e.CachedPricePerM = 0 }
		if e.CacheCreationPricePerM < 0 { e.CacheCreationPricePerM = 0 }
		if e.OutputPricePerM < 0 { e.OutputPricePerM = 0 }

		customPrices[e.Model] = calculator.ModelPrice{
			InputPricePerM:         e.InputPricePerM,
			CachedPricePerM:        e.CachedPricePerM,
			CacheCreationPricePerM: e.CacheCreationPricePerM,
			OutputPricePerM:        e.OutputPricePerM,
		}
	}

	// Persist to ~/.ai-flight-dashboard/custom_pricing.json
	home, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, "Failed to resolve user home directory for persistence", http.StatusInternalServerError)
		return
	}

	configDir := filepath.Join(home, ".ai-flight-dashboard")
	os.MkdirAll(configDir, 0755)
	
	// Load existing first to merge, so we don't lose other custom models
	existingCustomPrices := make(map[string]calculator.ModelPrice)
	customPricingPath := filepath.Join(configDir, "custom_pricing.json")
	if data, err := os.ReadFile(customPricingPath); err == nil {
		if err := json.Unmarshal(data, &existingCustomPrices); err != nil {
			http.Error(w, "Existing custom_pricing.json is corrupted. Refusing to overwrite.", http.StatusInternalServerError)
			return
		}
	}
	
	for k, v := range customPrices {
		existingCustomPrices[k] = v
	}
	
	data, err := json.MarshalIndent(existingCustomPrices, "", "  ")
	if err != nil {
		http.Error(w, "Failed to marshal pricing data", http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(customPricingPath, data, 0644); err != nil {
		http.Error(w, "Failed to write pricing data to disk", http.StatusInternalServerError)
		return
	}

	// Update memory only if persistence succeeds
	calc.UpdatePrices(customPrices)

	w.WriteHeader(http.StatusOK)
}

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

	// Dynamically update watcher
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
