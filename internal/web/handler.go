package web

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"sort"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
)

type ModelStats struct {
	Model       string  `json:"model"`
	Events      int     `json:"events"`
	InputTokens int     `json:"input_tokens"`
	CachedTokens int   `json:"cached_tokens"`
	OutputTokens int    `json:"output_tokens"`
	TotalCost   float64 `json:"total_cost"`
	InputPricePerM  float64 `json:"input_price_per_m"`
	CachedPricePerM float64 `json:"cached_price_per_m"`
	OutputPricePerM float64 `json:"output_price_per_m"`
}

type SourceStats struct {
	Name         string       `json:"name"`
	TotalInput   int          `json:"total_input"`
	TotalCached  int          `json:"total_cached"`
	TotalOutput  int          `json:"total_output"`
	TotalCost    float64      `json:"total_cost"`
	TotalEvents  int          `json:"total_events"`
	Models       []ModelStats `json:"models"`
}

type PeriodCost struct {
	Label        string  `json:"label"`
	Cost         float64 `json:"cost"`
	InputTokens  int     `json:"input_tokens"`
	CachedTokens int     `json:"cached_tokens"`
	OutputTokens int     `json:"output_tokens"`
}

type StatsResponse struct {
	Periods []PeriodCost  `json:"periods"`
	Sources []SourceStats `json:"sources"`
	Devices []string      `json:"devices"`
}

func NewHandler(database *db.DB, calc *calculator.Calculator) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		handleStats(w, r, database, calc)
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

	var periods []PeriodCost
	for _, win := range windows {
		cost, inTok, caTok, outTok, _ := database.QueryPeriodStatsSince(now.Add(-win.dur), deviceID)
		periods = append(periods, PeriodCost{Label: win.label, Cost: cost, InputTokens: inTok, CachedTokens: caTok, OutputTokens: outTok})
	}
	total, tIn, tCa, tOut, _ := database.QueryPeriodStatsAll(deviceID)
	periods = append(periods, PeriodCost{Label: "ALL", Cost: total, InputTokens: tIn, CachedTokens: tCa, OutputTokens: tOut})

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
			Model:           s.Model,
			Events:          s.Events,
			InputTokens:     s.InputTokens,
			CachedTokens:    s.CachedTokens,
			OutputTokens:    s.OutputTokens,
			TotalCost:       s.TotalCost,
			InputPricePerM:  price.InputPricePerM,
			CachedPricePerM: price.CachedPricePerM,
			OutputPricePerM: price.OutputPricePerM,
		})
		src.TotalInput += s.InputTokens
		src.TotalCached += s.CachedTokens
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
