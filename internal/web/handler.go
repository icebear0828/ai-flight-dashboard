package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/config"
	"ai-flight-dashboard/internal/dashboard"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/lan"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/watcher"
)

const (
	defaultSyncLimit = 1000
	maxSyncLimit     = 5000
)

func authMiddleware(token string, next http.HandlerFunc) http.HandlerFunc {
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

func syncAuthMiddleware(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			if !isPrivateLANRemote(r.RemoteAddr) {
				http.Error(w, "LAN sync token required", http.StatusUnauthorized)
				return
			}
			next(w, r)
			return
		}
		authMiddleware(token, next)(w, r)
	}
}

func isPrivateLANRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	if zoneAt := strings.IndexByte(host, '%'); zoneAt >= 0 {
		host = host[:zoneAt]
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// NewLANHandler exposes only the endpoints needed by LAN peers.
func NewLANHandler(database *db.DB, token string, lanInst *lan.LAN) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/lan/self", func(w http.ResponseWriter, r *http.Request) {
		handleLANSelf(w, r, lanInst)
	})
	mux.HandleFunc("/api/sync/pull", syncAuthMiddleware(token, func(w http.ResponseWriter, r *http.Request) {
		handleSyncPull(w, r, database)
	}))
	return mux
}

func handleLANSelf(w http.ResponseWriter, r *http.Request, lanInst *lan.LAN) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if lanInst == nil {
		http.NotFound(w, r)
		return
	}

	resp := model.LANSelfResponse{
		DeviceID: lanInst.DeviceID,
		HTTPPort: lanInst.HTTPPort,
	}
	if summary, ok := lanInst.CurrentSummary(); ok {
		resp.Summary = &summary
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func currentLAN(lanControl LANController) *lan.LAN {
	if lanControl == nil {
		return nil
	}
	return lanControl.CurrentLAN()
}

func handleSystemLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logPath := filepath.Join(config.GetDataDir(), "stats")
	if err := os.MkdirAll(logPath, 0755); err != nil {
		http.Error(w, "Failed to prepare system logs directory", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(model.SystemLogsResponse{Path: logPath})
}

func NewHandler(database *db.DB, calc *calculator.Calculator, wInst *watcher.Watcher, lanInst *lan.LAN, token string, distBinFS embed.FS) http.Handler {
	return NewHandlerWithLANController(database, calc, wInst, newStaticLANController(lanInst), token, distBinFS)
}

func NewHandlerWithLANController(database *db.DB, calc *calculator.Calculator, wInst *watcher.Watcher, lanControl LANController, token string, distBinFS embed.FS) http.Handler {
	mux := http.NewServeMux()

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
			authMiddleware(token, func(w http.ResponseWriter, r *http.Request) {
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
			authMiddleware(token, func(w http.ResponseWriter, r *http.Request) {
				handlePutConfig(w, r, wInst)
			})(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/track", authMiddleware(token, func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("/api/device-alias", authMiddleware(token, func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("/api/lan/self", func(w http.ResponseWriter, r *http.Request) {
		handleLANSelf(w, r, currentLAN(lanControl))
	})

	mux.HandleFunc("/api/lan/scan", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		peers := make([]string, 0)
		peerInfos := make([]model.LANPeerInfo, 0)
		lanInst := currentLAN(lanControl)
		if lanInst != nil {
			peers = lanInst.GetActivePeers()
			aliases, _ := database.GetDeviceAliases()
			for _, peer := range lanInst.GetActivePeerInfos() {
				summary, err := dashboard.BuildTokenSummary(database, peer.ID)
				if err != nil {
					summary = model.TokenSummary{}
				}
				if peer.HasSummary && (peer.SyncStatus != "ok" || summary.TokensTotal == 0) {
					summary = peer.Summary
				}
				displayName := peer.ID
				if alias := aliases[peer.ID]; alias != "" {
					displayName = alias
				}
				peerInfos = append(peerInfos, model.LANPeerInfo{
					ID:              peer.ID,
					DisplayName:     displayName,
					IP:              peer.IP,
					HTTPPort:        peer.HTTPPort,
					LastSeen:        peer.LastSeen,
					LastSync:        peer.LastSync,
					LastSyncAttempt: peer.LastSyncAttempt,
					SyncStatus:      peer.SyncStatus,
					SyncError:       peer.SyncError,
					Tokens24h:       summary.Tokens24h,
					TokensTotal:     summary.TokensTotal,
					CostTotal:       summary.CostTotal,
					Sources:         summary.Sources,
				})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(model.LANScanResponse{Peers: peers, PeerInfos: peerInfos})
	})

	mux.HandleFunc("/api/lan/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		status := model.LANStatusResponse{Enabled: false}
		if lanControl != nil {
			status = lanControl.Status()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	})

	mux.HandleFunc("/api/lan/join", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		status := model.LANStatusResponse{Enabled: false}
		var err error
		if lanControl != nil {
			status, err = lanControl.Join()
		}
		if err != nil {
			http.Error(w, "Failed to join LAN", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	})

	mux.HandleFunc("/api/lan/leave", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		status := model.LANStatusResponse{Enabled: false}
		var err error
		if lanControl != nil {
			status, err = lanControl.Leave()
		}
		if err != nil {
			http.Error(w, "Failed to leave LAN", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	})

	mux.HandleFunc("/api/system/logs", handleSystemLogs)

	mux.HandleFunc("/api/sync/pull", syncAuthMiddleware(token, func(w http.ResponseWriter, r *http.Request) {
		handleSyncPull(w, r, database)
	}))

	mux.HandleFunc("/api/pause", authMiddleware(token, func(w http.ResponseWriter, r *http.Request) {
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

func handleSyncPull(w http.ResponseWriter, r *http.Request, database *db.DB) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sinceStr := r.URL.Query().Get("since")
	var since time.Time
	if sinceStr != "" {
		t, err := time.Parse(time.RFC3339Nano, sinceStr)
		if err != nil {
			http.Error(w, "Invalid since format, expected RFC3339", http.StatusBadRequest)
			return
		}
		since = t
	}

	var afterID int64
	if afterStr := r.URL.Query().Get("after_id"); afterStr != "" {
		id, err := strconv.ParseInt(afterStr, 10, 64)
		if err != nil || id < 0 {
			http.Error(w, "Invalid after_id", http.StatusBadRequest)
			return
		}
		afterID = id
	}

	limit := defaultSyncLimit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed <= 0 {
			http.Error(w, "Invalid limit", http.StatusBadRequest)
			return
		}
		limit = parsed
	}
	if limit > maxSyncLimit {
		limit = maxSyncLimit
	}

	deviceID := r.URL.Query().Get("device_id")
	page, err := database.QuerySyncRecordsPageForDevice(since, afterID, limit, deviceID)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}

func handleStats(w http.ResponseWriter, r *http.Request, database *db.DB, calc *calculator.Calculator, wInst *watcher.Watcher) {
	deviceID := r.URL.Query().Get("device")
	source := r.URL.Query().Get("source") // "Claude Code", "Gemini CLI", or "" for all

	isPaused := false
	if wInst != nil {
		isPaused = wInst.IsPaused()
	}

	stats, err := dashboard.BuildStats(database, calc, deviceID, source, isPaused)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
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
	hitRate = model.CacheHitRatePercent(totalInput, totalCached)

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
		if e.InputPricePerM < 0 {
			e.InputPricePerM = 0
		}
		if e.CachedPricePerM < 0 {
			e.CachedPricePerM = 0
		}
		if e.CacheCreationPricePerM < 0 {
			e.CacheCreationPricePerM = 0
		}
		if e.OutputPricePerM < 0 {
			e.OutputPricePerM = 0
		}

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
