package watcher

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ParseLine parses a single JSONL line and returns a TokenUsage if it contains usage data.
// Exported so Scanner can reuse it.
func ParseLine(line string) (TokenUsage, bool) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line), &data); err != nil {
		return TokenUsage{}, false
	}

	ts := parseTimestamp(data)

	if msg, ok := data["message"].(map[string]interface{}); ok {
		if t, ok := msg["type"].(string); ok && t == "message" {
			if usage, ok := msg["usage"].(map[string]interface{}); ok {
				u := parseClaudeUsage(msg, usage)
				if isNoiseRecord(u) {
					return TokenUsage{}, false
				}
				if ts.IsZero() {
					ts = parseTimestamp(msg)
				}
				u.Timestamp = ts
				applyProjectFromCWD(&u, data, msg)
				if uuid, ok := data["uuid"].(string); ok {
					u.UUID = uuid
				}
				return u, true
			}
		}
	}

	if t, ok := data["type"].(string); ok && t == "assistant" {
		if usage, ok := data["usage"].(map[string]interface{}); ok {
			u := parseClaudeUsage(data, usage)
			u.Timestamp = ts
			applyProjectFromCWD(&u, data)
			return u, true
		}
	}

	if t, ok := data["type"].(string); ok && t == "gemini" {
		if tokens, ok := data["tokens"].(map[string]interface{}); ok {
			in := toInt(tokens["input"])
			out := toInt(tokens["output"])
			cached := toInt(tokens["cached"])
			thoughts := toInt(tokens["thoughts"])
			tool := toInt(tokens["tool"])
			total := toInt(tokens["total"])
			outputTotal := out + thoughts + tool
			if total > in+outputTotal {
				outputTotal = total - in
			}
			model := "gemini-2.5-pro"
			if m, ok := data["model"].(string); ok {
				model = m
			}
			u := TokenUsage{
				Source:       "Gemini CLI",
				Model:        model,
				InputTokens:  in,
				OutputTokens: outputTotal,
				CachedTokens: cached,
				Thoughts:     thoughts,
				Timestamp:    ts,
			}
			applyProjectFromCWD(&u, data)
			if id, ok := data["id"].(string); ok {
				u.UUID = id
			}
			return u, true
		}
	}

	return TokenUsage{}, false
}

func StableGeminiUUID(deviceID string, path string, lineStartOffset int64) string {
	deviceHash := sha256.Sum256([]byte(deviceID))
	pathHash := sha256.Sum256([]byte(filepath.ToSlash(path)))
	return fmt.Sprintf("gemini:%x:%x:%d", deviceHash[:8], pathHash[:16], lineStartOffset)
}

func parseClaudeUsage(container map[string]interface{}, usage map[string]interface{}) TokenUsage {
	model := "claude-3-7-sonnet-20250219"
	if m, ok := container["model"].(string); ok {
		model = m
	}
	source := "Claude Code"
	if strings.HasPrefix(model, "gemini") {
		source = "Gemini CLI"
	}
	cached := toInt(usage["cache_read_input_tokens"])
	cacheCreation := toInt(usage["cache_creation_input_tokens"])
	return TokenUsage{
		Source:              source,
		Model:               model,
		InputTokens:         toInt(usage["input_tokens"]) + cached + cacheCreation,
		OutputTokens:        toInt(usage["output_tokens"]),
		CachedTokens:        cached,
		CacheCreationTokens: cacheCreation,
	}
}

func applyProjectFromCWD(u *TokenUsage, containers ...map[string]interface{}) {
	project := projectNameFromCWDFields(containers...)
	if project != "" {
		u.Project = project
	}
}

// WithProjectFallback fills Project only when the parsed log did not provide
// reliable metadata. Folder names are a fallback because some CLIs encode path
// separators and hyphens identically.
func WithProjectFallback(u TokenUsage, fallback string) TokenUsage {
	if u.Project == "" || u.Project == "Default" {
		u.Project = fallback
	}
	if u.Project == "" {
		u.Project = "Default"
	}
	return u
}

// isNoiseRecord filters out synthetic/empty model records.
func isNoiseRecord(u TokenUsage) bool {
	if strings.HasPrefix(u.Model, "<") {
		return true
	}
	if u.InputTokens == 0 && u.OutputTokens == 0 && u.CachedTokens == 0 && u.CacheCreationTokens == 0 {
		return true
	}
	return false
}

func parseTimestamp(data map[string]interface{}) time.Time {
	if ts, ok := data["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			return t
		}
		if t, err := time.Parse("2006-01-02T15:04:05.000Z", ts); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			return t
		}
	}
	return time.Time{}
}

func toInt(v interface{}) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}
