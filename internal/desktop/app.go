package desktop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails application binding layer.
// It exposes Go backend functionality to the frontend via Wails bindings.
type App struct {
	ctx      context.Context
	database *db.DB
	calc     *calculator.Calculator
	deviceID string
}

// NewApp creates a new App with references to the shared backend services.
func NewApp(database *db.DB, calc *calculator.Calculator, deviceID string) *App {
	return &App{
		database: database,
		calc:     calc,
		deviceID: deviceID,
	}
}

// Startup is called by Wails when the application starts.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// GetContext returns the Wails runtime context.
func (a *App) GetContext() context.Context {
	return a.ctx
}

// --- Stats API (mirrors internal/web/handler.go) ---

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

type DeviceInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type StatsResponse struct {
	Periods  []PeriodCost     `json:"periods"`
	Sources  []SourceStats    `json:"sources"`
	Devices  []DeviceInfo     `json:"devices"`
	Projects []db.ProjectStat `json:"projects"`
}

// GetStats returns the full dashboard statistics, optionally filtered by device.
func (a *App) GetStats(deviceID string) (*StatsResponse, error) {
	now := time.Now().UTC()

	if deviceID == "all" {
		deviceID = ""
	}

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
		cost, inTok, caTok, caWTok, outTok, _ := a.database.QueryPeriodStatsSince(now.Add(-win.dur), deviceID)
		periods = append(periods, PeriodCost{Label: win.label, Cost: cost, InputTokens: inTok, CachedTokens: caTok, CacheCreationTokens: caWTok, OutputTokens: outTok})
	}
	total, tIn, tCa, tCaW, tOut, _ := a.database.QueryPeriodStatsAll(deviceID)
	periods = append(periods, PeriodCost{Label: "ALL", Cost: total, InputTokens: tIn, CachedTokens: tCa, CacheCreationTokens: tCaW, OutputTokens: tOut})

	stats, _ := a.database.QueryStatsSince(time.Time{}, deviceID)

	sourceMap := make(map[string]*SourceStats)
	for _, s := range stats {
		src, ok := sourceMap[s.Source]
		if !ok {
			src = &SourceStats{Name: s.Source}
			sourceMap[s.Source] = src
		}
		price, _ := a.calc.GetModelPrice(s.Model)
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

	devices, _ := a.database.QueryDevices()
	aliases, _ := a.database.GetDeviceAliases()

	var deviceInfos []DeviceInfo
	for _, id := range devices {
		name := id
		if alias, ok := aliases[id]; ok && alias != "" {
			name = alias
		}
		deviceInfos = append(deviceInfos, DeviceInfo{ID: id, DisplayName: name})
	}

	projects, _ := a.database.QueryProjectStatsSince(time.Time{}, deviceID)

	return &StatsResponse{
		Periods:  periods,
		Sources:  sources,
		Devices:  deviceInfos,
		Projects: projects,
	}, nil
}

// --- Cache Savings ---

type CacheSavingsResponse struct {
	ActualCost       float64 `json:"actual_cost"`
	HypotheticalCost float64 `json:"hypothetical_cost"`
	Saved            float64 `json:"saved"`
	SavedPercent     float64 `json:"saved_percent"`
	CacheHitRate     float64 `json:"cache_hit_rate"`
}

// GetCacheSavings returns cache savings analysis.
func (a *App) GetCacheSavings(deviceID string) (*CacheSavingsResponse, error) {
	if deviceID == "all" {
		deviceID = ""
	}

	records, err := a.database.QueryUsageRecords(time.Time{}, deviceID)
	if err != nil {
		return nil, err
	}

	var actualTotal, hypoTotal float64
	var totalInput, totalCached int

	for _, rec := range records {
		actual, _ := a.calc.CalculateCost(rec.Model, rec.InputTokens, rec.CachedTokens, rec.CacheCreationTokens, rec.OutputTokens)
		hypo, _ := a.calc.CalculateCostNoCaching(rec.Model, rec.InputTokens, rec.CachedTokens, rec.CacheCreationTokens, rec.OutputTokens)
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

	return &CacheSavingsResponse{
		ActualCost:       actualTotal,
		HypotheticalCost: hypoTotal,
		Saved:            saved,
		SavedPercent:     savedPct,
		CacheHitRate:     hitRate,
	}, nil
}

// --- Device Alias ---

// SetDeviceAlias sets a display name for a device.
func (a *App) SetDeviceAlias(deviceID, displayName string) error {
	return a.database.SetDeviceAlias(deviceID, displayName)
}

// --- Pricing Management (Phase 2) ---

type PricingEntry struct {
	Model          string  `json:"model"`
	InputPricePerM float64 `json:"input_price_per_m"`
	CachedPricePerM float64 `json:"cached_price_per_m"`
	OutputPricePerM float64 `json:"output_price_per_m"`
}

// GetPricing returns the current pricing table.
func (a *App) GetPricing() ([]PricingEntry, error) {
	models := a.calc.ListModels()
	var entries []PricingEntry
	for _, m := range models {
		price, _ := a.calc.GetModelPrice(m)
		entries = append(entries, PricingEntry{
			Model:          m,
			InputPricePerM: price.InputPricePerM,
			CachedPricePerM: price.CachedPricePerM,
			OutputPricePerM: price.OutputPricePerM,
		})
	}
	return entries, nil
}

// --- Window Controls ---

// MinimiseToTray hides the window (tray keeps running).
func (a *App) MinimiseToTray() {
	runtime.WindowHide(a.ctx)
}

// ShowWindow restores and focuses the window.
func (a *App) ShowWindow() {
	runtime.WindowShow(a.ctx)
	runtime.WindowSetAlwaysOnTop(a.ctx, false)
}

// --- Config ---

type AppConfig struct {
	AutoStart     bool     `json:"auto_start"`
	ExtraWatchDirs []string `json:"extra_watch_dirs"`
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ai-flight-dashboard", "config.json")
}

// GetConfig returns the current app configuration.
func (a *App) GetConfig() (*AppConfig, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return &AppConfig{}, nil // Return defaults if no config file
	}
	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &AppConfig{}, nil
	}
	return &cfg, nil
}

// SaveConfig persists the app configuration.
func (a *App) SaveConfig(cfg *AppConfig) error {
	dir := filepath.Dir(configPath())
	os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0644)
}
