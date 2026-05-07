package codexusage_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"ai-flight-dashboard/internal/codexusage"
	"ai-flight-dashboard/internal/testutil"

	_ "github.com/mattn/go-sqlite3"
)

func TestScanImportsCodexTelemetryEvents(t *testing.T) {
	database := testutil.NewTestDB(t)
	calc := testutil.NewTestCalc(t)

	dir := t.TempDir()
	statePath := filepath.Join(dir, "state_5.sqlite")
	logsPath := filepath.Join(dir, "logs_2.sqlite")
	createCodexState(t, statePath)
	createCodexLogs(t, logsPath)

	insertThread(t, statePath, "thread-1", "gpt-5.5", "/Users/c/token")
	insertLog(t, logsPath, 10, `event.name="codex.sse_event" event.kind=response.completed duration_ms=178 event.timestamp=2026-05-07T11:13:03.315Z conversation.id=thread-1 model=gpt-5.5`)
	insertLog(t, logsPath, 11, `event.name="codex.sse_event" event.kind=response.completed input_token_count=115541 output_token_count=713 cached_token_count=49024 reasoning_token_count=261 tool_token_count=116254 event.timestamp=2026-05-07T11:13:03.316Z conversation.id=thread-1 model=gpt-5.5 slug=gpt-5.5`)
	insertLog(t, logsPath, 12, `event.name="codex.sse_event" event.kind=response.completed input_token_count=999 output_token_count=1 cached_token_count=0 event.timestamp=2026-05-07T11:13:05.000Z conversation.id=missing-thread model=gpt-5.5`)

	s := codexusage.NewWithPaths(database, calc, "local", statePath, logsPath)
	count, err := s.Scan(nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 imported telemetry event, got %d", count)
	}

	cost, input, cached, _, output, err := database.QueryPeriodStatsAll("", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if input != 115541 || cached != 49024 || output != 713 {
		t.Errorf("unexpected token totals: input=%d cached=%d output=%d", input, cached, output)
	}
	if cost <= 0 {
		t.Errorf("expected Codex cost to be non-zero for priced model, got %f", cost)
	}

	projects, err := database.QueryProjectStatsSince(time.Time{}, "", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Project != "token" {
		t.Fatalf("expected Codex project token, got %+v", projects)
	}

	insertThread(t, statePath, "missing-thread", "gpt-5.5", "/Users/c/recovered")

	secondCount, err := s.Scan(nil)
	if err != nil {
		t.Fatal(err)
	}
	if secondCount != 1 {
		t.Fatalf("expected second scan to import recovered telemetry event, got %d", secondCount)
	}

	cost, input, cached, _, output, err = database.QueryPeriodStatsAll("", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if input != 116540 || cached != 49024 || output != 714 {
		t.Errorf("unexpected recovered token totals: input=%d cached=%d output=%d", input, cached, output)
	}
	if cost <= 0 {
		t.Errorf("expected recovered Codex cost to remain non-zero, got %f", cost)
	}

	thirdCount, err := s.Scan(nil)
	if err != nil {
		t.Fatal(err)
	}
	if thirdCount != 0 {
		t.Fatalf("expected third scan to be incremental, got %d", thirdCount)
	}
}

func createCodexState(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, err = conn.Exec(`
		CREATE TABLE threads (
			id TEXT PRIMARY KEY,
			model TEXT,
			cwd TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func createCodexLogs(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, err = conn.Exec(`
		CREATE TABLE logs (
			id INTEGER PRIMARY KEY,
			ts INTEGER NOT NULL,
			ts_nanos INTEGER NOT NULL,
			target TEXT NOT NULL,
			feedback_log_body TEXT
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func insertThread(t *testing.T, path string, id string, model string, cwd string) {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, err = conn.Exec("INSERT INTO threads (id, model, cwd) VALUES (?, ?, ?)", id, model, cwd)
	if err != nil {
		t.Fatal(err)
	}
}

func insertLog(t *testing.T, path string, id int64, body string) {
	t.Helper()
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, err = conn.Exec(
		"INSERT INTO logs (id, ts, ts_nanos, target, feedback_log_body) VALUES (?, ?, ?, ?, ?)",
		id, time.Date(2026, 5, 7, 11, 13, int(id), 0, time.UTC).Unix(), int64(0), "codex_otel.log_only", body,
	)
	if err != nil {
		t.Fatal(err)
	}
}
