package db

import (
	"database/sql"
	"errors"
	"time"

	"ai-flight-dashboard/internal/model"
	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

const logTimestampLayout = "2006-01-02T15:04:05.000000000Z"
const activeUsagePredicate = "COALESCE(superseded, 0) = 0"

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
	CREATE INDEX IF NOT EXISTS idx_usage_log_ts ON usage_records(log_timestamp);
	CREATE INDEX IF NOT EXISTS idx_usage_device ON usage_records(device_id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_device_uuid ON usage_records(device_id, uuid) WHERE uuid IS NOT NULL AND uuid != '';

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
	`
	_, err := conn.Exec(schema)
	if err != nil {
		return err
	}

	// Lightweight migrations for existing tables
	conn.Exec("ALTER TABLE usage_records ADD COLUMN device_id TEXT DEFAULT 'local'")
	conn.Exec("ALTER TABLE usage_records ADD COLUMN cache_creation_tokens INTEGER DEFAULT 0")
	conn.Exec("ALTER TABLE usage_records ADD COLUMN uuid TEXT")
	conn.Exec("ALTER TABLE usage_records ADD COLUMN project TEXT DEFAULT 'Default'")
	conn.Exec("ALTER TABLE usage_records ADD COLUMN superseded INTEGER DEFAULT 0")
	conn.Exec("ALTER TABLE usage_records ADD COLUMN updated_at DATETIME")
	conn.Exec("UPDATE usage_records SET updated_at = COALESCE(timestamp, log_timestamp, CURRENT_TIMESTAMP) WHERE updated_at IS NULL OR updated_at = ''")
	conn.Exec("DROP INDEX IF EXISTS idx_usage_uuid")
	conn.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_device_uuid ON usage_records(device_id, uuid) WHERE uuid IS NOT NULL AND uuid != ''")
	if err := ensurePartialDedupIndex(conn); err != nil {
		return err
	}

	return nil
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
	logTimestamp := formatLogTimestamp(logTS)
	updatedAt := formatLogTimestamp(time.Now().UTC())
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
	if r.Superseded {
		if u.UUID != "" {
			return d.insertUsageWithTime(u, r.CostUSD, u.Timestamp, r.FilePath, deviceID, true)
		}
		return d.supersedeOrInsertLegacySyncRecord(u, r.CostUSD, r.FilePath, deviceID)
	}
	return d.InsertUsageWithTime(u, r.CostUSD, u.Timestamp, r.FilePath, deviceID)
}

func (d *DB) supersedeOrInsertLegacySyncRecord(u model.TokenUsage, cost float64, filePath string, deviceID string) error {
	logTimestamp := formatLogTimestamp(u.Timestamp)
	updatedAt := formatLogTimestamp(time.Now().UTC())
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
	return d.insertUsageWithTime(u, cost, u.Timestamp, filePath, deviceID, true)
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

// QueryStatsSince returns per-model stats since the given time, sorted by cost descending.
func (d *DB) QueryStatsSince(since time.Time, deviceID string, source string) ([]ModelStat, error) {
	query := `
		SELECT source, model, COUNT(*) as events,
			SUM(input_tokens), SUM(cached_tokens), SUM(cache_creation_tokens), SUM(output_tokens),
			SUM(cost_usd)
		FROM usage_records
		WHERE COALESCE(superseded, 0) = 0
			AND julianday(log_timestamp) >= julianday(?)
	`
	args := []interface{}{formatLogTimestamp(since)}

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

// QueryProjectStatsSince returns per-project stats since the given time.
func (d *DB) QueryProjectStatsSince(since time.Time, deviceID string, source string) ([]model.ProjectStat, error) {
	query := `
		SELECT COALESCE(project, 'Default') as project, COUNT(*) as events,
			SUM(input_tokens), SUM(cached_tokens), SUM(cache_creation_tokens), SUM(output_tokens),
			SUM(cost_usd)
		FROM usage_records
		WHERE COALESCE(superseded, 0) = 0
			AND julianday(log_timestamp) >= julianday(?)
	`
	args := []interface{}{formatLogTimestamp(since)}

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
		FROM usage_records WHERE COALESCE(superseded, 0) = 0
			AND julianday(log_timestamp) >= julianday(?)`
	args := []interface{}{formatLogTimestamp(since)}

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
	query := `SELECT uuid, log_timestamp, source, model, project, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id, COALESCE(superseded, 0), COALESCE(updated_at, timestamp, log_timestamp)
		FROM usage_records WHERE julianday(COALESCE(updated_at, timestamp, log_timestamp)) >= julianday(?)
		ORDER BY julianday(COALESCE(updated_at, timestamp, log_timestamp)), id`

	rows, err := d.conn.Query(query, formatLogTimestamp(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []model.SyncRecord
	for rows.Next() {
		var r model.SyncRecord
		var logTsStr string
		var updatedAtStr string
		var uuidStr sql.NullString
		var superseded int

		if err := rows.Scan(&uuidStr, &logTsStr, &r.Source, &r.Model, &r.Project, &r.InputTokens, &r.CachedTokens, &r.CacheCreationTokens, &r.OutputTokens, &r.CostUSD, &r.FilePath, &r.DeviceID, &superseded, &updatedAtStr); err != nil {
			return nil, err
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
	}
	return records, rows.Err()
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
