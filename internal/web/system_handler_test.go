package web_test

import (
	"ai-flight-dashboard/internal/config"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/testutil"
	"ai-flight-dashboard/internal/web"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
