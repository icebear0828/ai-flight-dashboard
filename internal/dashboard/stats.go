package dashboard

import (
	"sort"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
)

// BuildStats constructs the dashboard stats response used by both the HTTP API
// and the Wails desktop binding.
func BuildStats(database *db.DB, calc *calculator.Calculator, deviceID string, source string, isPaused bool) (*model.StatsResponse, error) {
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

	periods := make([]model.PeriodCost, 0, len(windows)+1)
	for _, win := range windows {
		cost, inTok, caTok, caWTok, outTok, err := database.QueryPeriodStatsSince(now.Add(-win.dur), deviceID, source)
		if err != nil {
			return nil, err
		}
		periods = append(periods, model.PeriodCost{Label: win.label, Cost: cost, InputTokens: inTok, CachedTokens: caTok, CacheCreationTokens: caWTok, OutputTokens: outTok})
	}
	total, tIn, tCa, tCaW, tOut, err := database.QueryPeriodStatsAll(deviceID, source)
	if err != nil {
		return nil, err
	}
	periods = append(periods, model.PeriodCost{Label: "ALL", Cost: total, InputTokens: tIn, CachedTokens: tCa, CacheCreationTokens: tCaW, OutputTokens: tOut})

	stats, err := database.QueryStatsSince(time.Time{}, deviceID, source)
	if err != nil {
		return nil, err
	}

	sourceMap := make(map[string]*model.SourceStats)
	for _, s := range stats {
		src, ok := sourceMap[s.Source]
		if !ok {
			src = &model.SourceStats{Name: s.Source}
			sourceMap[s.Source] = src
		}
		price, _ := calc.GetModelPrice(s.Model)
		src.Models = append(src.Models, model.ModelStats{
			Model:                  s.Model,
			Events:                 s.Events,
			InputTokens:            s.InputTokens,
			CachedTokens:           s.CachedTokens,
			CacheCreationTokens:    s.CacheCreationTokens,
			OutputTokens:           s.OutputTokens,
			TotalCost:              s.TotalCost,
			InputPricePerM:         price.InputPricePerM,
			CachedPricePerM:        price.CachedPricePerM,
			CacheCreationPricePerM: price.CacheCreationPricePerM,
			OutputPricePerM:        price.OutputPricePerM,
		})
		src.TotalInput += s.InputTokens
		src.TotalCached += s.CachedTokens
		src.TotalCacheCreation += s.CacheCreationTokens
		src.TotalOutput += s.OutputTokens
		src.TotalCost += s.TotalCost
		src.TotalEvents += s.Events
	}

	sources := make([]model.SourceStats, 0, len(sourceMap))
	for _, s := range sourceMap {
		sources = append(sources, *s)
	}
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Name < sources[j].Name
	})

	devices, err := database.QueryDevices()
	if err != nil {
		return nil, err
	}
	aliases, err := database.GetDeviceAliases()
	if err != nil {
		return nil, err
	}

	deviceInfos := make([]model.DeviceInfo, 0, len(devices))
	for _, id := range devices {
		name := id
		if alias, ok := aliases[id]; ok && alias != "" {
			name = alias
		}
		deviceInfos = append(deviceInfos, model.DeviceInfo{ID: id, DisplayName: name})
	}

	projects, err := database.QueryProjectStatsSince(time.Time{}, deviceID, source)
	if err != nil {
		return nil, err
	}

	return &model.StatsResponse{
		Periods:  periods,
		Sources:  sources,
		Devices:  deviceInfos,
		Projects: projects,
		IsPaused: isPaused,
	}, nil
}

// BuildTokenSummary constructs the lightweight aggregate advertised to LAN
// peers during discovery.
func BuildTokenSummary(database *db.DB, deviceID string) (model.TokenSummary, error) {
	if deviceID == "all" {
		deviceID = ""
	}

	now := time.Now().UTC()
	_, in24h, _, _, out24h, err := database.QueryPeriodStatsSince(now.Add(-24*time.Hour), deviceID, "")
	if err != nil {
		return model.TokenSummary{}, err
	}
	totalCost, totalIn, _, _, totalOut, err := database.QueryPeriodStatsAll(deviceID, "")
	if err != nil {
		return model.TokenSummary{}, err
	}

	stats, err := database.QueryStatsSince(time.Time{}, deviceID, "")
	if err != nil {
		return model.TokenSummary{}, err
	}
	sourceSet := make(map[string]bool)
	for _, stat := range stats {
		sourceSet[stat.Source] = true
	}

	sourceNames := make([]string, 0, len(sourceSet))
	for source := range sourceSet {
		sourceNames = append(sourceNames, source)
	}
	sort.Strings(sourceNames)

	sources := make([]model.TokenSourceSummary, 0, len(sourceNames))
	for _, source := range sourceNames {
		_, sourceIn24h, _, _, sourceOut24h, err := database.QueryPeriodStatsSince(now.Add(-24*time.Hour), deviceID, source)
		if err != nil {
			return model.TokenSummary{}, err
		}
		sourceCost, sourceIn, _, _, sourceOut, err := database.QueryPeriodStatsAll(deviceID, source)
		if err != nil {
			return model.TokenSummary{}, err
		}
		sources = append(sources, model.TokenSourceSummary{
			Source:      source,
			Tokens24h:   sourceIn24h + sourceOut24h,
			TokensTotal: sourceIn + sourceOut,
			CostTotal:   sourceCost,
		})
	}

	return model.TokenSummary{
		Tokens24h:   in24h + out24h,
		TokensTotal: totalIn + totalOut,
		CostTotal:   totalCost,
		Sources:     sources,
	}, nil
}
