package web

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"sort"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
)

type ModelStats struct {
	Model               string  `json:"model"`
	Events              int     `json:"events"`
	InputTokens         int     `json:"input_tokens"`
	CachedTokens        int     `json:"cached_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	TotalCost           float64 `json:"total_cost"`
	InputPricePerM      float64 `json:"input_price_per_m"`
	CachedPricePerM     float64 `json:"cached_price_per_m"`
	OutputPricePerM     float64 `json:"output_price_per_m"`
}

type SourceStats struct {
	Name               string       `json:"name"`
	TotalInput         int          `json:"total_input"`
	TotalCached        int          `json:"total_cached"`
	TotalCacheCreation int          `json:"total_cache_creation"`
	TotalOutput        int          `json:"total_output"`
	TotalCost          float64      `json:"total_cost"`
	TotalEvents        int          `json:"total_events"`
	Models             []ModelStats `json:"models"`
}

type PeriodCost struct {
	Label               string  `json:"label"`
	Cost                float64 `json:"cost"`
	InputTokens         int     `json:"input_tokens"`
	CachedTokens        int     `json:"cached_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	OutputTokens        int     `json:"output_tokens"`
}

type StatsResponse struct {
	Periods []PeriodCost  `json:"periods"`
	Sources []SourceStats `json:"sources"`
	Devices []string      `json:"devices"`
}

type CacheSavingsResponse struct {
	ActualCost       float64 `json:"actual_cost"`
	HypotheticalCost float64 `json:"hypothetical_cost"`
	Saved            float64 `json:"saved"`
	SavedPercent     float64 `json:"saved_percent"`
	CacheHitRate     float64 `json:"cache_hit_rate"`
}

func NewHandler(database *db.DB, calc *calculator.Calculator, token string) http.Handler {
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

	var periods []PeriodCost
	for _, win := range windows {
		cost, inTok, caTok, caWTok, outTok, _ := database.QueryPeriodStatsSince(now.Add(-win.dur), deviceID)
		periods = append(periods, PeriodCost{Label: win.label, Cost: cost, InputTokens: inTok, CachedTokens: caTok, CacheCreationTokens: caWTok, OutputTokens: outTok})
	}
	total, tIn, tCa, tCaW, tOut, _ := database.QueryPeriodStatsAll(deviceID)
	periods = append(periods, PeriodCost{Label: "ALL", Cost: total, InputTokens: tIn, CachedTokens: tCa, CacheCreationTokens: tCaW, OutputTokens: tOut})

	// Get all-time stats grouped by model
	stats, _ := database.QueryStatsSince(time.Time{}, deviceID)

	// Group by source
	sourceMap := make(map[string]*SourceStats)
	for _, s := range stats {
		src, ok := sourceMap[s.Source]
		if !ok {
			src = &SourceStats{Name: s.Source}
			sourceMap[s.Source] = src
		}
		price, _ := calc.GetModelPrice(s.Model)
		src.Models = append(src.Models, ModelStats{
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

	var sources []SourceStats
	for _, s := range sourceMap {
		sources = append(sources, *s)
	}

	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Name < sources[j].Name
	})

	devices, _ := database.QueryDevices()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(StatsResponse{
		Periods: periods,
		Sources: sources,
		Devices: devices,
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
	json.NewEncoder(w).Encode(CacheSavingsResponse{
		ActualCost:       actualTotal,
		HypotheticalCost: hypoTotal,
		Saved:            saved,
		SavedPercent:     savedPct,
		CacheHitRate:     hitRate,
	})
}
