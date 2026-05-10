package codexusage

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/watcher"

	_ "github.com/mattn/go-sqlite3"
)

const (
	SourceName = "Codex"
	OffsetKey  = "codex:logs_2.sqlite:id"
)

var metricPattern = regexp.MustCompile(`([A-Za-z_.]+)=("[^"]*"|\S+)`)

type Scanner struct {
	db          *db.DB
	calc        *calculator.Calculator
	DeviceID    string
	StatePath   string
	LogsPath    string
	SessionsDir string
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
	reasoning    int
}

func New(database *db.DB, calc *calculator.Calculator, deviceID string) *Scanner {
	home, _ := os.UserHomeDir()
	return &Scanner{
		db:          database,
		calc:        calc,
		DeviceID:    deviceID,
		StatePath:   filepath.Join(home, ".codex", "state_5.sqlite"),
		LogsPath:    filepath.Join(home, ".codex", "logs_2.sqlite"),
		SessionsDir: filepath.Join(home, ".codex", "sessions"),
	}
}

func NewWithPaths(database *db.DB, calc *calculator.Calculator, deviceID string, statePath string, logsPath string) *Scanner {
	return &Scanner{db: database, calc: calc, DeviceID: deviceID, StatePath: statePath, LogsPath: logsPath}
}

func NewWithSessionPaths(database *db.DB, calc *calculator.Calculator, deviceID string, statePath string, logsPath string, sessionsDir string) *Scanner {
	return &Scanner{db: database, calc: calc, DeviceID: deviceID, StatePath: statePath, LogsPath: logsPath, SessionsDir: sessionsDir}
}

// Scan imports Codex usage from session JSONL totals when available, falling
// back to response.completed telemetry from ~/.codex/logs_2.sqlite.
func (s *Scanner) Scan(usageChan chan<- watcher.TokenUsage) (int, error) {
	threads := make(map[string]threadInfo)
	if s.StatePath != "" {
		if _, err := os.Stat(s.StatePath); err == nil {
			loaded, ok, err := loadThreads(s.StatePath)
			if err != nil {
				return 0, err
			}
			if ok {
				threads = loaded
			}
		}
	}

	sessionCount, hasSessionTotals, err := s.scanSessions(threads, usageChan)
	if err != nil {
		return sessionCount, err
	}
	if hasSessionTotals {
		if _, err := s.db.SupersedeUsageBySourceFilePathDeviceUUIDPrefix(SourceName, s.LogsPath, s.DeviceID, "codex:"); err != nil {
			return sessionCount, err
		}
		return sessionCount, nil
	}

	logCount, err := s.scanLogs(threads, usageChan)
	return sessionCount + logCount, err
}

func (s *Scanner) scanLogs(threads map[string]threadInfo, usageChan chan<- watcher.TokenUsage) (int, error) {
	if len(threads) == 0 || s.LogsPath == "" {
		return 0, nil
	}
	if _, err := os.Stat(s.LogsPath); err != nil {
		return 0, nil
	}

	offset, err := s.db.GetOffset(OffsetKey)
	if err != nil {
		return 0, err
	}

	logConn, err := openReadonlySQLite(s.LogsPath)
	if err != nil {
		return 0, err
	}
	defer logConn.Close()
	hasLogs, err := hasTable(logConn, "logs")
	if err != nil {
		if isOptionalCodexDBError(err) {
			return 0, nil
		}
		return 0, err
	}
	if !hasLogs {
		return 0, nil
	}

	rows, err := logConn.Query(`
			SELECT id, ts, ts_nanos, feedback_log_body
			FROM logs
			WHERE id > ?
				AND target = 'codex_otel.log_only'
				AND (
					feedback_log_body LIKE 'event.name="codex.sse_event" event.kind=response.completed% input_token_count=%'
					OR feedback_log_body LIKE 'event.name=codex.sse_event event.kind=response.completed% input_token_count=%'
					OR feedback_log_body LIKE '%: event.name="codex.sse_event" event.kind=response.completed% input_token_count=%'
					OR feedback_log_body LIKE '%: event.name=codex.sse_event event.kind=response.completed% input_token_count=%'
				)
			ORDER BY id ASC
		`, offset)
	if err != nil {
		if isOptionalCodexDBError(err) {
			return 0, nil
		}
		return 0, err
	}
	defer rows.Close()

	count := 0
	maxOffset := offset
	for rows.Next() {
		var logID, tsSec, tsNanos int64
		var body string
		if err := rows.Scan(&logID, &tsSec, &tsNanos, &body); err != nil {
			if isOptionalCodexDBError(err) {
				return count, nil
			}
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
			Thoughts:     event.reasoning,
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
		if isOptionalCodexDBError(err) {
			return count, nil
		}
		return count, err
	}
	if maxOffset > offset {
		if err := s.db.SetOffset(OffsetKey, maxOffset); err != nil {
			return count, err
		}
	}
	return count, nil
}

type codexSessionLine struct {
	Timestamp string              `json:"timestamp"`
	Type      string              `json:"type"`
	Payload   codexSessionPayload `json:"payload"`
}

type codexSessionPayload struct {
	ID    string            `json:"id"`
	CWD   string            `json:"cwd"`
	Model string            `json:"model"`
	Type  string            `json:"type"`
	Info  *codexSessionInfo `json:"info"`
}

type codexSessionInfo struct {
	TotalTokenUsage codexSessionTokenUsage `json:"total_token_usage"`
}

type codexSessionTokenUsage struct {
	InputTokens           int `json:"input_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
}

type codexSessionTotal struct {
	id        string
	cwd       string
	model     string
	ts        time.Time
	tokens    codexSessionTokenUsage
	hasTokens bool
}

func (s *Scanner) scanSessions(threads map[string]threadInfo, usageChan chan<- watcher.TokenUsage) (int, bool, error) {
	if s.SessionsDir == "" {
		return 0, false, nil
	}
	if _, err := os.Stat(s.SessionsDir); err != nil {
		return 0, false, nil
	}

	count := 0
	hasSessionTotals := false
	err := filepath.WalkDir(s.SessionsDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		n, hasTotals, err := s.scanSessionFile(path, threads, usageChan)
		if err != nil {
			return err
		}
		count += n
		if hasTotals {
			hasSessionTotals = true
		}
		return nil
	})
	if err != nil {
		return count, hasSessionTotals, err
	}
	return count, hasSessionTotals, nil
}

func (s *Scanner) scanSessionFile(path string, threads map[string]threadInfo, usageChan chan<- watcher.TokenUsage) (int, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false, nil
	}
	offsetKey := "codex:session:" + filepath.ToSlash(path)
	offset, err := s.db.GetOffset(offsetKey)
	if err != nil {
		return 0, false, err
	}
	if info.Size() == offset && offset > 0 {
		return 0, true, nil
	}

	total, err := parseCodexSessionFile(path, info.ModTime())
	if err != nil || !total.hasTokens {
		return 0, false, err
	}
	if thread, ok := threads[total.id]; ok {
		if total.model == "" {
			total.model = thread.model
		}
		if total.cwd == "" {
			total.cwd = thread.cwd
		}
	}
	if total.model == "" {
		total.model = "codex"
	}
	if total.id == "" {
		total.id = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	project := watcher.ExtractProjectNameFromCWD(total.cwd)
	if project == "" {
		project = "Default"
	}
	usage := model.TokenUsage{
		Source:       SourceName,
		Model:        total.model,
		Project:      project,
		InputTokens:  total.tokens.InputTokens,
		CachedTokens: total.tokens.CachedInputTokens,
		OutputTokens: total.tokens.OutputTokens,
		Thoughts:     total.tokens.ReasoningOutputTokens,
		Timestamp:    total.ts,
		UUID:         "codex-session:" + total.id,
	}
	cost, _ := s.calc.CalculateCost(usage.Model, usage.InputTokens, usage.CachedTokens, usage.CacheCreationTokens, usage.OutputTokens)
	if err := s.db.InsertUsageWithTime(usage, cost, usage.Timestamp, path, s.DeviceID); err != nil {
		return 0, true, err
	}
	if err := s.db.SetOffset(offsetKey, info.Size()); err != nil {
		return 0, true, err
	}
	if usageChan != nil {
		usageChan <- usage
	}
	return 1, true, nil
}

func parseCodexSessionFile(path string, fallbackTime time.Time) (codexSessionTotal, error) {
	file, err := os.Open(path)
	if err != nil {
		return codexSessionTotal{}, err
	}
	defer file.Close()

	total := codexSessionTotal{ts: fallbackTime.UTC()}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		var line codexSessionLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		switch line.Type {
		case "session_meta":
			if line.Payload.ID != "" {
				total.id = line.Payload.ID
			}
			if line.Payload.CWD != "" {
				total.cwd = line.Payload.CWD
			}
			if line.Payload.Model != "" {
				total.model = line.Payload.Model
			}
		case "event_msg":
			if line.Payload.Type != "token_count" || line.Payload.Info == nil {
				continue
			}
			tokens := line.Payload.Info.TotalTokenUsage
			if tokens.InputTokens == 0 && tokens.CachedInputTokens == 0 && tokens.OutputTokens == 0 {
				continue
			}
			total.tokens = tokens
			total.hasTokens = true
			if parsed, err := time.Parse(time.RFC3339Nano, line.Timestamp); err == nil {
				total.ts = parsed.UTC()
			}
		}
	}
	return total, scanner.Err()
}

func loadThreads(statePath string) (map[string]threadInfo, bool, error) {
	conn, err := openReadonlySQLite(statePath)
	if err != nil {
		if isOptionalCodexDBError(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer conn.Close()
	hasThreads, err := hasTable(conn, "threads")
	if err != nil {
		if isOptionalCodexDBError(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !hasThreads {
		return nil, false, nil
	}

	rows, err := conn.Query("SELECT id, COALESCE(model, ''), cwd FROM threads")
	if err != nil {
		if isOptionalCodexDBError(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer rows.Close()

	threads := make(map[string]threadInfo)
	for rows.Next() {
		var id, modelName, cwd string
		if err := rows.Scan(&id, &modelName, &cwd); err != nil {
			if isOptionalCodexDBError(err) {
				return nil, false, nil
			}
			return nil, false, err
		}
		threads[id] = threadInfo{model: modelName, cwd: cwd}
	}
	if err := rows.Err(); err != nil {
		if isOptionalCodexDBError(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return threads, true, nil
}

func parseCodexEvent(logID int64, tsSec int64, tsNanos int64, body string) (codexEvent, bool) {
	eventBody, ok := codexTelemetryEventBody(body)
	if !ok {
		return codexEvent{}, false
	}
	fields := parseMetrics(eventBody)
	if fields["event.name"] != "codex.sse_event" || fields["event.kind"] != "response.completed" {
		return codexEvent{}, false
	}
	input := parseIntField(fields, "input_token_count")
	output := parseIntField(fields, "output_token_count")
	cached := parseIntField(fields, "cached_token_count")
	reasoning := parseIntField(fields, "reasoning_token_count")
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
		reasoning:    reasoning,
	}, true
}

func codexTelemetryEventBody(body string) (string, bool) {
	if strings.HasPrefix(body, "event.name=") {
		return body, true
	}
	const prefixedEventMarker = ": event.name="
	idx := strings.Index(body, prefixedEventMarker)
	if idx == -1 {
		return "", false
	}
	return body[idx+2:], true
}

func parseMetrics(body string) map[string]string {
	fields := make(map[string]string)
	for _, match := range metricPattern.FindAllStringSubmatch(body, -1) {
		if _, exists := fields[match[1]]; exists {
			continue
		}
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

func hasTable(conn *sql.DB, name string) (bool, error) {
	var tableName string
	err := conn.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&tableName)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func isOptionalCodexDBError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "file is not a database") ||
		strings.Contains(msg, "database disk image is malformed") ||
		strings.Contains(msg, "unable to open database file") ||
		strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "no such column")
}
