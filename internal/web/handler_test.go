package web_test

import (
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/testutil"
	"ai-flight-dashboard/internal/web"
)

// emptyFS is a placeholder for tests that don't need dist-bin binaries
var emptyFS embed.FS

func TestAPIStats(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()

	// Claude records
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 5000, OutputTokens: 200},
		1.50, now.Add(-30*time.Minute), "/a.jsonl", "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 2000, CachedTokens: 8000, OutputTokens: 400},
		3.00, now.Add(-2*time.Hour), "/b.jsonl", "local",
	)
	// Gemini records
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 500, CachedTokens: 0, OutputTokens: 100},
		0.80, now.Add(-10*time.Minute), "/c.jsonl", "local",
	)

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
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

	var data model.StatsResponse
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
	var claude *model.SourceStats
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
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
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

func TestAPITrack(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	handler := web.NewHandler(database, calc, nil, nil, "secret-token", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	payload := `{"device_id":"remote-test","usage":{"source":"Claude Code","model":"claude-opus-4-7","input_tokens":100,"output_tokens":50}}`

	// Test unauthorized
	req, _ := http.NewRequest("POST", srv.URL+"/api/track", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test authorized
	req, _ = http.NewRequest("POST", srv.URL+"/api/track", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify it was inserted
	cost, _, _, _, _, _ := database.QueryPeriodStatsAll("remote-test", "")
	if cost == 0 {
		t.Fatal("expected cost to be calculated and inserted")
	}
}

func TestAPICacheSavings(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()

	// Claude: 1000 input (800 cached), 200 output
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 800, OutputTokens: 200},
		0, now.Add(-10*time.Minute), "/a.jsonl", "local",
	)
	// Gemini: 500 input (0 cached), 100 output
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 500, CachedTokens: 0, OutputTokens: 100},
		0, now.Add(-5*time.Minute), "/b.jsonl", "local",
	)

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/cache-savings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data model.CacheSavingsResponse
	json.NewDecoder(resp.Body).Decode(&data)

	// actual_cost: calculator computes real cost with cached pricing
	// hypothetical_cost: calculator computes cost as if cached_tokens were charged at full input price
	if data.HypotheticalCost <= data.ActualCost {
		t.Errorf("hypothetical should exceed actual: hypo=%f actual=%f", data.HypotheticalCost, data.ActualCost)
	}
	if data.Saved < 0 {
		t.Errorf("saved should be >= 0, got %f", data.Saved)
	}
	if data.CacheHitRate < 0 || data.CacheHitRate > 100 {
		t.Errorf("cache hit rate should be 0-100, got %f", data.CacheHitRate)
	}
}
