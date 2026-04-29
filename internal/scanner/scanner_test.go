package scanner_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/scanner"
)

func setupCalc(t *testing.T) *calculator.Calculator {
	t.Helper()
	pricingPath := filepath.Join(t.TempDir(), "pricing.json")
	os.WriteFile(pricingPath, []byte(`{"models":{"claude-opus-4-7":{"input_price_per_m":15,"cached_price_per_m":1.5,"output_price_per_m":75},"gemini-2.5-pro":{"input_price_per_m":1.25,"cached_price_per_m":0.31,"output_price_per_m":5}}}`), 0644)
	calc, err := calculator.New(pricingPath)
	if err != nil {
		t.Fatal(err)
	}
	return calc
}

func TestScanAll_FullScan(t *testing.T) {
	database, _ := db.New(filepath.Join(t.TempDir(), "test.db"))
	defer database.Close()
	calc := setupCalc(t)

	// Create fake Claude log directory structure
	logDir := t.TempDir()
	projectDir := filepath.Join(logDir, "projects", "-Users-c-test")
	os.MkdirAll(projectDir, 0755)

	ts := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339Nano)
	logFile := filepath.Join(projectDir, "session-abc.jsonl")
	content := fmt.Sprintf(
		`{"parentUuid":"x","timestamp":"%s","message":{"model":"claude-opus-4-7","type":"message","role":"assistant","content":[],"usage":{"input_tokens":1000,"cache_read_input_tokens":5000,"output_tokens":200}}}`,
		ts,
	) + "\n"
	// Write 3 usage lines
	os.WriteFile(logFile, []byte(content+content+content), 0644)

	s := scanner.New(database, calc, "test-device")
	count, err := s.ScanAll([]string{logDir})
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("expected 3 records scanned, got %d", count)
	}

	// Verify DB has data
	total, _, _, _, _ := database.QueryPeriodStatsAll("")
	if total < 0.01 {
		t.Errorf("expected non-zero cost, got %f", total)
	}
}

func TestScanAll_IncrementalScan(t *testing.T) {
	database, _ := db.New(filepath.Join(t.TempDir(), "test.db"))
	defer database.Close()
	calc := setupCalc(t)

	logDir := t.TempDir()
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	logFile := filepath.Join(logDir, "session.jsonl")
	line := fmt.Sprintf(
		`{"timestamp":"%s","message":{"model":"claude-opus-4-7","type":"message","role":"assistant","content":[],"usage":{"input_tokens":500,"cache_read_input_tokens":0,"output_tokens":100}}}`,
		ts,
	) + "\n"

	// First scan: 2 lines
	os.WriteFile(logFile, []byte(line+line), 0644)
	s := scanner.New(database, calc, "local")
	count1, _ := s.ScanAll([]string{logDir})
	if count1 != 2 {
		t.Errorf("first scan: expected 2, got %d", count1)
	}

	// Second scan without changes: should be 0
	count2, _ := s.ScanAll([]string{logDir})
	if count2 != 0 {
		t.Errorf("second scan: expected 0 (no new data), got %d", count2)
	}

	// Append 1 more line
	f, _ := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString(line)
	f.Close()

	// Third scan: only the new line
	count3, _ := s.ScanAll([]string{logDir})
	if count3 != 1 {
		t.Errorf("third scan: expected 1 (incremental), got %d", count3)
	}
}

func TestScanAll_GeminiFormat(t *testing.T) {
	database, _ := db.New(filepath.Join(t.TempDir(), "test.db"))
	defer database.Close()
	calc := setupCalc(t)

	logDir := t.TempDir()
	chatsDir := filepath.Join(logDir, "chats")
	os.MkdirAll(chatsDir, 0755)

	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	logFile := filepath.Join(chatsDir, "session-2026.jsonl")
	content := fmt.Sprintf(
		`{"id":"abc","timestamp":"%s","type":"gemini","content":"Hello","tokens":{"input":8000,"output":5,"cached":0},"model":"gemini-2.5-pro"}`,
		ts,
	) + "\n"
	os.WriteFile(logFile, []byte(content), 0644)

	s := scanner.New(database, calc, "local")
	count, _ := s.ScanAll([]string{logDir})
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}

	total, _, _, _, _ := database.QueryPeriodStatsAll("")
	if total < 0.01 {
		t.Errorf("expected non-zero Gemini cost, got %f", total)
	}
}

func TestScanAll_SkipsNonUsageLines(t *testing.T) {
	database, _ := db.New(filepath.Join(t.TempDir(), "test.db"))
	defer database.Close()
	calc := setupCalc(t)

	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "session.jsonl")
	// Mix of usage and non-usage lines
	content := `{"type":"system","content":"init"}
{"parentUuid":"x","timestamp":"2026-04-20T10:00:00Z","message":{"model":"claude-opus-4-7","type":"message","role":"assistant","content":[],"usage":{"input_tokens":100,"cache_read_input_tokens":0,"output_tokens":50}}}
{"type":"tool_result","content":"ok"}
`
	os.WriteFile(logFile, []byte(content), 0644)

	s := scanner.New(database, calc, "local")
	count, _ := s.ScanAll([]string{logDir})
	if count != 1 {
		t.Errorf("expected 1 usage line only, got %d", count)
	}
}
