package onboarding

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/testutil"
)

func TestBuildSourceCoverageReportsDetectedWatchingMissingAndUnsupportedSources(t *testing.T) {
	database := testutil.NewTestDB(t)
	home := t.TempDir()

	claudeDir := filepath.Join(home, ".claude", "projects")
	writeTestFile(t, filepath.Join(claudeDir, "session.jsonl"))

	geminiDir := filepath.Join(home, ".gemini", "tmp")
	writeTestFile(t, filepath.Join(geminiDir, "token", "chats", "session.jsonl"))

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{
			Source:       "Gemini CLI",
			Model:        "gemini-2.5-pro",
			Project:      "token",
			InputTokens:  1000,
			OutputTokens: 200,
		},
		1.25,
		now,
		filepath.Join(geminiDir, "token", "chats", "session.jsonl"),
		"local",
	); err != nil {
		t.Fatal(err)
	}

	status, err := BuildSourceCoverage(database, Options{HomeDir: home})
	if err != nil {
		t.Fatal(err)
	}

	claude := findSourceStatus(t, status.Sources, "Claude Code")
	if claude.Status != "detected" || claude.Health != "pending_import" {
		t.Fatalf("expected Claude detected pending import, got %+v", claude)
	}
	if claude.DataDir != claudeDir {
		t.Fatalf("expected Claude data dir %q, got %q", claudeDir, claude.DataDir)
	}

	gemini := findSourceStatus(t, status.Sources, "Gemini CLI")
	if gemini.Status != "watching" || gemini.Health != "complete" {
		t.Fatalf("expected Gemini watching complete, got %+v", gemini)
	}
	if gemini.Records != 1 || gemini.TotalCost != 1.25 || gemini.LastSeen == nil || gemini.LastSeen.IsZero() {
		t.Fatalf("expected Gemini DB aggregate, got %+v", gemini)
	}

	codex := findSourceStatus(t, status.Sources, "Codex")
	if codex.Status != "no_data" || codex.Health != "unavailable" {
		t.Fatalf("expected Codex no_data unavailable, got %+v", codex)
	}
	if codex.LastSeen != nil {
		t.Fatalf("expected Codex last_seen to be omitted without records, got %+v", codex.LastSeen)
	}

	antigravity := findSourceStatus(t, status.Sources, "Antigravity")
	if antigravity.Status != "unsupported" || antigravity.Health != "unsupported" {
		t.Fatalf("expected Antigravity unsupported, got %+v", antigravity)
	}
	if antigravity.Reason == "" {
		t.Fatal("expected unsupported source reason")
	}
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func findSourceStatus(t *testing.T, sources []model.SourceCoverage, source string) model.SourceCoverage {
	t.Helper()
	for _, item := range sources {
		if item.Source == source {
			return item
		}
	}
	t.Fatalf("source %q not found in %+v", source, sources)
	return model.SourceCoverage{}
}
