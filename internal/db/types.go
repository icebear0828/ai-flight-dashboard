package db

import (
	"time"
)

type ModelStat struct {
	Model               string
	Source              string
	Events              int
	InputTokens         int
	CachedTokens        int
	CacheCreationTokens int
	OutputTokens        int
	TotalCost           float64
}
type SourceTotalStat struct {
	Source              string
	Events              int
	InputTokens         int
	CachedTokens        int
	CacheCreationTokens int
	OutputTokens        int
	TotalCost           float64
}

type SourceCoverageStat struct {
	Source    string
	Records   int
	TotalCost float64
	LastSeen  time.Time
}

type PeriodStatsWindow struct {
	Label string
	Since time.Time
}
type PeriodStatsBucket struct {
	Label               string
	Cost                float64
	InputTokens         int
	CachedTokens        int
	CacheCreationTokens int
	OutputTokens        int
}

// UsageRecord represents a single stored usage record for per-row analysis.
type UsageRecord struct {
	Model               string
	InputTokens         int
	CachedTokens        int
	CacheCreationTokens int
	OutputTokens        int
	CostUSD             float64
}

// QueryUsageRecords returns individual usage records since the given time.
