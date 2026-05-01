package db

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"ai-flight-dashboard/internal/model"
)

type DB struct {
	conn *sql.DB
}

type ModelStat struct {
	Model      string
	Source     string
	Events     int
	InputTokens         int
	CachedTokens        int
	CacheCreationTokens int
	OutputTokens        int
	TotalCost  float64
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
	CREATE INDEX IF NOT EXISTS idx_usage_log_ts ON usage_records(log_timestamp);
	CREATE INDEX IF NOT EXISTS idx_usage_device ON usage_records(device_id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_uuid ON usage_records(uuid) WHERE uuid IS NOT NULL AND uuid != '';

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
	conn.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_uuid ON usage_records(uuid) WHERE uuid IS NOT NULL AND uuid != ''")
	conn.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_dedup ON usage_records(log_timestamp, file_path, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, device_id)")

	return nil
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
	if u.UUID != "" {
		query := `
		INSERT INTO usage_records (uuid, log_timestamp, source, model, project, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uuid) WHERE uuid IS NOT NULL AND uuid != '' DO UPDATE SET
			log_timestamp = excluded.log_timestamp,
			project = excluded.project,
			input_tokens = excluded.input_tokens,
			cached_tokens = excluded.cached_tokens,
			cache_creation_tokens = excluded.cache_creation_tokens,
			output_tokens = excluded.output_tokens,
			cost_usd = excluded.cost_usd
		`
		_, err := d.conn.Exec(query, u.UUID, logTS.Format(time.RFC3339), u.Source, u.Model, u.Project, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens, cost, filePath, deviceID)
		return err
	}

	query := `
	INSERT OR IGNORE INTO usage_records (log_timestamp, source, model, project, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, cost_usd, file_path, device_id)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := d.conn.Exec(query, logTS.Format(time.RFC3339), u.Source, u.Model, u.Project, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens, cost, filePath, deviceID)
	return err
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

// QueryPeriodStatsSince returns total cost and token breakdown since the given time.
func (d *DB) QueryPeriodStatsSince(since time.Time, deviceID string) (float64, int, int, int, int, error) {
	var cost sql.NullFloat64
	var inTok, cacheTok, cacheCreationTok, outTok sql.NullInt64
	query := "SELECT COALESCE(SUM(cost_usd), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(output_tokens), 0) FROM usage_records WHERE log_timestamp >= ?"
	args := []interface{}{since.Format(time.RFC3339)}
	
	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}

	err := d.conn.QueryRow(query, args...).Scan(&cost, &inTok, &cacheTok, &cacheCreationTok, &outTok)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	return cost.Float64, int(inTok.Int64), int(cacheTok.Int64), int(cacheCreationTok.Int64), int(outTok.Int64), nil
}

// QueryPeriodStatsAll returns total cumulative cost and token breakdown.
func (d *DB) QueryPeriodStatsAll(deviceID string) (float64, int, int, int, int, error) {
	var cost sql.NullFloat64
	var inTok, cacheTok, cacheCreationTok, outTok sql.NullInt64
	query := "SELECT COALESCE(SUM(cost_usd), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(output_tokens), 0) FROM usage_records WHERE 1=1"
	var args []interface{}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
	}

	err := d.conn.QueryRow(query, args...).Scan(&cost, &inTok, &cacheTok, &cacheCreationTok, &outTok)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	return cost.Float64, int(inTok.Int64), int(cacheTok.Int64), int(cacheCreationTok.Int64), int(outTok.Int64), nil
}

// QueryStatsSince returns per-model stats since the given time, sorted by cost descending.
func (d *DB) QueryStatsSince(since time.Time, deviceID string) ([]ModelStat, error) {
	query := `
		SELECT source, model, COUNT(*) as events,
			SUM(input_tokens), SUM(cached_tokens), SUM(cache_creation_tokens), SUM(output_tokens),
			SUM(cost_usd)
		FROM usage_records
		WHERE log_timestamp >= ?
	`
	args := []interface{}{since.Format(time.RFC3339)}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
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

// ProjectStat represents aggregated token usage and cost for a project.
type ProjectStat struct {
	Project             string  `json:"project"`
	Events              int     `json:"events"`
	InputTokens         int     `json:"input_tokens"`
	CachedTokens        int     `json:"cached_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	TotalCost           float64 `json:"total_cost"`
}

// QueryProjectStatsSince returns per-project stats since the given time.
func (d *DB) QueryProjectStatsSince(since time.Time, deviceID string) ([]ProjectStat, error) {
	query := `
		SELECT COALESCE(project, 'Default') as project, COUNT(*) as events,
			SUM(input_tokens), SUM(cached_tokens), SUM(cache_creation_tokens), SUM(output_tokens),
			SUM(cost_usd)
		FROM usage_records
		WHERE log_timestamp >= ?
	`
	args := []interface{}{since.Format(time.RFC3339)}

	if deviceID != "" && deviceID != "all" {
		query += " AND device_id = ?"
		args = append(args, deviceID)
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

	var stats []ProjectStat
	for rows.Next() {
		var s ProjectStat
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
	rows, err := d.conn.Query("SELECT DISTINCT device_id FROM usage_records ORDER BY device_id")
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
		FROM usage_records WHERE log_timestamp >= ?`
	args := []interface{}{since.Format(time.RFC3339)}

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

// DeduplicateExisting removes duplicate usage records, keeping the one with the lowest rowid.
// Returns the number of rows removed.
func (d *DB) DeduplicateExisting() (int64, error) {
	query := `
	DELETE FROM usage_records WHERE id NOT IN (
		SELECT MIN(id) FROM usage_records
		GROUP BY log_timestamp, file_path, model, input_tokens, cached_tokens, cache_creation_tokens, output_tokens, device_id
	) AND (uuid IS NULL OR uuid = '')`
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
