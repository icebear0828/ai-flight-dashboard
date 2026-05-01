package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"sort"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
)

func NewHandler(database *db.DB, calc *calculator.Calculator, token string, distBinFS embed.FS) http.Handler {
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
		handleStats(w, r, database, calc)
	})

	mux.HandleFunc("/api/cache-savings", func(w http.ResponseWriter, r *http.Request) {
		handleCacheSavings(w, r, database, calc)
	})

	mux.HandleFunc("/api/track", authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
			host = "localhost:9100"
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

func handleStats(w http.ResponseWriter, r *http.Request, database *db.DB, calc *calculator.Calculator) {
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(model.StatsResponse{
		Periods:  periods,
		Sources:  sources,
		Devices:  deviceInfos,
		Projects: projects,
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
