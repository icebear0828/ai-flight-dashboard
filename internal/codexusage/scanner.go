package codexusage

import (
	"os"
	"path/filepath"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/watcher"
)

const (
	SourceName = "Codex"
	OffsetKey  = "codex:logs_2.sqlite:id"
)

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
