package model

// --- Shared types used by both web/handler.go and desktop/app.go ---

// ModelStats represents per-model aggregated usage and cost.
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

// SourceStats represents per-source (e.g., "Claude Code") aggregated usage.
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

// PeriodCost represents token usage and cost over a time period.
type PeriodCost struct {
	Label               string  `json:"label"`
	Cost                float64 `json:"cost"`
	InputTokens         int     `json:"input_tokens"`
	CachedTokens        int     `json:"cached_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	OutputTokens        int     `json:"output_tokens"`
}

// DeviceInfo represents a device with its display name.
type DeviceInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// ProjectStat represents aggregated token usage and cost for a project.
type ProjectStat struct {
	Project             string  `json:"project"`
	Events              int     `json:"events"`
	InputTokens         int     `json:"input_tokens"`
	CachedTokens        int     `json:"cached_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	TotalCost           float64 `json:"total_cost"`
}

// StatsResponse is the full stats API response.
type StatsResponse struct {
	Periods  []PeriodCost   `json:"periods"`
	Sources  []SourceStats  `json:"sources"`
	Devices  []DeviceInfo   `json:"devices"`
	Projects []ProjectStat  `json:"projects"`
	IsPaused bool           `json:"is_paused"`
}

// CacheSavingsResponse is the cache savings analysis response.
type CacheSavingsResponse struct {
	ActualCost       float64 `json:"actual_cost"`
	HypotheticalCost float64 `json:"hypothetical_cost"`
	Saved            float64 `json:"saved"`
	SavedPercent     float64 `json:"saved_percent"`
	CacheHitRate     float64 `json:"cache_hit_rate"`
}
