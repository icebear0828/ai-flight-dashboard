package web_test

import (
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"ai-flight-dashboard/internal/lan"
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
	if len(claude.Models) == 1 {
		modelStats := claude.Models[0]
		if modelStats.InputTokens != 3000 || modelStats.CachedTokens != 13000 || modelStats.OutputTokens != 600 {
			t.Errorf("Claude model token breakdown missing/wrong: %+v", modelStats)
		}
		if modelStats.CacheCreationPricePerM != 22.5 {
			t.Errorf("Claude model cache creation price missing/wrong: %+v", modelStats)
		}
	}

	// Should have devices list
	if len(data.Devices) == 0 {
		t.Error("expected devices list")
	}
}

func TestAPIStatsCodexSourceFilter(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", Project: "api", InputTokens: 1000, OutputTokens: 200},
		1.50, now.Add(-2*time.Minute), "/claude.jsonl", "local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Codex", Model: "gpt-5.5", Project: "dashboard", InputTokens: 2000, CachedTokens: 1500, OutputTokens: 300},
		2.50, now.Add(-1*time.Minute), "/codex.sqlite", "local",
	); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(srv.URL + "/api/stats?device=all&source=Codex")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data model.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if len(data.Periods) != 8 {
		t.Fatalf("expected 8 periods, got %d", len(data.Periods))
	}
	if len(data.Sources) != 1 || data.Sources[0].Name != "Codex" {
		t.Fatalf("expected only Codex source, got %+v", data.Sources)
	}
	if len(data.Projects) != 1 || data.Projects[0].Project != "dashboard" {
		t.Fatalf("expected only Codex project, got %+v", data.Projects)
	}
	if data.Sources[0].TotalInput != 2000 || data.Sources[0].TotalCached != 1500 || data.Sources[0].TotalOutput != 300 {
		t.Fatalf("unexpected Codex token totals: %+v", data.Sources[0])
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

func TestLANScanIncludesPeerInfoAndTokenSummary(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, OutputTokens: 200},
		1.50, now.Add(-10*time.Minute), "/remote.jsonl", "remote-device",
	); err != nil {
		t.Fatal(err)
	}

	lanInst := lan.New("local-device", 19100)
	lanInst.RecordPeer("remote-device", "192.168.1.25", 19100)

	handler := web.NewHandler(database, calc, nil, lanInst, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/lan/scan")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data model.LANScanResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if len(data.Peers) != 1 || data.Peers[0] != "remote-device" {
		t.Fatalf("expected compatible peers list, got %+v", data.Peers)
	}
	if len(data.PeerInfos) != 1 {
		t.Fatalf("expected one peer info, got %+v", data.PeerInfos)
	}
	peer := data.PeerInfos[0]
	if peer.ID != "remote-device" || peer.IP != "192.168.1.25" || peer.HTTPPort != 19100 {
		t.Fatalf("unexpected peer info: %+v", peer)
	}
	if peer.SyncStatus != "pending" {
		t.Fatalf("expected pending sync status, got %q", peer.SyncStatus)
	}
	if peer.Tokens24h != 1200 || peer.TokensTotal != 1200 {
		t.Fatalf("expected token summaries to include synced DB data, got %+v", peer)
	}
}

func TestSyncPullPaginates(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	for i, uuid := range []string{"sync-pull-1", "sync-pull-2"} {
		if err := database.InsertUsageWithTime(
			model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 100 + i, OutputTokens: 10, UUID: uuid},
			1.00, now.Add(time.Duration(i)*time.Second), "/sync.jsonl", "remote",
		); err != nil {
			t.Fatal(err)
		}
	}

	handler := web.NewHandler(database, calc, nil, nil, "secret-token", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/sync/pull?limit=1", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var first model.SyncPullResponse
	if err := json.NewDecoder(resp.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	if len(first.Records) != 1 || !first.HasMore || first.NextUpdatedAt.IsZero() || first.NextAfterID == 0 {
		t.Fatalf("unexpected first sync page: %+v", first)
	}

	nextURL := srv.URL + "/api/sync/pull?limit=1&since=" + first.NextUpdatedAt.Format(time.RFC3339Nano) + "&after_id=" + strconv.FormatInt(first.NextAfterID, 10)
	req, _ = http.NewRequest(http.MethodGet, nextURL, nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var second model.SyncPullResponse
	if err := json.NewDecoder(resp.Body).Decode(&second); err != nil {
		t.Fatal(err)
	}
	if len(second.Records) != 1 || second.Records[0].UUID == first.Records[0].UUID {
		t.Fatalf("expected cursor to advance, first=%+v second=%+v", first, second)
	}
}

func TestLANHandlerExposesOnlySyncSurface(t *testing.T) {
	database, _ := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	handler := web.NewLANHandler(database, "secret-token")
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/stats")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected minimal LAN handler to hide dashboard API, got %d", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/sync/pull?limit=1", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected authorized sync pull, got %d", resp.StatusCode)
	}
}

func TestSyncPullRequiresToken(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/sync/pull")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected sync pull to require token, got %d", resp.StatusCode)
	}
}
