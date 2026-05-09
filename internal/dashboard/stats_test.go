package dashboard_test

import (
	"testing"
	"time"

	"ai-flight-dashboard/internal/dashboard"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/testutil"
)

func TestBuildTokenSummaryIncludesPerSourceBreakdown(t *testing.T) {
	database := testutil.NewTestDB(t)
	defer database.Close()

	now := time.Now().UTC()
	for _, row := range []struct {
		usage  model.TokenUsage
		cost   float64
		device string
	}{
		{
			usage:  model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, OutputTokens: 200},
			cost:   1.20,
			device: "remote",
		},
		{
			usage:  model.TokenUsage{Source: "Codex", Model: "gpt-5.5", InputTokens: 3000, OutputTokens: 400},
			cost:   3.40,
			device: "remote",
		},
		{
			usage:  model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 5000, OutputTokens: 600},
			cost:   5.60,
			device: "other",
		},
	} {
		if err := database.InsertUsageWithTime(row.usage, row.cost, now, "/session.jsonl", row.device); err != nil {
			t.Fatal(err)
		}
	}

	summary, err := dashboard.BuildTokenSummary(database, "remote")
	if err != nil {
		t.Fatal(err)
	}

	if summary.TokensTotal != 4600 || summary.Tokens24h != 4600 || summary.CostTotal != 4.60 {
		t.Fatalf("unexpected total summary: %+v", summary)
	}
	if len(summary.Sources) != 2 {
		t.Fatalf("expected two remote source summaries, got %+v", summary.Sources)
	}
	if summary.Sources[0].Source != "Claude Code" || summary.Sources[0].TokensTotal != 1200 {
		t.Fatalf("unexpected first source summary: %+v", summary.Sources)
	}
	if summary.Sources[1].Source != "Codex" || summary.Sources[1].TokensTotal != 3400 {
		t.Fatalf("unexpected second source summary: %+v", summary.Sources)
	}
}
