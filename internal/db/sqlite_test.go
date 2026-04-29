package db_test

import (
	"path/filepath"
	"testing"
	"time"

	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/watcher"
)

func TestDBInsert(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	u := watcher.TokenUsage{
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
	u := watcher.TokenUsage{
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
	cost, _, _, _, err := database.QueryPeriodStatsAll("")
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
	old := watcher.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50}
	database.InsertUsageWithTime(old, 1.00, now.Add(-48*time.Hour), "/a.jsonl", "dev1")

	// Insert recent record (30 min ago)
	recent := watcher.TokenUsage{Source: "Claude Code", Model: "m2", InputTokens: 200, OutputTokens: 100}
	database.InsertUsageWithTime(recent, 2.00, now.Add(-30*time.Minute), "/b.jsonl", "dev1")

	// Insert very recent (5 min ago) on different device
	veryRecent := watcher.TokenUsage{Source: "Gemini CLI", Model: "m3", InputTokens: 300, OutputTokens: 150}
	database.InsertUsageWithTime(veryRecent, 3.00, now.Add(-5*time.Minute), "/c.jsonl", "dev2")

	// Last 1 hour, all devices = 5.00
	cost, _, _, _, _ := database.QueryPeriodStatsSince(now.Add(-1*time.Hour), "")
	if cost < 4.99 || cost > 5.01 {
		t.Errorf("last 1h all: expected ~5.00, got %f", cost)
	}

	// Last 1 hour, dev1 only = 2.00
	cost, _, _, _, _ = database.QueryPeriodStatsSince(now.Add(-1*time.Hour), "dev1")
	if cost < 1.99 || cost > 2.01 {
		t.Errorf("last 1h dev1: expected ~2.00, got %f", cost)
	}

	// Cumulative all = 6.00
	cost, _, _, _, _ = database.QueryPeriodStatsAll("")
	if cost < 5.99 || cost > 6.01 {
		t.Errorf("cumulative: expected ~6.00, got %f", cost)
	}

	// Cumulative dev2 = 3.00
	cost, _, _, _, _ = database.QueryPeriodStatsAll("dev2")
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
		watcher.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 500, OutputTokens: 200},
		1.50, now.Add(-10*time.Minute), "/a.jsonl", "local",
	)
	database.InsertUsageWithTime(
		watcher.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 2000, CachedTokens: 1000, OutputTokens: 400},
		3.00, now.Add(-5*time.Minute), "/b.jsonl", "local",
	)
	database.InsertUsageWithTime(
		watcher.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 500, CachedTokens: 0, OutputTokens: 100},
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
		watcher.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50},
		1.00, now, "/a.jsonl", "mac-pro",
	)
	database.InsertUsageWithTime(
		watcher.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50},
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
