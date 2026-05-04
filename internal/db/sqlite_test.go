package db_test

import (
	"path/filepath"
	"testing"
	"time"

	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
)

func TestDBInsert(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	u := model.TokenUsage{
		Source:       "Test",
		Model:        "test-model",
		InputTokens:  100,
		CachedTokens: 20,
		OutputTokens: 50,
	}

	err = database.InsertUsage(u, 1.25, "test-device")
	if err != nil {
		t.Errorf("failed to insert usage: %v", err)
	}
}

func TestScanOffsets(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Default offset should be 0
	offset, err := database.GetOffset("/some/file.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if offset != 0 {
		t.Errorf("expected 0, got %d", offset)
	}

	// Set and read back
	err = database.SetOffset("/some/file.jsonl", 12345)
	if err != nil {
		t.Fatal(err)
	}
	offset, err = database.GetOffset("/some/file.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if offset != 12345 {
		t.Errorf("expected 12345, got %d", offset)
	}

	// Upsert (update existing)
	err = database.SetOffset("/some/file.jsonl", 99999)
	if err != nil {
		t.Fatal(err)
	}
	offset, _ = database.GetOffset("/some/file.jsonl")
	if offset != 99999 {
		t.Errorf("expected 99999, got %d", offset)
	}
}

func TestInsertUsageWithTime(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ts := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	u := model.TokenUsage{
		Source:       "Claude Code",
		Model:        "claude-opus-4-7",
		InputTokens:  1000,
		CachedTokens: 500,
		OutputTokens: 200,
	}

	err = database.InsertUsageWithTime(u, 0.50, ts, "/path/to/file.jsonl", "my-mac")
	if err != nil {
		t.Fatal(err)
	}

	// Verify it's queryable
	cost, _, _, _, _, err := database.QueryPeriodStatsAll("", "")
	if err != nil {
		t.Fatal(err)
	}
	if cost < 0.49 || cost > 0.51 {
		t.Errorf("expected ~0.50, got %f", cost)
	}
}

func TestQueryCostSince(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()

	// Insert old record (2 days ago)
	old := model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50}
	database.InsertUsageWithTime(old, 1.00, now.Add(-48*time.Hour), "/a.jsonl", "dev1")

	// Insert recent record (30 min ago)
	recent := model.TokenUsage{Source: "Claude Code", Model: "m2", InputTokens: 200, OutputTokens: 100}
	database.InsertUsageWithTime(recent, 2.00, now.Add(-30*time.Minute), "/b.jsonl", "dev1")

	// Insert very recent (5 min ago) on different device
	veryRecent := model.TokenUsage{Source: "Gemini CLI", Model: "m3", InputTokens: 300, OutputTokens: 150}
	database.InsertUsageWithTime(veryRecent, 3.00, now.Add(-5*time.Minute), "/c.jsonl", "dev2")

	// Last 1 hour, all devices = 5.00
	cost, _, _, _, _, _ := database.QueryPeriodStatsSince(now.Add(-1*time.Hour), "", "")
	if cost < 4.99 || cost > 5.01 {
		t.Errorf("last 1h all: expected ~5.00, got %f", cost)
	}

	// Last 1 hour, dev1 only = 2.00
	cost, _, _, _, _, _ = database.QueryPeriodStatsSince(now.Add(-1*time.Hour), "dev1", "")
	if cost < 1.99 || cost > 2.01 {
		t.Errorf("last 1h dev1: expected ~2.00, got %f", cost)
	}

	// Cumulative all = 6.00
	cost, _, _, _, _, _ = database.QueryPeriodStatsAll("", "")
	if cost < 5.99 || cost > 6.01 {
		t.Errorf("cumulative: expected ~6.00, got %f", cost)
	}

	// Cumulative dev2 = 3.00
	cost, _, _, _, _, _ = database.QueryPeriodStatsAll("dev2", "")
	if cost < 2.99 || cost > 3.01 {
		t.Errorf("cumulative dev2: expected ~3.00, got %f", cost)
	}
}

func TestQueryStatsSince(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()

	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 500, OutputTokens: 200},
		1.50, now.Add(-10*time.Minute), "/a.jsonl", "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 2000, CachedTokens: 1000, OutputTokens: 400},
		3.00, now.Add(-5*time.Minute), "/b.jsonl", "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 500, CachedTokens: 0, OutputTokens: 100},
		0.80, now.Add(-1*time.Minute), "/c.jsonl", "local",
	)

	stats, err := database.QueryStatsSince(now.Add(-1*time.Hour), "")
	if err != nil {
		t.Fatal(err)
	}

	if len(stats) != 2 {
		t.Fatalf("expected 2 model groups, got %d", len(stats))
	}

	// Stats should be sorted by cost descending
	if stats[0].Model != "claude-opus-4-7" {
		t.Errorf("expected first model claude-opus-4-7, got %s", stats[0].Model)
	}
	if stats[0].TotalCost < 4.49 || stats[0].TotalCost > 4.51 {
		t.Errorf("expected claude cost ~4.50, got %f", stats[0].TotalCost)
	}
	if stats[0].Events != 2 {
		t.Errorf("expected 2 events, got %d", stats[0].Events)
	}
}

func TestQueryDevices(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50},
		1.00, now, "/a.jsonl", "mac-pro",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50},
		1.00, now, "/b.jsonl", "linux-server",
	)

	devices, err := database.QueryDevices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
}

func TestInsertDedup(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	u := model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 500, OutputTokens: 200, UUID: "test-uuid-123"}

	// Insert same record twice — second should UPSERT by UUID
	err = database.InsertUsageWithTime(u, 1.50, now, "/a.jsonl", "local")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate streaming update: output tokens increased, but same UUID
	u2 := u
	u2.OutputTokens = 300
	err = database.InsertUsageWithTime(u2, 2.00, now, "/a.jsonl", "local")
	if err != nil {
		t.Fatal(err)
	}

	// Should only have 1 record, not 2
	cost, _, _, _, _, _ := database.QueryPeriodStatsAll("", "")
	if cost < 1.99 || cost > 2.01 {
		t.Errorf("expected ~2.00 (upserted record), got %f (dedup failed)", cost)
	}
}

func TestInsertDedup_DifferentDevices(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	u := model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50}

	// Same usage but different devices — should both be kept
	database.InsertUsageWithTime(u, 1.00, now, "/a.jsonl", "mac")
	database.InsertUsageWithTime(u, 1.00, now, "/a.jsonl", "linux")

	cost, _, _, _, _, _ := database.QueryPeriodStatsAll("", "")
	if cost < 1.99 || cost > 2.01 {
		t.Errorf("expected ~2.00 (2 different devices), got %f", cost)
	}
}

func TestQueryCacheSavings(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()

	// Record 1: 1000 input (800 cached), 200 output
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 800, OutputTokens: 200},
		1.50, now.Add(-10*time.Minute), "/a.jsonl", "local",
	)
	// Record 2: 500 input (0 cached), 100 output
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 500, CachedTokens: 0, OutputTokens: 100},
		0.80, now.Add(-5*time.Minute), "/b.jsonl", "local",
	)

	records, err := database.QueryUsageRecords(now.Add(-1*time.Hour), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	// Verify fields
	totalCached := 0
	totalInput := 0
	for _, r := range records {
		totalCached += r.CachedTokens
		totalInput += r.InputTokens
	}
	if totalCached != 800 {
		t.Errorf("expected total cached 800, got %d", totalCached)
	}
	if totalInput != 1500 {
		t.Errorf("expected total input 1500, got %d", totalInput)
	}
}

func TestDeduplicateExisting(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	u := model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, CachedTokens: 0, OutputTokens: 50}

	// Simulate legacy data: drop the unique index, force-insert 3 identical rows, then re-add it.
	database.RawExec("DROP INDEX IF EXISTS idx_usage_dedup")
	for i := 0; i < 3; i++ {
		database.RawExec(
			"INSERT INTO usage_records (log_timestamp, source, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			now.Format(time.RFC3339), u.Source, u.Model, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens, 1.00, "/a.jsonl", "local",
		)
	}

	// Verify we have 3 records before dedup
	cost, _, _, _, _, _ := database.QueryPeriodStatsAll("", "")
	if cost < 2.99 {
		t.Fatalf("setup: expected ~3.00 before dedup, got %f", cost)
	}

	// Run dedup
	removed, err := database.DeduplicateExisting()
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Errorf("expected 2 duplicates removed, got %d", removed)
	}

	// After dedup, should be exactly 1 record
	cost, _, _, _, _, _ = database.QueryPeriodStatsAll("", "")
	if cost < 0.99 || cost > 1.01 {
		t.Errorf("expected ~1.00 after dedup, got %f", cost)
	}
}

