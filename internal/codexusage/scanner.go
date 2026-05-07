package codexusage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/watcher"

	_ "github.com/mattn/go-sqlite3"
)

const (
	SourceName = "Codex"
	offsetKey  = "codex:logs_2.sqlite:id"
)

var metricPattern = regexp.MustCompile(`([A-Za-z_.]+)=("[^"]*"|\S+)`)

type Scanner struct {
	db        *db.DB
	calc      *calculator.Calculator
	DeviceID  string
	StatePath string
	LogsPath  string
}

type threadInfo struct {
	model string
	cwd   string
}

type codexEvent struct {
	logID        int64
	ts           time.Time
	conversation string
	model        string
	input        int
	cached       int
	output       int
}

func New(database *db.DB, calc *calculator.Calculator, deviceID string) *Scanner {
	home, _ := os.UserHomeDir()
	return &Scanner{
		db:        database,
		calc:      calc,
		DeviceID:  deviceID,
		StatePath: filepath.Join(home, ".codex", "state_5.sqlite"),
		LogsPath:  filepath.Join(home, ".codex", "logs_2.sqlite"),
	}
}

func NewWithPaths(database *db.DB, calc *calculator.Calculator, deviceID string, statePath string, logsPath string) *Scanner {
	return &Scanner{db: database, calc: calc, DeviceID: deviceID, StatePath: statePath, LogsPath: logsPath}
}

// Scan imports Codex response.completed telemetry from ~/.codex/logs_2.sqlite.
// The telemetry has explicit input/cached/output fields and conversation.id;
// state_5.sqlite is used only to resolve project cwd for that conversation.
func (s *Scanner) Scan(usageChan chan<- watcher.TokenUsage) (int, error) {
	if s.StatePath == "" || s.LogsPath == "" {
		return 0, nil
	}
	if _, err := os.Stat(s.StatePath); err != nil {
		return 0, nil
	}
	if _, err := os.Stat(s.LogsPath); err != nil {
		return 0, nil
	}

	threads, err := loadThreads(s.StatePath)
	if err != nil {
		return 0, err
	}

	offset, err := s.db.GetOffset(offsetKey)
	if err != nil {
		return 0, err
	}

	logConn, err := openReadonlySQLite(s.LogsPath)
	if err != nil {
		return 0, err
	}
	defer logConn.Close()

	rows, err := logConn.Query(`
		SELECT id, ts, ts_nanos, feedback_log_body
		FROM logs
		WHERE id > ?
			AND target = 'codex_otel.log_only'
			AND feedback_log_body LIKE '%event.kind=response.completed%'
			AND feedback_log_body LIKE '%input_token_count=%'
		ORDER BY id ASC
	`, offset)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	maxOffset := offset
	for rows.Next() {
		var logID, tsSec, tsNanos int64
		var body string
		if err := rows.Scan(&logID, &tsSec, &tsNanos, &body); err != nil {
			return count, err
		}
		event, ok := parseCodexEvent(logID, tsSec, tsNanos, body)
		if !ok {
			if logID > maxOffset {
				maxOffset = logID
			}
			continue
		}
		info, ok := threads[event.conversation]
		if !ok {
			// The telemetry row can become visible before Codex has flushed the
			// corresponding thread metadata. Stop here so a later scan can retry
			// this event instead of permanently advancing past it.
			break
		}
		modelName := event.model
		if modelName == "" {
			modelName = info.model
		}
		if modelName == "" {
			modelName = "codex"
		}

		u := model.TokenUsage{
			Source:       SourceName,
			Model:        modelName,
			Project:      watcher.ExtractProjectNameFromCWD(info.cwd),
			InputTokens:  event.input,
			CachedTokens: event.cached,
			OutputTokens: event.output,
			Timestamp:    event.ts,
			UUID:         fmt.Sprintf("codex:%d", event.logID),
		}
		cost, _ := s.calc.CalculateCost(u.Model, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens)
		if err := s.db.InsertUsageWithTime(u, cost, u.Timestamp, s.LogsPath, s.DeviceID); err != nil {
			return count, err
		}
		if logID > maxOffset {
			maxOffset = logID
		}
		if usageChan != nil {
			usageChan <- u
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return count, err
	}
	if maxOffset > offset {
		if err := s.db.SetOffset(offsetKey, maxOffset); err != nil {
			return count, err
		}
	}
	return count, nil
}

func loadThreads(statePath string) (map[string]threadInfo, error) {
	conn, err := openReadonlySQLite(statePath)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	rows, err := conn.Query("SELECT id, COALESCE(model, ''), cwd FROM threads")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	threads := make(map[string]threadInfo)
	for rows.Next() {
		var id, modelName, cwd string
		if err := rows.Scan(&id, &modelName, &cwd); err != nil {
			return nil, err
		}
		threads[id] = threadInfo{model: modelName, cwd: cwd}
	}
	return threads, rows.Err()
}

func parseCodexEvent(logID int64, tsSec int64, tsNanos int64, body string) (codexEvent, bool) {
	fields := parseMetrics(body)
	input := parseIntField(fields, "input_token_count")
	output := parseIntField(fields, "output_token_count")
	cached := parseIntField(fields, "cached_token_count")
	conversation := fields["conversation.id"]
	if conversation == "" || input == 0 && output == 0 && cached == 0 {
		return codexEvent{}, false
	}

	ts := time.Unix(tsSec, tsNanos).UTC()
	if eventTS := fields["event.timestamp"]; eventTS != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, eventTS); err == nil {
			ts = parsed.UTC()
		}
	}

	return codexEvent{
		logID:        logID,
		ts:           ts,
		conversation: conversation,
		model:        fields["model"],
		input:        input,
		cached:       cached,
		output:       output,
	}, true
}

func parseMetrics(body string) map[string]string {
	fields := make(map[string]string)
	for _, match := range metricPattern.FindAllStringSubmatch(body, -1) {
		value := match[2]
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}
		fields[match[1]] = value
	}
	return fields
}

func parseIntField(fields map[string]string, key string) int {
	value, err := strconv.Atoi(fields[key])
	if err != nil {
		return 0
	}
	return value
}

func openReadonlySQLite(path string) (*sql.DB, error) {
	return sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro&_busy_timeout=5000", filepath.ToSlash(path)))
}
