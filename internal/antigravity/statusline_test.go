package antigravity

import (
	"strings"
	"testing"
	"time"
)

func TestParseStatuslineBuildsUsageFromCamelCaseMetadata(t *testing.T) {
	raw := []byte(`{
		"cwd": "/Users/c/token",
		"activeModel": "gemini-2.5-pro",
		"conversationId": "conv-1",
		"stepIdx": 7,
		"tokenUsage": {
			"promptTokenCount": 1000,
			"cachedContentTokenCount": 200,
			"candidatesTokenCount": 150,
			"thoughtsTokenCount": 50,
			"totalTokenCount": 1200
		}
	}`)

	usage, ok, err := ParseStatusline(raw, time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected usage")
	}

	if usage.Source != "Antigravity" {
		t.Fatalf("expected Antigravity source, got %q", usage.Source)
	}
	if usage.Model != "gemini-2.5-pro" {
		t.Fatalf("expected model from activeModel, got %q", usage.Model)
	}
	if usage.Project != "token" {
		t.Fatalf("expected cwd-derived project token, got %q", usage.Project)
	}
	if usage.InputTokens != 1000 || usage.CachedTokens != 200 || usage.OutputTokens != 200 || usage.Thoughts != 50 {
		t.Fatalf("unexpected token accounting: %+v", usage)
	}
	if usage.UUID != "antigravity-statusline:conv-1" {
		t.Fatalf("expected conversation-level UUID, got %q", usage.UUID)
	}
	if usage.Timestamp.IsZero() {
		t.Fatal("expected timestamp to be set")
	}
}

func TestParseStatuslineSupportsSnakeCaseUsageMetadata(t *testing.T) {
	raw := []byte(`{
		"workspacePaths": ["/Users/c/wiki"],
		"model": "gemini-2.5-pro",
		"conversation_id": "conv-2",
		"usage_metadata": {
			"prompt_token_count": 800,
			"cached_content_token_count": 100,
			"candidates_token_count": 80,
			"thoughts_token_count": 20,
			"total_token_count": 900
		}
	}`)

	usage, ok, err := ParseStatusline(raw, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected usage")
	}
	if usage.Project != "wiki" {
		t.Fatalf("expected workspace-derived project wiki, got %q", usage.Project)
	}
	if usage.InputTokens != 800 || usage.CachedTokens != 100 || usage.OutputTokens != 100 || usage.Thoughts != 20 {
		t.Fatalf("unexpected token accounting: %+v", usage)
	}
	if usage.UUID != "antigravity-statusline:conv-2" {
		t.Fatalf("expected conversation-level UUID, got %q", usage.UUID)
	}
}

func TestParseStatuslineSupportsCurrentCLIStatuslineShape(t *testing.T) {
	raw := []byte(`{
		"cwd": "/Users/c",
		"session_id": "e5eb6631-8ca4-4119-b314-6625b2894992",
		"conversation_id": "e5eb6631-8ca4-4119-b314-6625b2894992",
		"transcript_path": "/Users/c/.gemini/antigravity/brain/e5eb6631-8ca4-4119-b314-6625b2894992/.system_generated/logs/transcript.jsonl",
		"model": {
			"id": "Gemini 3.5 Flash (High)",
			"display_name": "Gemini 3.5 Flash (High)"
		},
		"workspace": {
			"current_dir": "/Users/c",
			"project_dir": "/Users/c/token"
		},
		"context_window": {
			"total_input_tokens": 161,
			"total_output_tokens": 1064,
			"context_window_size": 1048576,
			"current_usage": {
				"input_tokens": 16857,
				"output_tokens": 1064,
				"cache_creation_input_tokens": 10,
				"cache_read_input_tokens": 42
			}
		},
		"product": "antigravity",
		"agent_state": "idle"
	}`)

	usage, ok, err := ParseStatusline(raw, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected usage")
	}
	if usage.Model != "gemini-3.5-flash" {
		t.Fatalf("expected normalized Gemini 3.5 Flash model, got %q", usage.Model)
	}
	if usage.Project != "token" {
		t.Fatalf("expected workspace.project_dir-derived project token, got %q", usage.Project)
	}
	if usage.InputTokens != 16909 || usage.CachedTokens != 42 || usage.CacheCreationTokens != 10 || usage.OutputTokens != 1064 {
		t.Fatalf("unexpected current CLI token accounting: %+v", usage)
	}
	if usage.UUID != "antigravity-statusline:e5eb6631-8ca4-4119-b314-6625b2894992" {
		t.Fatalf("expected conversation-level UUID, got %q", usage.UUID)
	}
}

func TestParseStatuslineIgnoresZeroCurrentUsageFromInitializingSession(t *testing.T) {
	raw := []byte(`{
		"cwd": "/private/tmp/token-antigravity-pr",
		"session_id": "47944395-8267-491e-857e-cded9e285e7e",
		"conversation_id": "47944395-8267-491e-857e-cded9e285e7e",
		"model": {
			"id": "Gemini 3.5 Flash (High)",
			"display_name": "Gemini 3.5 Flash (High)"
		},
		"workspace": {
			"current_dir": "/private/tmp/token-antigravity-pr",
			"project_dir": "file:///private/tmp/token-antigravity-pr"
		},
		"context_window": {
			"total_input_tokens": 151,
			"total_output_tokens": 0,
			"context_window_size": 1048576,
			"current_usage": {
				"input_tokens": 0,
				"output_tokens": 0,
				"cache_creation_input_tokens": 0,
				"cache_read_input_tokens": 0
			}
		},
		"product": "antigravity",
		"agent_state": "idle"
	}`)

	usage, ok, err := ParseStatusline(raw, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("expected zero current usage to be ignored, got %+v", usage)
	}
}

func TestParseStatuslineDerivesOutputFromTotalWhenNeeded(t *testing.T) {
	raw := []byte(`{
		"currentWorkingDirectory": "/Users/c/video",
		"active_model": "gemini-2.5-pro",
		"transcriptPath": "/tmp/ag/transcript.jsonl",
		"usage": {
			"input_tokens": 1000,
			"cached_tokens": 300,
			"total_tokens": 1400
		}
	}`)

	usage, ok, err := ParseStatusline(raw, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected usage")
	}
	if usage.Project != "video" {
		t.Fatalf("expected cwd-derived project video, got %q", usage.Project)
	}
	if usage.InputTokens != 1000 || usage.CachedTokens != 300 || usage.OutputTokens != 400 {
		t.Fatalf("unexpected derived output accounting: %+v", usage)
	}
	if !strings.HasPrefix(usage.UUID, "antigravity-statusline:transcript:") {
		t.Fatalf("expected transcript-derived UUID, got %q", usage.UUID)
	}
}

func TestParseStatuslineNoUsageIsNoop(t *testing.T) {
	raw := []byte(`{"cwd":"/Users/c/token","activeModel":"gemini-2.5-pro","state":"idle"}`)

	usage, ok, err := ParseStatusline(raw, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("expected no usage, got %+v", usage)
	}
}
