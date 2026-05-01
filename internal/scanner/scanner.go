package scanner

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/watcher"
)

type Scanner struct {
	db       *db.DB
	calc     *calculator.Calculator
	DeviceID string
}

func New(database *db.DB, calc *calculator.Calculator, deviceID string) *Scanner {
	return &Scanner{db: database, calc: calc, DeviceID: deviceID}
}

// ScanAll walks all dirs, finds .jsonl files, reads from last offset, parses usage, inserts into DB.
// Returns the number of new usage records inserted.
func (s *Scanner) ScanAll(dirs []string, usageChan chan<- watcher.TokenUsage) (int, error) {
	total := 0
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // skip inaccessible
			}
			if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
				return nil
			}
			n, err := s.scanFile(path, usageChan)
			if err != nil {
				return nil // skip broken files, don't abort scan
			}
			total += n
			return nil
		})
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// ScanKnownFiles reads all known file paths from the database and directly stats them.
// This is extremely fast (<1ms) compared to ScanAll, avoiding directory traversal overhead.
func (s *Scanner) ScanKnownFiles(usageChan chan<- watcher.TokenUsage) (int, error) {
	files, err := s.db.ListKnownFiles()
	if err != nil {
		return 0, err
	}
	
	total := 0
	for _, path := range files {
		// Only check if it exists and hasn't shrunk/disappeared
		info, err := os.Stat(path)
		if err != nil {
			continue // skip broken files
		}
		
		// Optimization: Check offset here to avoid opening file if no new data
		offset, err := s.db.GetOffset(path)
		if err != nil {
			continue
		}
		if info.Size() == offset {
			continue
		}
		
		n, _ := s.scanFile(path, usageChan)
		total += n
	}
	return total, nil
}

func (s *Scanner) scanFile(path string, usageChan chan<- watcher.TokenUsage) (int, error) {
	offset, err := s.db.GetOffset(path)
	if err != nil {
		return 0, err
	}

	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return 0, err
	}

	if info.Size() < offset {
		offset = 0 // file was truncated/rotated, rescan from start
	}
	if info.Size() == offset {
		return 0, nil // no new data
	}

	if _, err := file.Seek(offset, 0); err != nil {
		return 0, err
	}

	// First pass: collect all entries, dedup by UUID (keep last = final token counts)
	type entry struct {
		u    watcher.TokenUsage
		cost float64
		ts   time.Time
	}
	uuidMap := make(map[string]entry)  // uuid -> last entry
	var noUUID []entry                  // entries without uuid

	projectName := watcher.ExtractProjectName(path)

	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		u, ok := watcher.ParseLine(line)
		if !ok {
			continue
		}
		u.Project = projectName
		cost, _ := s.calc.CalculateCost(u.Model, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens)
		ts := u.Timestamp
		if ts.IsZero() {
			ts = info.ModTime()
		}
		e := entry{u: u, cost: cost, ts: ts}
		if u.UUID != "" {
			uuidMap[u.UUID] = e // overwrite = keep last
		} else {
			noUUID = append(noUUID, e)
		}
	}

	// Insert deduplicated entries
	count := 0
	for _, e := range uuidMap {
		if err := s.db.InsertUsageWithTime(e.u, e.cost, e.ts, path, s.DeviceID); err != nil {
			continue
		}
		if usageChan != nil {
			usageChan <- e.u
		}
		count++
	}
	for _, e := range noUUID {
		if err := s.db.InsertUsageWithTime(e.u, e.cost, e.ts, path, s.DeviceID); err != nil {
			continue
		}
		if usageChan != nil {
			usageChan <- e.u
		}
		count++
	}

	// Update offset to current position
	newOffset, _ := file.Seek(0, 1)
	s.db.SetOffset(path, newOffset)

	// Cache this directory for fast startup next time
	s.db.UpsertKnownDir(filepath.Dir(path))

	return count, nil
}
