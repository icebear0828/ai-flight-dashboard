package watcher_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-flight-dashboard/internal/watcher"
)

func TestWatcher(t *testing.T) {
	w, err := watcher.New("test")
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	tempDir := t.TempDir()
	err = w.WatchDir(tempDir)
	if err != nil {
		t.Fatalf("failed to watch dir: %v", err)
	}

	logFile := filepath.Join(tempDir, "session.jsonl")
	
	// Write a Claude log (Trigger Create event)
	claudeLog := `{"type":"assistant", "model": "claude-3-7-sonnet-20250219", "usage": {"input_tokens": 100, "output_tokens": 50, "cache_read_input_tokens": 20}}` + "\n"
	err = os.WriteFile(logFile, []byte(claudeLog), 0644)
	if err != nil {
		t.Fatalf("failed to write log: %v", err)
	}

	// Wait for event
	select {
	case usage := <-w.UsageChan:
		if usage.Source != "Claude Code" {
			t.Errorf("Expected source Claude Code, got %s", usage.Source)
		}
		// Claude: input_tokens=100 + cache_read=20 => InputTokens=120, CachedTokens=20
		if usage.InputTokens != 120 || usage.OutputTokens != 50 || usage.CachedTokens != 20 {
			t.Errorf("Tokens parsed incorrectly: %+v", usage)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for watcher event (Claude)")
	}

	// Write a Gemini log by appending (Trigger Write event)
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	geminiLog := `{"type":"gemini", "model": "gemini-2.5-pro", "tokens": {"input": 200, "output": 150, "cached": 50}}` + "\n"
	_, err = file.WriteString(geminiLog)
	file.Close()

	// Wait for second event
	select {
	case usage := <-w.UsageChan:
		if usage.Source != "Gemini CLI" {
			t.Errorf("Expected source Gemini CLI, got %s", usage.Source)
		}
		if usage.InputTokens != 200 || usage.OutputTokens != 150 || usage.CachedTokens != 50 {
			t.Errorf("Tokens parsed incorrectly: %+v", usage)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for second watcher event (Gemini)")
	}
}

func TestRealClaudeCodeFormat(t *testing.T) {
	w, err := watcher.New("test")
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	tempDir := t.TempDir()
	w.WatchDir(tempDir)

	logFile := filepath.Join(tempDir, "session.jsonl")

	// This is the REAL format Claude Code writes to disk
	realLog := `{"parentUuid":"abc","message":{"model":"claude-sonnet-4-6","type":"message","role":"assistant","content":[],"stop_reason":"end_turn","usage":{"input_tokens":5000,"cache_creation_input_tokens":200,"cache_read_input_tokens":10000,"output_tokens":1500}}}` + "\n"
	err = os.WriteFile(logFile, []byte(realLog), 0644)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case usage := <-w.UsageChan:
		if usage.Source != "Claude Code" {
			t.Errorf("Expected source Claude Code, got %s", usage.Source)
		}
		if usage.Model != "claude-sonnet-4-6" {
			t.Errorf("Expected model claude-sonnet-4-6, got %s", usage.Model)
		}
		// Claude: input_tokens=5000 + cache_read=10000 => InputTokens=15000
		if usage.InputTokens != 15000 {
			t.Errorf("Expected input 15000 (5000+10000), got %d", usage.InputTokens)
		}
		if usage.CachedTokens != 10000 {
			t.Errorf("Expected cached 10000, got %d", usage.CachedTokens)
		}
		if usage.OutputTokens != 1500 {
			t.Errorf("Expected output 1500, got %d", usage.OutputTokens)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout: watcher did not parse real Claude Code JSONL format")
	}
}

func TestWatchDirRecursive(t *testing.T) {
	w, err := watcher.New("test")
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	// Create nested dir structure simulating ~/.claude/projects/-Users-c-xxx/<uuid>/
	tempDir := t.TempDir()
	projectDir := filepath.Join(tempDir, "projects", "-Users-c-myproject")
	subagentDir := filepath.Join(projectDir, "abc-uuid", "subagents")
	os.MkdirAll(subagentDir, 0755)

	err = w.WatchDirRecursive(tempDir)
	if err != nil {
		t.Fatalf("failed to watch dir recursive: %v", err)
	}

	// Write jsonl into a deeply nested subdir (simulates real Claude log location)
	logFile := filepath.Join(projectDir, "abc-uuid.jsonl")
	// Claude: input=500 + cache_read=100 => InputTokens=600
	claudeLog := `{"type":"assistant", "model": "claude-3-7-sonnet-20250219", "usage": {"input_tokens": 500, "output_tokens": 200, "cache_read_input_tokens": 100}}` + "\n"
	err = os.WriteFile(logFile, []byte(claudeLog), 0644)
	if err != nil {
		t.Fatalf("failed to write nested log: %v", err)
	}

	select {
	case usage := <-w.UsageChan:
		if usage.InputTokens != 600 || usage.OutputTokens != 200 || usage.CachedTokens != 100 {
			t.Errorf("Nested dir tokens wrong: %+v (expected InputTokens=600)", usage)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout: recursive watcher did not pick up nested .jsonl")
	}

	// Also write into subagents/ dir
	subLog := filepath.Join(subagentDir, "agent-xxx.jsonl")
	// Claude: input=300 + cache_read=0 => InputTokens=300
	subEntry := `{"type":"assistant", "model": "claude-3-7-sonnet-20250219", "usage": {"input_tokens": 300, "output_tokens": 100, "cache_read_input_tokens": 0}}` + "\n"
	err = os.WriteFile(subLog, []byte(subEntry), 0644)
	if err != nil {
		t.Fatalf("failed to write subagent log: %v", err)
	}

	select {
	case usage := <-w.UsageChan:
		if usage.InputTokens != 300 || usage.OutputTokens != 100 {
			t.Errorf("Subagent dir tokens wrong: %+v", usage)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout: recursive watcher did not pick up subagent .jsonl")
	}
}

func TestWatchDirRecursive_NewSubdir(t *testing.T) {
	w, err := watcher.New("test")
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	tempDir := t.TempDir()
	err = w.WatchDirRecursive(tempDir)
	if err != nil {
		t.Fatalf("failed to watch: %v", err)
	}

	// Create a NEW subdirectory after watcher started (simulates new Claude session)
	newSessionDir := filepath.Join(tempDir, "new-session-uuid")
	os.MkdirAll(newSessionDir, 0755)
	// Give fsnotify time to auto-register the new subdir
	time.Sleep(200 * time.Millisecond)

	logFile := filepath.Join(newSessionDir, "session.jsonl")
	// Claude: input=999 + cache_read=0 => InputTokens=999
	entry := `{"type":"assistant", "model": "claude-3-7-sonnet-20250219", "usage": {"input_tokens": 999, "output_tokens": 111, "cache_read_input_tokens": 0}}` + "\n"
	err = os.WriteFile(logFile, []byte(entry), 0644)
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	select {
	case usage := <-w.UsageChan:
		if usage.InputTokens != 999 || usage.OutputTokens != 111 {
			t.Errorf("Dynamic subdir tokens wrong: %+v", usage)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout: watcher did not auto-register dynamically created subdir")
	}
}
