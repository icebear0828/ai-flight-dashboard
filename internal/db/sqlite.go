package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"ai-flight-dashboard/internal/model"
	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

const logTimestampLayout = "2006-01-02T15:04:05.000000000Z"
const activeUsagePredicate = "COALESCE(superseded, 0) = 0"
const sourceTotalsSummaryMigrationKey = "migration:source-totals-summary-v2"

var errInvalidLogTimestamp = errors.New("invalid log timestamp")

type ModelStat struct {
	Model               string
	Source              string
	Events              int
	InputTokens         int
	CachedTokens        int
	CacheCreationTokens int
	OutputTokens        int
	TotalCost           float64
}

type SourceTotalStat struct {
	Source              string
	Events              int
	InputTokens         int
	CachedTokens        int
	CacheCreationTokens int
	OutputTokens        int
	TotalCost           float64
}

type PeriodStatsWindow struct {
	Label string
	Since time.Time
}

type PeriodStatsBucket struct {
	Label               string
	Cost                float64
	InputTokens         int
	CachedTokens        int
	CacheCreationTokens int
	OutputTokens        int
}

func New(dsn string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	// Enforce single connection pool to prevent concurrent write locks in WAL mode
	conn.SetMaxOpenConns(1)

	// WAL mode for concurrent read/write
	conn.Exec("PRAGMA journal_mode=WAL")
	conn.Exec("PRAGMA busy_timeout=5000")

	if err := initSchema(conn); err != nil {
		return nil, err
	}

	return &DB{conn: conn}, nil
}

func initSchema(conn *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS usage_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
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

	CREATE TABLE IF NOT EXISTS scan_offsets (
		file_path TEXT PRIMARY KEY,
		byte_offset INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS known_dirs (
		dir_path TEXT PRIMARY KEY,
		last_seen DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS device_aliases (
		device_id TEXT PRIMARY KEY,
		display_name TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS usage_source_totals (
		device_id TEXT NOT NULL,
		source TEXT NOT NULL,
		events INTEGER NOT NULL,
		input_tokens INTEGER NOT NULL,
		cached_tokens INTEGER NOT NULL,
		cache_creation_tokens INTEGER NOT NULL,
		output_tokens INTEGER NOT NULL,
		cost_usd REAL NOT NULL,
		PRIMARY KEY (device_id, source)
	);
	`
	_, err := conn.Exec(schema)
	if err != nil {
		return err
	}

	// Lightweight migrations for existing tables. Index creation intentionally
	// runs after ALTER TABLE so old databases missing newer columns still open.
	for _, stmt := range []string{
		"ALTER TABLE usage_records ADD COLUMN device_id TEXT DEFAULT 'local'",
		"ALTER TABLE usage_records ADD COLUMN cache_creation_tokens INTEGER DEFAULT 0",
		"ALTER TABLE usage_records ADD COLUMN uuid TEXT",
		"ALTER TABLE usage_records ADD COLUMN project TEXT DEFAULT 'Default'",
		"ALTER TABLE usage_records ADD COLUMN superseded INTEGER DEFAULT 0",
		"ALTER TABLE usage_records ADD COLUMN updated_at DATETIME",
	} {
		if err := execMigration(conn, stmt, true); err != nil {
			return err
		}
	}
	for _, stmt := range []string{
		"UPDATE usage_records SET updated_at = COALESCE(timestamp, log_timestamp, CURRENT_TIMESTAMP) WHERE updated_at IS NULL OR updated_at = ''",
		"DROP INDEX IF EXISTS idx_usage_uuid",
		"CREATE INDEX IF NOT EXISTS idx_usage_log_ts ON usage_records(log_timestamp)",
		"CREATE INDEX IF NOT EXISTS idx_usage_device ON usage_records(device_id)",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_device_uuid ON usage_records(device_id, uuid) WHERE uuid IS NOT NULL AND uuid != ''",
		"CREATE INDEX IF NOT EXISTS idx_usage_active_source_log_ts ON usage_records(source, log_timestamp) WHERE COALESCE(superseded, 0) = 0",
		"CREATE INDEX IF NOT EXISTS idx_usage_active_device_source_log_ts ON usage_records(device_id, source, log_timestamp) WHERE COALESCE(superseded, 0) = 0",
		"CREATE INDEX IF NOT EXISTS idx_usage_active_source_log_jd ON usage_records(source, julianday(log_timestamp)) WHERE COALESCE(superseded, 0) = 0",
		"CREATE INDEX IF NOT EXISTS idx_usage_active_device_source_log_jd ON usage_records(device_id, source, julianday(log_timestamp)) WHERE COALESCE(superseded, 0) = 0",
		"CREATE INDEX IF NOT EXISTS idx_usage_active_source_model ON usage_records(source, model) WHERE COALESCE(superseded, 0) = 0",
		"CREATE INDEX IF NOT EXISTS idx_usage_active_source_project ON usage_records(source, project) WHERE COALESCE(superseded, 0) = 0",
		"CREATE INDEX IF NOT EXISTS idx_usage_active_updated_at ON usage_records(julianday(COALESCE(updated_at, timestamp, log_timestamp)))",
	} {
		if err := execMigration(conn, stmt, false); err != nil {
			return err
		}
	}
	if err := ensurePartialDedupIndex(conn); err != nil {
		return err
	}
	if err := ensureSourceTotalsSummary(conn); err != nil {
		return err
	}

	return nil
}

func execMigration(conn *sql.DB, stmt string, ignoreDuplicateColumn bool) error {
	_, err := conn.Exec(stmt)
	if err == nil {
		return nil
	}
	if ignoreDuplicateColumn && strings.Contains(err.Error(), "duplicate column name") {
		return nil
	}
	return err
}

func ensurePartialDedupIndex(conn *sql.DB) error {
	const migrationKey = "migration:partial-dedup-index-v2"
	var done int64
	err := conn.QueryRow("SELECT byte_offset FROM scan_offsets WHERE file_path = ?", migrationKey).Scan(&done)
	if err == nil && done == 1 {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if _, err := conn.Exec("DROP INDEX IF EXISTS idx_usage_dedup"); err != nil {
		return err
	}
	if _, err := conn.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_dedup ON usage_records(log_timestamp, file_path, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, device_id) WHERE (uuid IS NULL OR uuid = '') AND COALESCE(superseded, 0) = 0"); err != nil {
		return err
	}
	_, err = conn.Exec(`INSERT INTO scan_offsets (file_path, byte_offset) VALUES (?, 1)
	ON CONFLICT(file_path) DO UPDATE SET byte_offset = excluded.byte_offset`, migrationKey)
	return err
}

func ensureSourceTotalsSummary(conn *sql.DB) error {
	if err := execMigration(conn, "CREATE INDEX IF NOT EXISTS idx_usage_source_totals_source ON usage_source_totals(source)", false); err != nil {
		return err
	}

	var done int64
	err := conn.QueryRow("SELECT byte_offset FROM scan_offsets WHERE file_path = ?", sourceTotalsSummaryMigrationKey).Scan(&done)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	recreate := err == sql.ErrNoRows || done != 1
	if err := ensureSourceTotalsSummaryTriggers(conn, recreate); err != nil {
		return err
	}
	if !recreate {
		return nil
	}
	if err := rebuildSourceTotalsSummary(conn); err != nil {
		return err
	}
	_, err = conn.Exec(`INSERT INTO scan_offsets (file_path, byte_offset) VALUES (?, 1)
	ON CONFLICT(file_path) DO UPDATE SET byte_offset = excluded.byte_offset`, sourceTotalsSummaryMigrationKey)
	return err
}

func ensureSourceTotalsSummaryTriggers(conn *sql.DB, recreate bool) error {
	if recreate {
		for _, stmt := range []string{
			"DROP TRIGGER IF EXISTS trg_usage_source_totals_insert",
			"DROP TRIGGER IF EXISTS trg_usage_source_totals_delete",
			"DROP TRIGGER IF EXISTS trg_usage_source_totals_update_remove",
			"DROP TRIGGER IF EXISTS trg_usage_source_totals_update_add",
		} {
			if err := execMigration(conn, stmt, false); err != nil {
				return err
			}
		}
	}

	for _, stmt := range []string{
		`CREATE TRIGGER IF NOT EXISTS trg_usage_source_totals_insert
		AFTER INSERT ON usage_records
		WHEN COALESCE(NEW.superseded, 0) = 0
		BEGIN
			INSERT INTO usage_source_totals (
				device_id, source, events, input_tokens, cached_tokens,
				cache_creation_tokens, output_tokens, cost_usd
			)
			VALUES (
				COALESCE(NEW.device_id, 'local'), NEW.source, 1,
				COALESCE(NEW.input_tokens, 0), COALESCE(NEW.cached_tokens, 0),
				COALESCE(NEW.cache_creation_tokens, 0), COALESCE(NEW.output_tokens, 0),
				COALESCE(NEW.cost_usd, 0)
			)
			ON CONFLICT(device_id, source) DO UPDATE SET
				events = usage_source_totals.events + excluded.events,
				input_tokens = usage_source_totals.input_tokens + excluded.input_tokens,
				cached_tokens = usage_source_totals.cached_tokens + excluded.cached_tokens,
				cache_creation_tokens = usage_source_totals.cache_creation_tokens + excluded.cache_creation_tokens,
				output_tokens = usage_source_totals.output_tokens + excluded.output_tokens,
				cost_usd = usage_source_totals.cost_usd + excluded.cost_usd;
		END`,
		`CREATE TRIGGER IF NOT EXISTS trg_usage_source_totals_delete
		AFTER DELETE ON usage_records
		WHEN COALESCE(OLD.superseded, 0) = 0
		BEGIN
			UPDATE usage_source_totals SET
				events = events - 1,
				input_tokens = input_tokens - COALESCE(OLD.input_tokens, 0),
				cached_tokens = cached_tokens - COALESCE(OLD.cached_tokens, 0),
				cache_creation_tokens = cache_creation_tokens - COALESCE(OLD.cache_creation_tokens, 0),
				output_tokens = output_tokens - COALESCE(OLD.output_tokens, 0),
				cost_usd = cost_usd - COALESCE(OLD.cost_usd, 0)
			WHERE device_id = COALESCE(OLD.device_id, 'local') AND source = OLD.source;
			DELETE FROM usage_source_totals WHERE events <= 0;
		END`,
		`CREATE TRIGGER IF NOT EXISTS trg_usage_source_totals_update_remove
		AFTER UPDATE ON usage_records
		WHEN COALESCE(OLD.superseded, 0) = 0
			AND (
				COALESCE(NEW.superseded, 0) != 0
				OR COALESCE(OLD.device_id, 'local') IS NOT COALESCE(NEW.device_id, 'local')
				OR OLD.source IS NOT NEW.source
				OR COALESCE(OLD.input_tokens, 0) IS NOT COALESCE(NEW.input_tokens, 0)
				OR COALESCE(OLD.cached_tokens, 0) IS NOT COALESCE(NEW.cached_tokens, 0)
				OR COALESCE(OLD.cache_creation_tokens, 0) IS NOT COALESCE(NEW.cache_creation_tokens, 0)
				OR COALESCE(OLD.output_tokens, 0) IS NOT COALESCE(NEW.output_tokens, 0)
				OR COALESCE(OLD.cost_usd, 0) IS NOT COALESCE(NEW.cost_usd, 0)
			)
		BEGIN
			UPDATE usage_source_totals SET
				events = events - 1,
				input_tokens = input_tokens - COALESCE(OLD.input_tokens, 0),
				cached_tokens = cached_tokens - COALESCE(OLD.cached_tokens, 0),
				cache_creation_tokens = cache_creation_tokens - COALESCE(OLD.cache_creation_tokens, 0),
				output_tokens = output_tokens - COALESCE(OLD.output_tokens, 0),
				cost_usd = cost_usd - COALESCE(OLD.cost_usd, 0)
			WHERE device_id = COALESCE(OLD.device_id, 'local') AND source = OLD.source;
			DELETE FROM usage_source_totals WHERE events <= 0;
		END`,
		`CREATE TRIGGER IF NOT EXISTS trg_usage_source_totals_update_add
		AFTER UPDATE ON usage_records
		WHEN COALESCE(NEW.superseded, 0) = 0
			AND (
				COALESCE(OLD.superseded, 0) != 0
				OR COALESCE(OLD.device_id, 'local') IS NOT COALESCE(NEW.device_id, 'local')
				OR OLD.source IS NOT NEW.source
				OR COALESCE(OLD.input_tokens, 0) IS NOT COALESCE(NEW.input_tokens, 0)
				OR COALESCE(OLD.cached_tokens, 0) IS NOT COALESCE(NEW.cached_tokens, 0)
				OR COALESCE(OLD.cache_creation_tokens, 0) IS NOT COALESCE(NEW.cache_creation_tokens, 0)
				OR COALESCE(OLD.output_tokens, 0) IS NOT COALESCE(NEW.output_tokens, 0)
				OR COALESCE(OLD.cost_usd, 0) IS NOT COALESCE(NEW.cost_usd, 0)
			)
		BEGIN
			INSERT INTO usage_source_totals (
				device_id, source, events, input_tokens, cached_tokens,
				cache_creation_tokens, output_tokens, cost_usd
			)
			VALUES (
				COALESCE(NEW.device_id, 'local'), NEW.source, 1,
				COALESCE(NEW.input_tokens, 0), COALESCE(NEW.cached_tokens, 0),
				COALESCE(NEW.cache_creation_tokens, 0), COALESCE(NEW.output_tokens, 0),
				COALESCE(NEW.cost_usd, 0)
			)
			ON CONFLICT(device_id, source) DO UPDATE SET
				events = usage_source_totals.events + excluded.events,
				input_tokens = usage_source_totals.input_tokens + excluded.input_tokens,
				cached_tokens = usage_source_totals.cached_tokens + excluded.cached_tokens,
				cache_creation_tokens = usage_source_totals.cache_creation_tokens + excluded.cache_creation_tokens,
				output_tokens = usage_source_totals.output_tokens + excluded.output_tokens,
				cost_usd = usage_source_totals.cost_usd + excluded.cost_usd;
		END`,
	} {
		if err := execMigration(conn, stmt, false); err != nil {
			return err
		}
	}
	return nil
}

// RebuildSourceTotalsSummary rebuilds the materialized all-time source totals
// from usage_records. The raw usage table remains the source of truth.
func (d *DB) RebuildSourceTotalsSummary() error {
	return rebuildSourceTotalsSummary(d.conn)
}

func rebuildSourceTotalsSummary(conn *sql.DB) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM usage_source_totals"); err != nil {
		return err
	}
	_, err = tx.Exec(`
		INSERT INTO usage_source_totals (
			device_id, source, events, input_tokens, cached_tokens,
			cache_creation_tokens, output_tokens, cost_usd
		)
		SELECT
			COALESCE(device_id, 'local'), source, COUNT(*),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(cached_tokens), 0),
			COALESCE(SUM(cache_creation_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cost_usd), 0)
		FROM usage_records
		WHERE ` + activeUsagePredicate + `
		GROUP BY COALESCE(device_id, 'local'), source
	`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// InsertUsage inserts a usage record with current timestamp (for live watcher).
func (d *DB) InsertUsage(u model.TokenUsage, cost float64, deviceID string) error {
	ts := time.Now().UTC()
	if !u.Timestamp.IsZero() {
		ts = u.Timestamp
	}
	return d.InsertUsageWithTime(u, cost, ts, "", deviceID)
}

// InsertUsageWithTime inserts a usage record with an explicit log timestamp.
// If UUID is present, it will UPSERT (overwrite older states of the same generation).
// If UUID is empty, duplicate records are silently ignored.
func (d *DB) InsertUsageWithTime(u model.TokenUsage, cost float64, logTS time.Time, filePath string, deviceID string) error {
	return d.insertUsageWithTime(u, cost, logTS, filePath, deviceID, false)
}

func (d *DB) insertUsageWithTime(u model.TokenUsage, cost float64, logTS time.Time, filePath string, deviceID string, superseded bool) error {
	return d.insertUsageWithTimeUpdatedAt(u, cost, logTS, filePath, deviceID, time.Now().UTC(), superseded)
}

func (d *DB) insertUsageWithTimeUpdatedAt(u model.TokenUsage, cost float64, logTS time.Time, filePath string, deviceID string, updatedAtTS time.Time, superseded bool) error {
	if updatedAtTS.IsZero() {
		updatedAtTS = time.Now().UTC()
	}
	logTimestamp := formatLogTimestamp(logTS)
	updatedAt := formatLogTimestamp(updatedAtTS)
	supersededValue := 0
	if superseded {
		supersededValue = 1
	}
	if u.UUID != "" {
		query := `
		INSERT INTO usage_records (uuid, log_timestamp, updated_at, source, model, project, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id, superseded)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(device_id, uuid) WHERE uuid IS NOT NULL AND uuid != '' DO UPDATE SET
			source = excluded.source,
			model = excluded.model,
			log_timestamp = excluded.log_timestamp,
			updated_at = excluded.updated_at,
			project = excluded.project,
			input_tokens = excluded.input_tokens,
			cached_tokens = excluded.cached_tokens,
			cache_creation_tokens = excluded.cache_creation_tokens,
			output_tokens = excluded.output_tokens,
			cost_usd = excluded.cost_usd,
			file_path = CASE
				WHEN excluded.file_path != '' THEN excluded.file_path
				ELSE usage_records.file_path
			END,
			device_id = excluded.device_id,
			superseded = excluded.superseded
		`
		_, err := d.conn.Exec(query, u.UUID, logTimestamp, updatedAt, u.Source, u.Model, u.Project, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens, cost, filePath, deviceID, supersededValue)
		return err
	}

	query := `
	INSERT OR IGNORE INTO usage_records (log_timestamp, updated_at, source, model, project, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id, superseded)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := d.conn.Exec(query, logTimestamp, updatedAt, u.Source, u.Model, u.Project, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens, cost, filePath, deviceID, supersededValue)
	return err
}

// UpsertSyncRecord applies a LAN synchronization record, including tombstone-like
// superseded markers for legacy rows that do not have UUIDs.
func (d *DB) UpsertSyncRecord(r model.SyncRecord) error {
	deviceID := r.DeviceID
	if deviceID == "" {
		deviceID = r.TokenUsage.DeviceID
	}
	u := r.TokenUsage
	u.DeviceID = deviceID
	updatedAt := r.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	if r.Superseded {
		if u.UUID != "" {
			return d.insertUsageWithTimeUpdatedAt(u, r.CostUSD, u.Timestamp, r.FilePath, deviceID, updatedAt, true)
		}
		return d.supersedeOrInsertLegacySyncRecord(u, r.CostUSD, r.FilePath, deviceID, updatedAt)
	}
	return d.insertUsageWithTimeUpdatedAt(u, r.CostUSD, u.Timestamp, r.FilePath, deviceID, updatedAt, false)
}

func (d *DB) supersedeOrInsertLegacySyncRecord(u model.TokenUsage, cost float64, filePath string, deviceID string, updatedAtTS time.Time) error {
	logTimestamp := formatLogTimestamp(u.Timestamp)
	updatedAt := formatLogTimestamp(updatedAtTS)
	result, err := d.conn.Exec(`UPDATE usage_records SET superseded = 1, updated_at = ?
		WHERE log_timestamp = ? AND source = ? AND model = ? AND input_tokens = ? AND cached_tokens = ?
			AND cache_creation_tokens = ? AND output_tokens = ? AND file_path = ?
			AND device_id = ? AND (uuid IS NULL OR uuid = '')`,
		updatedAt, logTimestamp, u.Source, u.Model, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens, filePath, deviceID,
	)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil {
		return err
	} else if changed > 0 {
		return nil
	}
	return d.insertUsageWithTimeUpdatedAt(u, cost, u.Timestamp, filePath, deviceID, updatedAtTS, true)
}

func formatLogTimestamp(ts time.Time) string {
	return ts.UTC().Format(logTimestampLayout)
}

func parseLogTimestamp(value string) (time.Time, error) {
	for _, layout := range []string{
		logTimestampLayout,
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
	} {
		if ts, err := time.Parse(layout, value); err == nil {
			return ts, nil
		}
	}
	return time.Time{}, errInvalidLogTimestamp
}

// GetOffset returns the last scanned byte offset for a file. Returns 0 if not found.
func (d *DB) GetOffset(filePath string) (int64, error) {
	var offset int64
	err := d.conn.QueryRow("SELECT byte_offset FROM scan_offsets WHERE file_path = ?", filePath).Scan(&offset)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return offset, err
}

// SetOffset upserts the byte offset for a file.
func (d *DB) SetOffset(filePath string, offset int64) error {
	query := `INSERT INTO scan_offsets (file_path, byte_offset) VALUES (?, ?)
	ON CONFLICT(file_path) DO UPDATE SET byte_offset = excluded.byte_offset`
	_, err := d.conn.Exec(query, filePath, offset)
	return err
}

// ResetOffsetsLike resets scan offsets matching a SQL LIKE pattern so files are
// re-read by the incremental scanner. UUID/upsert dedup keeps the rescan safe.
func (d *DB) ResetOffsetsLike(pattern string) (int64, error) {
	result, err := d.conn.Exec("UPDATE scan_offsets SET byte_offset = 0 WHERE file_path LIKE ?", pattern)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ResetOffset resets the scan offset for one exact file path.
func (d *DB) ResetOffset(filePath string) (int64, error) {
	result, err := d.conn.Exec("UPDATE scan_offsets SET byte_offset = 0 WHERE file_path = ?", filePath)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// SupersedeLegacyUsageBySourceFilePathsAndDevices hides legacy no-UUID rows
// matching exact source, file path, and device IDs. Empty path/device lists are
// no-ops. Rows are retained for audit and recovery.
func (d *DB) SupersedeLegacyUsageBySourceFilePathsAndDevices(source string, filePaths []string, deviceIDs []string) (int64, error) {
	if source == "" || len(filePaths) == 0 || len(deviceIDs) == 0 {
		return 0, nil
	}

	seen := make(map[string]struct{})
	var changed int64
	for _, filePath := range filePaths {
		if filePath == "" {
			continue
		}
		for _, deviceID := range deviceIDs {
			key := filePath + "\x00" + deviceID
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result, err := d.conn.Exec("UPDATE usage_records SET superseded = 1, updated_at = ? WHERE source = ? AND file_path = ? AND device_id = ? AND (uuid IS NULL OR uuid = '') AND COALESCE(superseded, 0) = 0", formatLogTimestamp(time.Now().UTC()), source, filePath, deviceID)
			if err != nil {
				return changed, err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return changed, err
			}
			changed += n
		}
	}
	return changed, nil
}

// SupersedeUsageBySourceFilePathDeviceUUIDPrefix hides active rows from a
// superseded source that used different UUIDs for the same underlying usage.
func (d *DB) SupersedeUsageBySourceFilePathDeviceUUIDPrefix(source string, filePath string, deviceID string, uuidPrefix string) (int64, error) {
	if source == "" || filePath == "" || deviceID == "" || uuidPrefix == "" {
		return 0, nil
	}
	result, err := d.conn.Exec(
		"UPDATE usage_records SET superseded = 1, updated_at = ? WHERE source = ? AND file_path = ? AND device_id = ? AND uuid LIKE ? AND COALESCE(superseded, 0) = 0",
		formatLogTimestamp(time.Now().UTC()), source, filePath, deviceID, uuidPrefix+"%",
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// QueryPeriodStatsSince returns total cost and token breakdown since the given time.
// source filters by source column (e.g. "Claude Code", "Gemini CLI"); empty means all.
func (d *DB) QueryPeriodStatsSince(since time.Time, deviceID string, source string) (float64, int, int, int, int, error) {
	var cost sql.NullFloat64
	var inTok, cacheTok, cacheCreationTok, outTok sql.NullInt64
	query := "SELECT COALESCE(SUM(cost_usd), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(output_tokens), 0) FROM usage_records WHERE " + activeUsagePredicate + " AND julianday(log_timestamp) >= julianday(?)"
	args := []interface{}{formatLogTimestamp(since)}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	err := d.conn.QueryRow(query, args...).Scan(&cost, &inTok, &cacheTok, &cacheCreationTok, &outTok)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	return cost.Float64, int(inTok.Int64), int(cacheTok.Int64), int(cacheCreationTok.Int64), int(outTok.Int64), nil
}

// QueryPeriodStatsBuckets returns multiple period aggregates in a single scan.
// A zero Since value means the all-time bucket.
func (d *DB) QueryPeriodStatsBuckets(windows []PeriodStatsWindow, deviceID string, source string) ([]PeriodStatsBucket, error) {
	if len(windows) == 0 {
		return []PeriodStatsBucket{}, nil
	}

	var query strings.Builder
	query.WriteString("SELECT ")
	args := make([]interface{}, 0, len(windows)*5+2)
	for i, window := range windows {
		if i > 0 {
			query.WriteString(", ")
		}
		conditional := !window.Since.IsZero()
		appendPeriodAggregateExpr(&query, "cost_usd", conditional)
		query.WriteString(", ")
		appendPeriodAggregateExpr(&query, "input_tokens", conditional)
		query.WriteString(", ")
		appendPeriodAggregateExpr(&query, "cached_tokens", conditional)
		query.WriteString(", ")
		appendPeriodAggregateExpr(&query, "cache_creation_tokens", conditional)
		query.WriteString(", ")
		appendPeriodAggregateExpr(&query, "output_tokens", conditional)
		if conditional {
			since := formatLogTimestamp(window.Since)
			args = append(args, since, since, since, since, since)
		}
	}
	query.WriteString(" FROM usage_records WHERE ")
	query.WriteString(activeUsagePredicate)

	if deviceID != "" && deviceID != "all" {
		query.WriteString(" AND device_id = ?")
		args = append(args, deviceID)
	}
	if source != "" {
		query.WriteString(" AND source = ?")
		args = append(args, source)
	}

	values := make([]struct {
		cost          sql.NullFloat64
		input         sql.NullInt64
		cached        sql.NullInt64
		cacheCreation sql.NullInt64
		output        sql.NullInt64
	}, len(windows))
	scanArgs := make([]interface{}, 0, len(windows)*5)
	for i := range values {
		scanArgs = append(scanArgs, &values[i].cost, &values[i].input, &values[i].cached, &values[i].cacheCreation, &values[i].output)
	}

	if err := d.conn.QueryRow(query.String(), args...).Scan(scanArgs...); err != nil {
		return nil, err
	}

	buckets := make([]PeriodStatsBucket, 0, len(windows))
	for i, window := range windows {
		buckets = append(buckets, PeriodStatsBucket{
			Label:               window.Label,
			Cost:                values[i].cost.Float64,
			InputTokens:         int(values[i].input.Int64),
			CachedTokens:        int(values[i].cached.Int64),
			CacheCreationTokens: int(values[i].cacheCreation.Int64),
			OutputTokens:        int(values[i].output.Int64),
		})
	}
	return buckets, nil
}

func appendPeriodAggregateExpr(query *strings.Builder, column string, conditional bool) {
	if conditional {
		query.WriteString("COALESCE(SUM(CASE WHEN julianday(log_timestamp) >= julianday(?) THEN ")
		query.WriteString(column)
		query.WriteString(" ELSE 0 END), 0)")
		return
	}
	query.WriteString("COALESCE(SUM(")
	query.WriteString(column)
	query.WriteString("), 0)")
}

// QueryPeriodStatsAll returns total cumulative cost and token breakdown.
// source filters by source column; empty means all.
func (d *DB) QueryPeriodStatsAll(deviceID string, source string) (float64, int, int, int, int, error) {
	var cost sql.NullFloat64
	var inTok, cacheTok, cacheCreationTok, outTok sql.NullInt64
	query := "SELECT COALESCE(SUM(cost_usd), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(output_tokens), 0) FROM usage_records WHERE " + activeUsagePredicate
	var args []interface{}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	err := d.conn.QueryRow(query, args...).Scan(&cost, &inTok, &cacheTok, &cacheCreationTok, &outTok)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	return cost.Float64, int(inTok.Int64), int(cacheTok.Int64), int(cacheCreationTok.Int64), int(outTok.Int64), nil
}

// QueryTokenSourceSummaries returns per-source token totals in one aggregate query.
func (d *DB) QueryTokenSourceSummaries(since time.Time, deviceID string) ([]model.TokenSourceSummary, error) {
	query := `SELECT source,
		COALESCE(SUM(CASE WHEN julianday(log_timestamp) >= julianday(?) THEN input_tokens + output_tokens ELSE 0 END), 0),
		COALESCE(SUM(input_tokens + output_tokens), 0),
		COALESCE(SUM(cost_usd), 0)
		FROM usage_records
		WHERE ` + activeUsagePredicate
	args := []interface{}{formatLogTimestamp(since)}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}

	query += " GROUP BY source ORDER BY source"

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []model.TokenSourceSummary
	for rows.Next() {
		var summary model.TokenSourceSummary
		if err := rows.Scan(&summary.Source, &summary.Tokens24h, &summary.TokensTotal, &summary.CostTotal); err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

// QueryStatsSince returns per-model stats since the given time, sorted by cost descending.
func (d *DB) QueryStatsSince(since time.Time, deviceID string, source string) ([]ModelStat, error) {
	query := `
		SELECT source, model, COUNT(*) as events,
			SUM(input_tokens), SUM(cached_tokens), SUM(cache_creation_tokens), SUM(output_tokens),
			SUM(cost_usd)
		FROM usage_records
		WHERE COALESCE(superseded, 0) = 0
	`
	var args []interface{}

	if !since.IsZero() {
		query += " AND julianday(log_timestamp) >= julianday(?)"
		args = append(args, formatLogTimestamp(since))
	}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	query += `
		GROUP BY source, model
		ORDER BY SUM(cost_usd) DESC
	`

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []ModelStat
	for rows.Next() {
		var s ModelStat
		if err := rows.Scan(&s.Source, &s.Model, &s.Events,
			&s.InputTokens, &s.CachedTokens, &s.CacheCreationTokens, &s.OutputTokens, &s.TotalCost); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// QuerySourceTotalsSince returns per-source aggregate stats without model/project details.
func (d *DB) QuerySourceTotalsSince(since time.Time, deviceID string, source string) ([]SourceTotalStat, error) {
	if since.IsZero() {
		return d.QuerySourceTotalsSummary(deviceID, source)
	}

	query := `
		SELECT source, COUNT(*) as events,
			SUM(input_tokens), SUM(cached_tokens), SUM(cache_creation_tokens), SUM(output_tokens),
			SUM(cost_usd)
		FROM usage_records
		WHERE COALESCE(superseded, 0) = 0
	`
	var args []interface{}

	if !since.IsZero() {
		query += " AND julianday(log_timestamp) >= julianday(?)"
		args = append(args, formatLogTimestamp(since))
	}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	query += `
		GROUP BY source
		ORDER BY SUM(cost_usd) DESC
	`

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []SourceTotalStat
	for rows.Next() {
		var s SourceTotalStat
		if err := rows.Scan(&s.Source, &s.Events,
			&s.InputTokens, &s.CachedTokens, &s.CacheCreationTokens, &s.OutputTokens, &s.TotalCost); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// QuerySourceTotalsSummary returns all-time per-source totals from the maintained summary table.
func (d *DB) QuerySourceTotalsSummary(deviceID string, source string) ([]SourceTotalStat, error) {
	query := `
		SELECT source, COALESCE(SUM(events), 0) as events,
			COALESCE(SUM(input_tokens), 0), COALESCE(SUM(cached_tokens), 0),
			COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(cost_usd), 0)
		FROM usage_source_totals
		WHERE 1 = 1
	`
	var args []interface{}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	query += `
		GROUP BY source
		ORDER BY SUM(cost_usd) DESC
	`

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []SourceTotalStat
	for rows.Next() {
		var s SourceTotalStat
		if err := rows.Scan(&s.Source, &s.Events,
			&s.InputTokens, &s.CachedTokens, &s.CacheCreationTokens, &s.OutputTokens, &s.TotalCost); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// QueryProjectStatsSince returns per-project stats since the given time.
func (d *DB) QueryProjectStatsSince(since time.Time, deviceID string, source string) ([]model.ProjectStat, error) {
	query := `
		SELECT COALESCE(project, 'Default') as project, COUNT(*) as events,
			SUM(input_tokens), SUM(cached_tokens), SUM(cache_creation_tokens), SUM(output_tokens),
			SUM(cost_usd)
		FROM usage_records
		WHERE COALESCE(superseded, 0) = 0
	`
	var args []interface{}

	if !since.IsZero() {
		query += " AND julianday(log_timestamp) >= julianday(?)"
		args = append(args, formatLogTimestamp(since))
	}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}
	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	query += `
		GROUP BY project
		ORDER BY SUM(cost_usd) DESC
	`

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []model.ProjectStat
	for rows.Next() {
		var s model.ProjectStat
		if err := rows.Scan(&s.Project, &s.Events,
			&s.InputTokens, &s.CachedTokens, &s.CacheCreationTokens, &s.OutputTokens, &s.TotalCost); err != nil {
			return nil, err
		}
		s.CacheHitRate = model.CacheHitRatePercent(s.InputTokens, s.CachedTokens)
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// QueryDevices returns a list of unique devices.
func (d *DB) QueryDevices() ([]string, error) {
	rows, err := d.conn.Query("SELECT DISTINCT device_id FROM usage_records WHERE " + activeUsagePredicate + " ORDER BY device_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		devices = append(devices, id)
	}
	return devices, rows.Err()
}

// UsageRecord represents a single stored usage record for per-row analysis.
type UsageRecord struct {
	Model               string
	InputTokens         int
	CachedTokens        int
	CacheCreationTokens int
	OutputTokens        int
	CostUSD             float64
}

// QueryUsageRecords returns individual usage records since the given time.
func (d *DB) QueryUsageRecords(since time.Time, deviceID string) ([]UsageRecord, error) {
	query := `SELECT model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd
		FROM usage_records WHERE COALESCE(superseded, 0) = 0`
	var args []interface{}

	if !since.IsZero() {
		query += " AND julianday(log_timestamp) >= julianday(?)"
		args = append(args, formatLogTimestamp(since))
	}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []UsageRecord
	for rows.Next() {
		var r UsageRecord
		if err := rows.Scan(&r.Model, &r.InputTokens, &r.CachedTokens, &r.CacheCreationTokens, &r.OutputTokens, &r.CostUSD); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// QuerySyncRecordsSince returns raw records for P2P LAN DB synchronization.
// The cursor is based on updated_at, not log_timestamp, so historical repairs
// and superseded markers propagate even when the underlying usage happened long ago.
func (d *DB) QuerySyncRecordsSince(since time.Time) ([]model.SyncRecord, error) {
	page, err := d.QuerySyncRecordsPage(since, 0, 0)
	if err != nil {
		return nil, err
	}
	return page.Records, nil
}

// QuerySyncRecordsPage returns a page of synchronization records. The cursor is
// (updated_at, id), so rows with identical updated_at values cannot be skipped.
func (d *DB) QuerySyncRecordsPage(since time.Time, afterID int64, limit int) (model.SyncPullResponse, error) {
	return d.querySyncRecordsPage(since, afterID, limit, "")
}

// QuerySyncRecordsPageForDevice returns a page scoped to one device ID.
func (d *DB) QuerySyncRecordsPageForDevice(since time.Time, afterID int64, limit int, deviceID string) (model.SyncPullResponse, error) {
	return d.querySyncRecordsPage(since, afterID, limit, deviceID)
}

func (d *DB) querySyncRecordsPage(since time.Time, afterID int64, limit int, deviceID string) (model.SyncPullResponse, error) {
	query := `SELECT uuid, log_timestamp, source, model, project, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id, COALESCE(superseded, 0), COALESCE(updated_at, timestamp, log_timestamp), id
		FROM usage_records WHERE `
	args := []interface{}{formatLogTimestamp(since)}
	if afterID > 0 {
		query += `(julianday(COALESCE(updated_at, timestamp, log_timestamp)) > julianday(?)
			OR (julianday(COALESCE(updated_at, timestamp, log_timestamp)) = julianday(?) AND id > ?))`
		args = append(args, formatLogTimestamp(since), afterID)
	} else {
		query += `julianday(COALESCE(updated_at, timestamp, log_timestamp)) >= julianday(?)`
	}
	if deviceID != "" {
		query += ` AND device_id = ?`
		args = append(args, deviceID)
	}
	query += ` ORDER BY julianday(COALESCE(updated_at, timestamp, log_timestamp)), id`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit+1)
	}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return model.SyncPullResponse{}, err
	}
	defer rows.Close()

	var records []model.SyncRecord
	var lastUpdatedAt time.Time
	var lastID int64
	hasMore := false
	for rows.Next() {
		var r model.SyncRecord
		var rowID int64
		var logTsStr string
		var updatedAtStr string
		var uuidStr sql.NullString
		var superseded int

		if err := rows.Scan(&uuidStr, &logTsStr, &r.Source, &r.Model, &r.Project, &r.InputTokens, &r.CachedTokens, &r.CacheCreationTokens, &r.OutputTokens, &r.CostUSD, &r.FilePath, &r.DeviceID, &superseded, &updatedAtStr, &rowID); err != nil {
			return model.SyncPullResponse{}, err
		}
		if limit > 0 && len(records) >= limit {
			hasMore = true
			break
		}

		r.UUID = uuidStr.String
		if t, err := parseLogTimestamp(logTsStr); err == nil {
			r.Timestamp = t
		}
		if t, err := parseLogTimestamp(updatedAtStr); err == nil {
			r.UpdatedAt = t
		}
		r.Superseded = superseded != 0
		records = append(records, r)
		lastUpdatedAt = r.UpdatedAt
		lastID = rowID
	}
	if err := rows.Err(); err != nil {
		return model.SyncPullResponse{}, err
	}
	return model.SyncPullResponse{
		Records:       records,
		NextUpdatedAt: lastUpdatedAt,
		NextAfterID:   lastID,
		HasMore:       hasMore,
	}, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

// UpsertKnownDir records a directory known to contain JSONL files.
func (d *DB) UpsertKnownDir(dirPath string) error {
	query := `INSERT INTO known_dirs (dir_path) VALUES (?)
	ON CONFLICT(dir_path) DO UPDATE SET last_seen = CURRENT_TIMESTAMP`
	_, err := d.conn.Exec(query, dirPath)
	return err
}

// ListKnownDirs returns all cached JSONL directories.
func (d *DB) ListKnownDirs() ([]string, error) {
	rows, err := d.conn.Query("SELECT dir_path FROM known_dirs ORDER BY dir_path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dirs []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		dirs = append(dirs, p)
	}
	return dirs, rows.Err()
}

// ListKnownFiles returns all file paths cached in scan_offsets.
func (d *DB) ListKnownFiles() ([]string, error) {
	rows, err := d.conn.Query("SELECT file_path FROM scan_offsets ORDER BY file_path")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		files = append(files, p)
	}
	return files, rows.Err()
}

// BackfillProjectsFromFilePaths recalculates Default project names for records
// that still have their originating log path.
func (d *DB) BackfillProjectsFromFilePaths(extractProject func(string) string) (int64, error) {
	rows, err := d.conn.Query("SELECT id, file_path FROM usage_records WHERE " + activeUsagePredicate + " AND file_path != '' AND (project IS NULL OR project = '' OR project = 'Default')")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type update struct {
		id      int64
		project string
	}
	var updates []update
	for rows.Next() {
		var id int64
		var filePath string
		if err := rows.Scan(&id, &filePath); err != nil {
			return 0, err
		}
		project := extractProject(filePath)
		if project != "" && project != "Default" {
			updates = append(updates, update{id: id, project: project})
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(updates) == 0 {
		return 0, nil
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare("UPDATE usage_records SET project = ? WHERE id = ?")
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	defer stmt.Close()

	var changed int64
	for _, u := range updates {
		result, err := stmt.Exec(u.project, u.id)
		if err != nil {
			tx.Rollback()
			return changed, err
		}
		n, _ := result.RowsAffected()
		changed += n
	}
	if err := tx.Commit(); err != nil {
		return changed, err
	}
	return changed, nil
}

// DeduplicateExisting removes duplicate usage records, keeping the one with the lowest rowid.
// Returns the number of rows removed.
func (d *DB) DeduplicateExisting() (int64, error) {
	query := `
	DELETE FROM usage_records WHERE id NOT IN (
		SELECT MIN(id) FROM usage_records
		WHERE COALESCE(superseded, 0) = 0
		GROUP BY log_timestamp, file_path, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, device_id
	) AND (uuid IS NULL OR uuid = '') AND COALESCE(superseded, 0) = 0`
	result, err := d.conn.Exec(query)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// SetDeviceAlias sets or updates the display name for a device ID.
func (d *DB) SetDeviceAlias(deviceID, displayName string) error {
	query := `INSERT INTO device_aliases (device_id, display_name) VALUES (?, ?)
	ON CONFLICT(device_id) DO UPDATE SET display_name = excluded.display_name`
	_, err := d.conn.Exec(query, deviceID, displayName)
	return err
}

// GetDeviceAliases returns a map of deviceID to displayName.
func (d *DB) GetDeviceAliases() (map[string]string, error) {
	rows, err := d.conn.Query("SELECT device_id, display_name FROM device_aliases")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aliases := make(map[string]string)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err == nil {
			aliases[id] = name
		}
	}
	return aliases, nil
}

// RawExec executes a raw SQL statement. Exposed for testing only.
func (d *DB) RawExec(query string, args ...interface{}) error {
	_, err := d.conn.Exec(query, args...)
	return err
}
