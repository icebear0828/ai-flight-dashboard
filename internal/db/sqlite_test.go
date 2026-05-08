package db_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
)

func TestDBInsert(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	u := model.TokenUsage{
		Source:       "Test",
		Model:        "test-model",
		InputTokens:  100,
		CachedTokens: 20,
		OutputTokens: 50,
	}

	err = database.InsertUsage(u, 1.25, "test-device")
	if err != nil {
		t.Errorf("failed to insert usage: %v", err)
	}
}

func TestScanOffsets(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Default offset should be 0
	offset, err := database.GetOffset("/some/file.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if offset != 0 {
		t.Errorf("expected 0, got %d", offset)
	}

	// Set and read back
	err = database.SetOffset("/some/file.jsonl", 12345)
	if err != nil {
		t.Fatal(err)
	}
	offset, err = database.GetOffset("/some/file.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if offset != 12345 {
		t.Errorf("expected 12345, got %d", offset)
	}

	// Upsert (update existing)
	err = database.SetOffset("/some/file.jsonl", 99999)
	if err != nil {
		t.Fatal(err)
	}
	offset, _ = database.GetOffset("/some/file.jsonl")
	if offset != 99999 {
		t.Errorf("expected 99999, got %d", offset)
	}
}

func TestInsertUsageWithTime(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ts := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	u := model.TokenUsage{
		Source:       "Claude Code",
		Model:        "claude-opus-4-7",
		InputTokens:  1000,
		CachedTokens: 500,
		OutputTokens: 200,
	}

	err = database.InsertUsageWithTime(u, 0.50, ts, "/path/to/file.jsonl", "my-mac")
	if err != nil {
		t.Fatal(err)
	}

	// Verify it's queryable
	cost, _, _, _, _, err := database.QueryPeriodStatsAll("", "")
	if err != nil {
		t.Fatal(err)
	}
	if cost < 0.49 || cost > 0.51 {
		t.Errorf("expected ~0.50, got %f", cost)
	}
}

func TestQueryCostSince(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()

	// Insert old record (2 days ago)
	old := model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50}
	database.InsertUsageWithTime(old, 1.00, now.Add(-48*time.Hour), "/a.jsonl", "dev1")

	// Insert recent record (30 min ago)
	recent := model.TokenUsage{Source: "Claude Code", Model: "m2", InputTokens: 200, OutputTokens: 100}
	database.InsertUsageWithTime(recent, 2.00, now.Add(-30*time.Minute), "/b.jsonl", "dev1")

	// Insert very recent (5 min ago) on different device
	veryRecent := model.TokenUsage{Source: "Gemini CLI", Model: "m3", InputTokens: 300, OutputTokens: 150}
	database.InsertUsageWithTime(veryRecent, 3.00, now.Add(-5*time.Minute), "/c.jsonl", "dev2")

	// Last 1 hour, all devices = 5.00
	cost, _, _, _, _, _ := database.QueryPeriodStatsSince(now.Add(-1*time.Hour), "", "")
	if cost < 4.99 || cost > 5.01 {
		t.Errorf("last 1h all: expected ~5.00, got %f", cost)
	}

	// Last 1 hour, dev1 only = 2.00
	cost, _, _, _, _, _ = database.QueryPeriodStatsSince(now.Add(-1*time.Hour), "dev1", "")
	if cost < 1.99 || cost > 2.01 {
		t.Errorf("last 1h dev1: expected ~2.00, got %f", cost)
	}

	// Cumulative all = 6.00
	cost, _, _, _, _, _ = database.QueryPeriodStatsAll("", "")
	if cost < 5.99 || cost > 6.01 {
		t.Errorf("cumulative: expected ~6.00, got %f", cost)
	}

	// Cumulative dev2 = 3.00
	cost, _, _, _, _, _ = database.QueryPeriodStatsAll("dev2", "")
	if cost < 2.99 || cost > 3.01 {
		t.Errorf("cumulative dev2: expected ~3.00, got %f", cost)
	}
}

func TestQueryStatsSince(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()

	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 500, OutputTokens: 200},
		1.50, now.Add(-10*time.Minute), "/a.jsonl", "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 2000, CachedTokens: 1000, OutputTokens: 400},
		3.00, now.Add(-5*time.Minute), "/b.jsonl", "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 500, CachedTokens: 0, OutputTokens: 100},
		0.80, now.Add(-1*time.Minute), "/c.jsonl", "local",
	)

	stats, err := database.QueryStatsSince(now.Add(-1*time.Hour), "", "")
	if err != nil {
		t.Fatal(err)
	}

	if len(stats) != 2 {
		t.Fatalf("expected 2 model groups, got %d", len(stats))
	}

	// Stats should be sorted by cost descending
	if stats[0].Model != "claude-opus-4-7" {
		t.Errorf("expected first model claude-opus-4-7, got %s", stats[0].Model)
	}
	if stats[0].TotalCost < 4.49 || stats[0].TotalCost > 4.51 {
		t.Errorf("expected claude cost ~4.50, got %f", stats[0].TotalCost)
	}
	if stats[0].Events != 2 {
		t.Errorf("expected 2 events, got %d", stats[0].Events)
	}
}

func TestQuerySinceHandlesMixedTimestampPrecision(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	err = database.RawExec(
		"INSERT INTO usage_records (log_timestamp, source, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"2026-05-07T11:13:02Z", "Claude Code", "claude-opus-4-7", 200, 0, 0, 20, 2.00, "/old.jsonl", "local",
	)
	if err != nil {
		t.Fatal(err)
	}
	err = database.RawExec(
		"INSERT INTO usage_records (log_timestamp, source, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"2026-05-07T11:13:03.316Z", "Gemini CLI", "gemini-2.5-pro", 100, 0, 0, 10, 1.00, "/new.jsonl", "local",
	)
	if err != nil {
		t.Fatal(err)
	}

	since := time.Date(2026, 5, 7, 11, 13, 3, 0, time.UTC)
	cost, input, _, _, output, err := database.QueryPeriodStatsSince(since, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if cost != 1.00 || input != 100 || output != 10 {
		t.Fatalf("expected fractional same-second row only, got cost=%f input=%d output=%d", cost, input, output)
	}

	records, err := database.QuerySyncRecordsSince(time.Now().UTC().Add(-1 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected sync records to use updated_at cursor, got %+v", records)
	}
	var foundGemini bool
	for _, record := range records {
		if record.Timestamp.IsZero() || record.UpdatedAt.IsZero() {
			t.Fatalf("expected parsed sync timestamps, got %+v", records)
		}
		if record.Source == "Gemini CLI" {
			foundGemini = true
		}
	}
	if !foundGemini {
		t.Fatalf("expected parsed fractional Gemini sync record, got %+v", records)
	}
}

func TestSupersededRowsAreExcludedFromDefaultQueries(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	path := "/Users/c/.gemini/tmp/wiki/chats/session.jsonl"
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-3.1-pro-preview", InputTokens: 1000, OutputTokens: 300},
		1.00, now, path, "local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-3.1-pro-preview", InputTokens: 2000, OutputTokens: 400, UUID: "gemini:active"},
		2.00, now.Add(time.Second), path, "local",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-3.1-pro-preview", InputTokens: 3000, OutputTokens: 500},
		3.00, now, "/Users/c/.gemini/tmp/old/chats/session.jsonl", "old-device",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SupersedeLegacyUsageBySourceFilePathsAndDevices("Gemini CLI", []string{path, "/Users/c/.gemini/tmp/old/chats/session.jsonl"}, []string{"local", "old-device"}); err != nil {
		t.Fatal(err)
	}

	cost, input, _, _, output, err := database.QueryPeriodStatsAll("", "Gemini CLI")
	if err != nil {
		t.Fatal(err)
	}
	if cost != 2.00 || input != 2000 || output != 400 {
		t.Fatalf("expected only active Gemini row in stats, got cost=%f input=%d output=%d", cost, input, output)
	}

	devices, err := database.QueryDevices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0] != "local" {
		t.Fatalf("expected only active local device, got %+v", devices)
	}

	usageRecords, err := database.QueryUsageRecords(time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(usageRecords) != 1 || usageRecords[0].InputTokens != 2000 {
		t.Fatalf("expected one active usage record, got %+v", usageRecords)
	}

	syncRecords, err := database.QuerySyncRecordsSince(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(syncRecords) != 3 {
		t.Fatalf("expected active and superseded sync records, got %+v", syncRecords)
	}
	var active, superseded int
	for _, r := range syncRecords {
		if r.Superseded {
			superseded++
		} else {
			active++
		}
		if r.UpdatedAt.IsZero() {
			t.Fatalf("expected sync record updated_at, got %+v", r)
		}
	}
	if active != 1 || superseded != 2 {
		t.Fatalf("expected 1 active and 2 superseded sync records, active=%d superseded=%d records=%+v", active, superseded, syncRecords)
	}
}

func TestQueryStatsSourceFilters(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", Project: "api", InputTokens: 1000, OutputTokens: 200},
		1.50, now, "/claude.jsonl", "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Codex", Model: "gpt-5.5", Project: "dashboard", InputTokens: 2000, CachedTokens: 1500, OutputTokens: 300},
		2.50, now, "/codex.sqlite", "local",
	)

	stats, err := database.QueryStatsSince(time.Time{}, "", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Source != "Codex" || stats[0].Model != "gpt-5.5" {
		t.Fatalf("expected only Codex model stats, got %+v", stats)
	}

	projects, err := database.QueryProjectStatsSince(time.Time{}, "", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Project != "dashboard" {
		t.Fatalf("expected only Codex project stats, got %+v", projects)
	}
}

func TestLegacyDatabaseMigratesStatsColumnsAndIndexes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`
		CREATE TABLE usage_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			log_timestamp DATETIME,
			source TEXT NOT NULL,
			model TEXT NOT NULL,
			project TEXT DEFAULT 'Default',
			input_tokens INTEGER NOT NULL,
			cached_tokens INTEGER NOT NULL,
			cache_creation_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER NOT NULL,
			cost_usd REAL NOT NULL,
			file_path TEXT DEFAULT '',
			device_id TEXT DEFAULT 'local',
			uuid TEXT
		);
		CREATE TABLE scan_offsets (
			file_path TEXT PRIMARY KEY,
			byte_offset INTEGER NOT NULL
		);
		INSERT INTO usage_records (log_timestamp, source, model, project, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id)
		VALUES ('2026-05-07T11:13:03Z', 'Codex', 'gpt-5.5', 'legacy-win', 1000, 100, 0, 200, 1.25, 'C:\Users\c\.codex\logs_2.sqlite', 'windows-box');
	`)
	if err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := db.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	cost, input, cached, _, output, err := database.QueryPeriodStatsAll("windows-box", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if cost != 1.25 || input != 1000 || cached != 100 || output != 200 {
		t.Fatalf("expected migrated legacy Codex row in stats, got cost=%f input=%d cached=%d output=%d", cost, input, cached, output)
	}

	stats, err := database.QueryStatsSince(time.Time{}, "windows-box", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Source != "Codex" {
		t.Fatalf("expected all-time Codex stats after migration, got %+v", stats)
	}

	check, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	for _, indexName := range []string{
		"idx_usage_active_source_log_jd",
		"idx_usage_active_device_source_log_jd",
		"idx_usage_active_source_model",
		"idx_usage_active_source_project",
	} {
		var found string
		err := check.QueryRow("SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?", indexName).Scan(&found)
		if err != nil {
			t.Fatalf("expected migrated index %s: %v", indexName, err)
		}
	}
}

func TestResetOffsetsLike(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	database.SetOffset("/Users/c/.gemini/tmp/wiki/chats/session.jsonl", 100)
	database.SetOffset("/Users/c/.claude/projects/-Users-c-token/session.jsonl", 200)

	changed, err := database.ResetOffsetsLike("%/.gemini/tmp/%")
	if err != nil {
		t.Fatal(err)
	}
	if changed != 1 {
		t.Fatalf("expected 1 reset offset, got %d", changed)
	}

	geminiOffset, err := database.GetOffset("/Users/c/.gemini/tmp/wiki/chats/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	claudeOffset, err := database.GetOffset("/Users/c/.claude/projects/-Users-c-token/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if geminiOffset != 0 || claudeOffset != 200 {
		t.Fatalf("unexpected offsets: gemini=%d claude=%d", geminiOffset, claudeOffset)
	}
}

func TestResetOffset(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	database.SetOffset("/Users/c/.gemini/tmp/wiki/chats/session.jsonl", 100)
	database.SetOffset("/Users/c/.gemini/tmp/wiki/chats/other.jsonl", 200)

	changed, err := database.ResetOffset("/Users/c/.gemini/tmp/wiki/chats/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if changed != 1 {
		t.Fatalf("expected 1 reset offset, got %d", changed)
	}

	geminiOffset, err := database.GetOffset("/Users/c/.gemini/tmp/wiki/chats/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	otherOffset, err := database.GetOffset("/Users/c/.gemini/tmp/wiki/chats/other.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if geminiOffset != 0 || otherOffset != 200 {
		t.Fatalf("unexpected offsets: gemini=%d other=%d", geminiOffset, otherOffset)
	}
}

func TestSupersedeLegacyUsageBySourceFilePathsAndDevices(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	replayableGeminiPath := "/Users/c/.gemini/tmp/wiki/chats/session.jsonl"
	missingGeminiPath := "/Users/c/.gemini/tmp/wiki/chats/missing.jsonl"
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-3.1-pro-preview", InputTokens: 1000, OutputTokens: 300},
		1.00, now, replayableGeminiPath, "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-3.1-pro-preview", InputTokens: 2000, OutputTokens: 400},
		2.00, now, replayableGeminiPath, "remote-mac",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-3.1-pro-preview", InputTokens: 3000, OutputTokens: 500},
		3.00, now, missingGeminiPath, "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 4000, OutputTokens: 600},
		4.00, now, replayableGeminiPath, "local",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-3.1-pro-preview", InputTokens: 5000, OutputTokens: 700, UUID: "gemini:new"},
		5.00, now, replayableGeminiPath, "local",
	)

	superseded, err := database.SupersedeLegacyUsageBySourceFilePathsAndDevices("Gemini CLI", []string{replayableGeminiPath}, []string{"local", "test-device"})
	if err != nil {
		t.Fatal(err)
	}
	if superseded != 1 {
		t.Fatalf("expected 1 local replayable Gemini row superseded, got %d", superseded)
	}

	geminiCost, geminiIn, _, _, geminiOut, err := database.QueryPeriodStatsAll("", "Gemini CLI")
	if err != nil {
		t.Fatal(err)
	}
	if geminiCost != 10.00 || geminiIn != 10000 || geminiOut != 1600 {
		t.Fatalf("unexpected remaining Gemini stats: cost=%f input=%d output=%d", geminiCost, geminiIn, geminiOut)
	}
	remoteGeminiCost, remoteGeminiIn, _, _, remoteGeminiOut, err := database.QueryPeriodStatsAll("remote-mac", "Gemini CLI")
	if err != nil {
		t.Fatal(err)
	}
	if remoteGeminiCost != 2.00 || remoteGeminiIn != 2000 || remoteGeminiOut != 400 {
		t.Fatalf("unexpected remote Gemini stats: cost=%f input=%d output=%d", remoteGeminiCost, remoteGeminiIn, remoteGeminiOut)
	}

	claudeCost, claudeIn, _, _, claudeOut, err := database.QueryPeriodStatsAll("", "Claude Code")
	if err != nil {
		t.Fatal(err)
	}
	if claudeCost != 4.00 || claudeIn != 4000 || claudeOut != 600 {
		t.Fatalf("unexpected Claude stats: cost=%f input=%d output=%d", claudeCost, claudeIn, claudeOut)
	}
}

func TestInsertUsageWithUUIDDoesNotHitLegacyDedupIndex(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ts := time.Date(2026, 4, 15, 7, 33, 41, 342_000_000, time.UTC)
	first := model.TokenUsage{
		Source:              "Claude Code",
		Model:               "claude-opus-4-6",
		InputTokens:         78442,
		CachedTokens:        77912,
		CacheCreationTokens: 529,
		OutputTokens:        204,
		UUID:                "uuid-1",
	}
	second := first
	second.UUID = "uuid-2"

	if err := database.InsertUsageWithTime(first, 1.00, ts, "/same-second.jsonl", "local"); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(second, 1.00, ts.Add(190*time.Millisecond), "/same-second.jsonl", "local"); err != nil {
		t.Fatal(err)
	}

	stats, err := database.QueryStatsSince(time.Time{}, "", "Claude Code")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].Events != 2 {
		t.Fatalf("expected both UUID records to be retained, got %+v", stats)
	}
}

func TestInsertUsageWithUUIDIsScopedByDevice(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	ts := time.Date(2026, 5, 7, 11, 13, 3, 0, time.UTC)
	u := model.TokenUsage{
		Source:       "Codex",
		Model:        "gpt-5.5",
		InputTokens:  1000,
		OutputTokens: 100,
		UUID:         "codex:1",
	}
	if err := database.InsertUsageWithTime(u, 1.00, ts, "/mac/logs.sqlite", "mac"); err != nil {
		t.Fatal(err)
	}
	remote := u
	remote.InputTokens = 2000
	if err := database.InsertUsageWithTime(remote, 2.00, ts, "/linux/logs.sqlite", "linux"); err != nil {
		t.Fatal(err)
	}

	cost, input, _, _, output, err := database.QueryPeriodStatsAll("", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if cost != 3.00 || input != 3000 || output != 200 {
		t.Fatalf("expected same UUID on different devices to retain both rows, cost=%f input=%d output=%d", cost, input, output)
	}

	update := u
	update.OutputTokens = 150
	if err := database.InsertUsageWithTime(update, 1.50, ts.Add(time.Second), "/mac/logs.sqlite", "mac"); err != nil {
		t.Fatal(err)
	}
	macCost, macInput, _, _, macOutput, err := database.QueryPeriodStatsAll("mac", "Codex")
	if err != nil {
		t.Fatal(err)
	}
	if macCost != 1.50 || macInput != 1000 || macOutput != 150 {
		t.Fatalf("expected same device UUID to upsert, cost=%f input=%d output=%d", macCost, macInput, macOutput)
	}
}

func TestExistingDatabaseMigrationAddsUpdatedAt(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`
		CREATE TABLE usage_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			log_timestamp DATETIME,
			source TEXT NOT NULL,
			model TEXT NOT NULL,
			project TEXT DEFAULT 'Default',
			input_tokens INTEGER NOT NULL,
			cached_tokens INTEGER NOT NULL,
			cache_creation_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER NOT NULL,
			cost_usd REAL NOT NULL,
			file_path TEXT DEFAULT '',
			device_id TEXT DEFAULT 'local',
			uuid TEXT,
			superseded INTEGER DEFAULT 0
		);
		CREATE TABLE scan_offsets (file_path TEXT PRIMARY KEY, byte_offset INTEGER NOT NULL);
		CREATE TABLE known_dirs (dir_path TEXT PRIMARY KEY, last_seen DATETIME DEFAULT CURRENT_TIMESTAMP);
		CREATE TABLE device_aliases (device_id TEXT PRIMARY KEY, display_name TEXT NOT NULL);
		INSERT INTO usage_records (log_timestamp, source, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id)
		VALUES ('2026-01-01T00:00:00Z', 'Claude Code', 'claude-opus-4-7', 100, 0, 0, 10, 1.00, '/old.jsonl', 'mac');
	`); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := db.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Codex", Model: "gpt-5.5", InputTokens: 200, OutputTokens: 20, UUID: "codex:1"},
		2.00, time.Now().UTC(), "/codex.sqlite", "mac",
	); err != nil {
		t.Fatal(err)
	}
	records, err := database.QuerySyncRecordsSince(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected migrated legacy and new sync records, got %+v", records)
	}
	for _, r := range records {
		if r.UpdatedAt.IsZero() {
			t.Fatalf("expected migrated updated_at, got %+v", records)
		}
	}
}

func TestSyncUsesUpdatedAtAndAppliesSupersededMarkers(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	oldLogTS := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	path := "/Users/c/.gemini/tmp/wiki/chats/session.jsonl"
	legacy := model.TokenUsage{Source: "Gemini CLI", Model: "gemini-3.1-pro-preview", InputTokens: 1000, OutputTokens: 100, Timestamp: oldLogTS}
	if err := database.InsertUsageWithTime(legacy, 1.00, oldLogTS, path, "mac"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SupersedeLegacyUsageBySourceFilePathsAndDevices("Gemini CLI", []string{path}, []string{"mac"}); err != nil {
		t.Fatal(err)
	}

	records, err := database.QuerySyncRecordsSince(time.Now().UTC().Add(-1 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !records[0].Superseded || records[0].UpdatedAt.IsZero() {
		t.Fatalf("expected historical superseded record to sync by updated_at, got %+v", records)
	}

	peer, err := db.New(filepath.Join(t.TempDir(), "peer.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	if err := peer.InsertUsageWithTime(legacy, 1.00, oldLogTS, path, "mac"); err != nil {
		t.Fatal(err)
	}
	if err := peer.UpsertSyncRecord(records[0]); err != nil {
		t.Fatal(err)
	}
	cost, input, _, _, _, err := peer.QueryPeriodStatsAll("mac", "Gemini CLI")
	if err != nil {
		t.Fatal(err)
	}
	if cost != 0 || input != 0 {
		t.Fatalf("expected sync superseded marker to hide peer legacy row, cost=%f input=%d", cost, input)
	}
}

func TestQuerySyncRecordsPageUsesCursor(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 100, OutputTokens: 10, UUID: "sync-page-1"},
		1.00, now, "/one.jsonl", "mac",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 200, OutputTokens: 20, UUID: "sync-page-2"},
		2.00, now.Add(time.Second), "/two.jsonl", "mac",
	); err != nil {
		t.Fatal(err)
	}

	first, err := database.QuerySyncRecordsPage(time.Time{}, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Records) != 1 || !first.HasMore || first.NextUpdatedAt.IsZero() || first.NextAfterID == 0 {
		t.Fatalf("unexpected first page: %+v", first)
	}
	second, err := database.QuerySyncRecordsPage(first.NextUpdatedAt, first.NextAfterID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Records) != 1 || second.Records[0].UUID == first.Records[0].UUID {
		t.Fatalf("expected second page to advance cursor, first=%+v second=%+v", first, second)
	}
}

func TestQueryDevices(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50},
		1.00, now, "/a.jsonl", "mac-pro",
	)
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50},
		1.00, now, "/b.jsonl", "linux-server",
	)

	devices, err := database.QueryDevices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
}

func TestInsertDedup(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	u := model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 500, OutputTokens: 200, UUID: "test-uuid-123"}

	// Insert same record twice — second should UPSERT by UUID
	err = database.InsertUsageWithTime(u, 1.50, now, "/a.jsonl", "local")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate streaming update: output tokens increased, but same UUID
	u2 := u
	u2.OutputTokens = 300
	err = database.InsertUsageWithTime(u2, 2.00, now, "/a.jsonl", "local")
	if err != nil {
		t.Fatal(err)
	}

	// Should only have 1 record, not 2
	cost, _, _, _, _, _ := database.QueryPeriodStatsAll("", "")
	if cost < 1.99 || cost > 2.01 {
		t.Errorf("expected ~2.00 (upserted record), got %f (dedup failed)", cost)
	}
}

func TestInsertDedup_DifferentDevices(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	u := model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, OutputTokens: 50}

	// Same usage but different devices — should both be kept
	database.InsertUsageWithTime(u, 1.00, now, "/a.jsonl", "mac")
	database.InsertUsageWithTime(u, 1.00, now, "/a.jsonl", "linux")

	cost, _, _, _, _, _ := database.QueryPeriodStatsAll("", "")
	if cost < 1.99 || cost > 2.01 {
		t.Errorf("expected ~2.00 (2 different devices), got %f", cost)
	}
}

func TestQueryCacheSavings(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()

	// Record 1: 1000 input (800 cached), 200 output
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 1000, CachedTokens: 800, OutputTokens: 200},
		1.50, now.Add(-10*time.Minute), "/a.jsonl", "local",
	)
	// Record 2: 500 input (0 cached), 100 output
	database.InsertUsageWithTime(
		model.TokenUsage{Source: "Gemini CLI", Model: "gemini-2.5-pro", InputTokens: 500, CachedTokens: 0, OutputTokens: 100},
		0.80, now.Add(-5*time.Minute), "/b.jsonl", "local",
	)

	records, err := database.QueryUsageRecords(now.Add(-1*time.Hour), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	// Verify fields
	totalCached := 0
	totalInput := 0
	for _, r := range records {
		totalCached += r.CachedTokens
		totalInput += r.InputTokens
	}
	if totalCached != 800 {
		t.Errorf("expected total cached 800, got %d", totalCached)
	}
	if totalInput != 1500 {
		t.Errorf("expected total input 1500, got %d", totalInput)
	}
}

func TestDeduplicateExisting(t *testing.T) {
	database, err := db.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	u := model.TokenUsage{Source: "Claude Code", Model: "m1", InputTokens: 100, CachedTokens: 0, OutputTokens: 50}

	// Simulate legacy data: drop the unique index, force-insert 3 identical rows, then re-add it.
	database.RawExec("DROP INDEX IF EXISTS idx_usage_dedup")
	for i := 0; i < 3; i++ {
		database.RawExec(
			"INSERT INTO usage_records (log_timestamp, source, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			now.Format(time.RFC3339), u.Source, u.Model, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens, 1.00, "/a.jsonl", "local",
		)
	}

	// Verify we have 3 records before dedup
	cost, _, _, _, _, _ := database.QueryPeriodStatsAll("", "")
	if cost < 2.99 {
		t.Fatalf("setup: expected ~3.00 before dedup, got %f", cost)
	}

	// Run dedup
	removed, err := database.DeduplicateExisting()
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Errorf("expected 2 duplicates removed, got %d", removed)
	}

	// After dedup, should be exactly 1 record
	cost, _, _, _, _, _ = database.QueryPeriodStatsAll("", "")
	if cost < 0.99 || cost > 1.01 {
		t.Errorf("expected ~1.00 after dedup, got %f", cost)
	}
}
