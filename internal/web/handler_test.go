package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/watcher"
	"ai-flight-dashboard/internal/web"
)

func setup(t *testing.T) (*db.DB, *calculator.Calculator) {
	t.Helper()
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	pricingPath := filepath.Join(t.TempDir(), "p.json")
	os.WriteFile(pricingPath, []byte(`{"models":{"claude-opus-4-7":{"input_price_per_m":15,"cached_price_per_m":1.5,"output_price_per_m":75},"gemini-2.5-pro":{"input_price_per_m":1.25,"cached_price_per_m":0.31,"output_price_per_m":5}}}`), 0644)
	calc, _ := calculator.New(pricingPath)
	return database, calc
}

func TestAPIStats(t *testing.T) {
	database, calc := setup(t)
	defer database.Close()

	now := time.Now().UTC()

	// Claude records
	database.InsertUsageWithTime(
		watcher.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 5000, OutputTokens: 200},
		1.50, now.Add(-30*time.Minute), "/a.jsonl", "local",
	)
	database.InsertUsageWithTime(
		watcher.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 2000, CachedTokens: 8000, OutputTokens: 400},
		3.00, now.Add(-2*time.Hour), "/b.jsonl", "local",
	)
	// Gemini records
	database.InsertUsageWithTime(
		watcher.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 500, CachedTokens: 0, OutputTokens: 100},
		0.80, now.Add(-10*time.Minute), "/c.jsonl", "local",
	)

	handler := web.NewHandler(database, calc)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data web.StatsResponse
	json.NewDecoder(resp.Body).Decode(&data)

	// Should have time periods
	if len(data.Periods) == 0 {
		t.Fatal("expected periods")
	}

	// Should have sources grouped
	if len(data.Sources) < 2 {
		t.Fatalf("expected at least 2 sources, got %d", len(data.Sources))
	}

	// Find Claude source
	var claude *web.SourceStats
	for i := range data.Sources {
		if data.Sources[i].Name == "Claude Code" {
			claude = &data.Sources[i]
		}
	}
	if claude == nil {
		t.Fatal("Claude Code source not found")
	}
	if claude.TotalInput != 3000 || claude.TotalCached != 13000 || claude.TotalOutput != 600 {
		t.Errorf("Claude totals wrong: %+v", claude)
	}
	if len(claude.Models) != 1 {
		t.Errorf("expected 1 Claude model, got %d", len(claude.Models))
	}

	// Should have devices list
	if len(data.Devices) == 0 {
		t.Error("expected devices list")
	}
}

func TestStaticPage(t *testing.T) {
	database, calc := setup(t)
	defer database.Close()

	handler := web.NewHandler(database, calc)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 for /, got %d", resp.StatusCode)
	}
}
