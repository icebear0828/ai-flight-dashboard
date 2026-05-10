package web_test

import (
	"embed"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"ai-flight-dashboard/internal/config"
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

func TestAPIStatsDetailModes(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Codex", Model: "gpt-5.5", Project: "dashboard", InputTokens: 2000, CachedTokens: 1500, OutputTokens: 300},
		2.50, now.Add(-1*time.Minute), "/codex.sqlite", "local",
	); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	summaryResp, err := http.Get(srv.URL + "/api/stats?device=all&source=Codex&detail=summary")
	if err != nil {
		t.Fatal(err)
	}
	defer summaryResp.Body.Close()
	if summaryResp.StatusCode != http.StatusOK {
		t.Fatalf("expected summary 200, got %d", summaryResp.StatusCode)
	}
	var summary model.StatsResponse
	if err := json.NewDecoder(summaryResp.Body).Decode(&summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Periods) == 0 || len(summary.Devices) == 0 {
		t.Fatalf("expected summary periods/devices, got %+v", summary)
	}
	if len(summary.Sources) != 1 || len(summary.Sources[0].Models) != 0 || len(summary.Projects) != 0 {
		t.Fatalf("summary should include source totals only, got %+v", summary)
	}

	detailsResp, err := http.Get(srv.URL + "/api/stats?device=all&source=Codex&detail=details")
	if err != nil {
		t.Fatal(err)
	}
	defer detailsResp.Body.Close()
	if detailsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected details 200, got %d", detailsResp.StatusCode)
	}
	var details model.StatsResponse
	if err := json.NewDecoder(detailsResp.Body).Decode(&details); err != nil {
		t.Fatal(err)
	}
	if len(details.Periods) != 0 || len(details.Devices) != 0 {
		t.Fatalf("details should omit summary periods/devices, got %+v", details)
	}
	if len(details.Sources) != 1 || len(details.Sources[0].Models) != 1 || len(details.Projects) != 1 {
		t.Fatalf("expected details models/projects, got %+v", details)
	}

	badResp, err := http.Get(srv.URL + "/api/stats?detail=invalid")
	if err != nil {
		t.Fatal(err)
	}
	defer badResp.Body.Close()
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid detail 400, got %d", badResp.StatusCode)
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
	for _, row := range []struct {
		usage model.TokenUsage
		cost  float64
		path  string
	}{
		{
			usage: model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, OutputTokens: 200},
			cost:  1.50,
			path:  "/remote-claude.jsonl",
		},
		{
			usage: model.TokenUsage{Source: "Codex", Model: "gpt-5.5", InputTokens: 3000, OutputTokens: 400},
			cost:  3.40,
			path:  "/remote-codex.jsonl",
		},
	} {
		if err := database.InsertUsageWithTime(row.usage, row.cost, now.Add(-10*time.Minute), row.path, "remote-device"); err != nil {
			t.Fatal(err)
		}
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
	if peer.Tokens24h != 4600 || peer.TokensTotal != 4600 {
		t.Fatalf("expected token summaries to include synced DB data, got %+v", peer)
	}
	if len(peer.Sources) != 2 || peer.Sources[0].Source != "Claude Code" || peer.Sources[1].Source != "Codex" {
		t.Fatalf("expected LAN peer source summaries, got %+v", peer.Sources)
	}
}

func TestLANScanUsesAdvertisedSummaryWhenPeerIsDiscoveryOnly(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	lanInst := lan.New("local-device", 19100)
	lanInst.RecordPeerSummary("remote-device", "192.168.1.25", 0, model.TokenSummary{
		Tokens24h:   1200,
		TokensTotal: 3400,
		CostTotal:   1.23,
		Sources: []model.TokenSourceSummary{
			{Source: "Claude Code", Tokens24h: 700, TokensTotal: 2100, CostTotal: 0.80},
			{Source: "Gemini CLI", Tokens24h: 500, TokensTotal: 1300, CostTotal: 0.43},
		},
	})

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
	if len(data.PeerInfos) != 1 {
		t.Fatalf("expected one peer info, got %+v", data.PeerInfos)
	}
	peer := data.PeerInfos[0]
	if peer.SyncStatus != "discovery_only" {
		t.Fatalf("expected discovery_only sync status, got %+v", peer)
	}
	if peer.Tokens24h != 1200 || peer.TokensTotal != 3400 || peer.CostTotal != 1.23 {
		t.Fatalf("expected advertised token summary, got %+v", peer)
	}
	if len(peer.Sources) != 2 || peer.Sources[0].Source != "Claude Code" || peer.Sources[1].Source != "Gemini CLI" {
		t.Fatalf("expected advertised source summaries, got %+v", peer.Sources)
	}
}

func TestLANSelfEndpointIncludesDeviceAndSummary(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	lanInst := lan.New("local-device", 19100)
	lanInst.SetSummaryProvider(func() model.TokenSummary {
		return model.TokenSummary{
			Tokens24h:   222,
			TokensTotal: 333,
			CostTotal:   4.56,
			Sources: []model.TokenSourceSummary{
				{Source: "Codex", Tokens24h: 222, TokensTotal: 333, CostTotal: 4.56},
			},
		}
	})

	handler := web.NewHandler(database, calc, nil, lanInst, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/lan/self")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data model.LANSelfResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if data.DeviceID != "local-device" || data.HTTPPort != 19100 {
		t.Fatalf("unexpected LAN self identity: %+v", data)
	}
	if data.Summary == nil || data.Summary.Tokens24h != 222 || data.Summary.TokensTotal != 333 || data.Summary.CostTotal != 4.56 {
		t.Fatalf("unexpected LAN self summary: %+v", data.Summary)
	}
	if len(data.Summary.Sources) != 1 || data.Summary.Sources[0].Source != "Codex" {
		t.Fatalf("unexpected LAN self source summary: %+v", data.Summary.Sources)
	}
}

func TestLANScanAndJoinRemainAvailableWithSyncToken(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	lanInst := lan.New("local-device", 19100)
	lanInst.RecordPeer("remote-device", "192.168.1.25", 0)

	handler := web.NewHandler(database, calc, nil, lanInst, "secret-token", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/lan/scan")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected unauthenticated UI LAN scan to remain available with sync token, got %d", resp.StatusCode)
	}

	var data model.LANScanResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if len(data.PeerInfos) != 1 || data.PeerInfos[0].ID != "remote-device" {
		t.Fatalf("expected LAN peer info, got %+v", data)
	}

	resp, err = http.Post(srv.URL+"/api/lan/join", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected unauthenticated UI LAN join to remain available with sync token, got %d", resp.StatusCode)
	}
}

func TestLANStatusJoinAndLeave(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	lanInst := lan.New("local-device", 19100)
	handler := web.NewHandler(database, calc, nil, lanInst, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	client := http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get(srv.URL + "/api/lan/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected LAN status 200, got %d", resp.StatusCode)
	}
	var status model.LANStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if !status.Enabled {
		t.Fatalf("expected LAN enabled before leave, got %+v", status)
	}

	resp, err = client.Post(srv.URL+"/api/lan/leave", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected LAN leave 200, got %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Enabled {
		t.Fatalf("expected LAN disabled after leave, got %+v", status)
	}

	resp, err = client.Post(srv.URL+"/api/lan/join", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected LAN join 200, got %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if !status.Enabled {
		t.Fatalf("expected LAN enabled after join, got %+v", status)
	}
}

func TestSystemLogsEndpointReturnsStatsDirectory(t *testing.T) {
	dataDir := t.TempDir()
	config.SetDataDir(dataDir)
	defer config.SetDataDir("")

	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/system/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected system logs endpoint 200, got %d", resp.StatusCode)
	}

	var data model.SystemLogsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dataDir, "stats")
	if data.Path != want {
		t.Fatalf("expected system logs path %q, got %q", want, data.Path)
	}
}

func TestPricingPersistenceUsesDataDir(t *testing.T) {
	dataDir := t.TempDir()
	config.SetDataDir(dataDir)
	defer config.SetDataDir("")

	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	handler := web.NewHandler(database, calc, nil, nil, "secret-token", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	body := strings.NewReader(`[{
		"model": "gpt-5.5",
		"input_price_per_m": 6,
		"cached_price_per_m": 0.6,
		"cache_creation_price_per_m": 6,
		"output_price_per_m": 36
	}]`)
	req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/pricing", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected pricing update 200, got %d", resp.StatusCode)
	}

	customPricingPath := filepath.Join(dataDir, "custom_pricing.json")
	data, err := os.ReadFile(customPricingPath)
	if err != nil {
		t.Fatalf("expected custom pricing to be written to data-dir: %v", err)
	}
	if !strings.Contains(string(data), `"gpt-5.5"`) {
		t.Fatalf("custom pricing missing updated model: %s", string(data))
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

func TestSyncPullFiltersDeviceID(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 100, OutputTokens: 10, UUID: "local-first"},
		1.00, now, "/local.jsonl", "local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 200, OutputTokens: 20, UUID: "remote-first"},
		2.00, now.Add(time.Second), "/remote.jsonl", "remote",
	); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "secret-token", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/sync/pull?limit=1&device_id=remote", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected sync pull OK, got %d", resp.StatusCode)
	}

	var page model.SyncPullResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 1 || page.Records[0].DeviceID != "remote" || page.Records[0].UUID != "remote-first" {
		t.Fatalf("expected filtered remote sync record, got %+v", page)
	}
}

func TestLANHandlerExposesOnlySyncSurface(t *testing.T) {
	database, _ := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	lanInst := lan.New("local-device", 19100)
	handler := web.NewLANHandler(database, "secret-token", lanInst)
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

func TestSyncPullAllowsPrivateLANWithoutToken(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, OutputTokens: 200, UUID: "zero-config-sync"},
		1.50, now, "/remote.jsonl", "local-device",
	); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/pull", nil)
	req.RemoteAddr = "192.168.10.5:42310"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected private LAN sync pull without token, got %d", rec.Code)
	}

	var data model.SyncPullResponse
	if err := json.NewDecoder(rec.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if len(data.Records) != 1 || data.Records[0].UUID != "zero-config-sync" {
		t.Fatalf("expected zero-config sync record, got %+v", data)
	}
}

func TestSyncPullRejectsPublicRemoteWithoutToken(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/pull", nil)
	req.RemoteAddr = "203.0.113.5:42310"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected public sync pull without token to be rejected, got %d", rec.Code)
	}
}

func TestDevicesAPIListsAliasesAndSupersedesDevice(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50, UUID: "old-device-row"},
		1.00, now, "/old.jsonl", "probe-local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "m2", InputTokens: 200, OutputTokens: 75, UUID: "real-device-row"},
		2.00, now, "/real.jsonl", "nas.local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.SetDeviceAlias("nas.local", "NAS"); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	req.RemoteAddr = "127.0.0.1:42310"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected devices list, got %d", rec.Code)
	}
	var devices []model.DeviceSummary
	if err := json.NewDecoder(rec.Body).Decode(&devices); err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected two devices, got %+v", devices)
	}
	if devices[0].ID != "nas.local" || devices[0].DisplayName != "NAS" {
		t.Fatalf("expected aliased nas.local first by cost, got %+v", devices)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/devices?device_id=probe-local", nil)
	deleteReq.RemoteAddr = "127.0.0.1:42310"
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected supersede success, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}
	var result model.DeviceSupersedeResponse
	if err := json.NewDecoder(deleteRec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.DeviceID != "probe-local" || result.SupersededCount != 1 {
		t.Fatalf("unexpected supersede response: %+v", result)
	}

	remaining, err := database.QueryDevices()
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0] != "nas.local" {
		t.Fatalf("expected probe-local hidden, got %+v", remaining)
	}
}

func TestDeviceAliasAPIDeletesAlias(t *testing.T) {
	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()
	if err := database.SetDeviceAlias("nas.local", "NAS"); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	req := httptest.NewRequest(http.MethodDelete, "/api/device-alias?device_id=nas.local", nil)
	req.RemoteAddr = "127.0.0.1:42310"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected alias delete success, got %d", rec.Code)
	}

	aliases, err := database.GetDeviceAliases()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := aliases["nas.local"]; ok {
		t.Fatalf("expected alias to be deleted, got %+v", aliases)
	}
}
