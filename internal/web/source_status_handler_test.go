package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/testutil"
	"ai-flight-dashboard/internal/web"
)

func TestAPISourceStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	database, calc := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	codexSessions := filepath.Join(home, ".codex", "sessions")
	if err := os.MkdirAll(codexSessions, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexSessions, "session.jsonl"), []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{
			Source:       "Codex",
			Model:        "gpt-5.5",
			Project:      "token",
			InputTokens:  2000,
			OutputTokens: 300,
		},
		2.5,
		now,
		filepath.Join(codexSessions, "session.jsonl"),
		"local",
	); err != nil {
		t.Fatal(err)
	}

	handler := web.NewHandler(database, calc, nil, nil, "", emptyFS)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/sources/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var data model.SourceCoverageResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}

	codex := findAPISourceStatus(t, data.Sources, "Codex")
	if codex.Status != "watching" || codex.Health != "complete" {
		t.Fatalf("expected Codex watching complete, got %+v", codex)
	}
	if codex.Records != 1 || codex.TotalCost != 2.5 || codex.LastSeen == nil || codex.LastSeen.IsZero() {
		t.Fatalf("expected Codex aggregate, got %+v", codex)
	}
}

func findAPISourceStatus(t *testing.T, sources []model.SourceCoverage, source string) model.SourceCoverage {
	t.Helper()
	for _, item := range sources {
		if item.Source == source {
			return item
		}
	}
	t.Fatalf("source %q not found in %+v", source, sources)
	return model.SourceCoverage{}
}
