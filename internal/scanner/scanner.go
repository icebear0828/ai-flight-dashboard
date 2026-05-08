package scanner

import (
	"bufio"
	"io"
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
	return s.scanAll(dirs, usageChan, false)
}

// ScanAllStrict is used by repair flows where silently skipping a replay file
// could leave repaired history incomplete.
func (s *Scanner) ScanAllStrict(dirs []string, usageChan chan<- watcher.TokenUsage) (int, error) {
	return s.scanAll(dirs, usageChan, true)
}

func (s *Scanner) scanAll(dirs []string, usageChan chan<- watcher.TokenUsage, strict bool) (int, error) {
	total := 0
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				if strict {
					return err
				}
				return nil // skip inaccessible
			}
			if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
				return nil
			}
			n, err := s.scanFile(path, usageChan, strict)
			if err != nil {
				if strict {
					return err
				}
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

		n, _ := s.scanFile(path, usageChan, false)
		total += n
	}
	return total, nil
}

func (s *Scanner) scanFile(path string, usageChan chan<- watcher.TokenUsage, processPartialEOF bool) (int, error) {
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
	uuidMap := make(map[string]entry) // uuid -> last entry
	var noUUID []entry                // entries without uuid

	projectName := watcher.ExtractProjectName(path)
	currentProject := projectName

	reader := bufio.NewReader(file)
	newOffset := offset

	for {
		lineBytes, err := reader.ReadBytes('\n')
		completeLine := len(lineBytes) > 0 && lineBytes[len(lineBytes)-1] == '\n'
		partialEOF := processPartialEOF && len(lineBytes) > 0 && err == io.EOF
		if completeLine || partialEOF {
			lineStartOffset := newOffset
			line := string(lineBytes)
			if project := watcher.ExtractProjectNameFromLine(line); project != "" {
				currentProject = project
			}
			u, ok := watcher.ParseLine(line)
			if ok {
				newOffset += int64(len(lineBytes))
				u = watcher.WithProjectFallback(u, currentProject)
				if u.Source == "Gemini CLI" && u.UUID == "" {
					u.UUID = watcher.StableGeminiUUID(s.DeviceID, path, lineStartOffset)
				}
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
			} else if completeLine {
				newOffset += int64(len(lineBytes))
			}
		} else {
			// Partial line at EOF, don't advance offset, just break and wait for more
			break
		}
		if err != nil {
			break
		}
	}

	// Insert deduplicated entries
	count := 0
	for _, e := range uuidMap {
		if err := s.db.InsertUsageWithTime(e.u, e.cost, e.ts, path, s.DeviceID); err != nil {
			return count, err
		}
		if usageChan != nil {
			usageChan <- e.u
		}
		count++
	}
	for _, e := range noUUID {
		if err := s.db.InsertUsageWithTime(e.u, e.cost, e.ts, path, s.DeviceID); err != nil {
			return count, err
		}
		if usageChan != nil {
			usageChan <- e.u
		}
		count++
	}

	// Cache this directory for fast startup next time
	if err := s.db.UpsertKnownDir(filepath.Dir(path)); err != nil {
		return count, err
	}

	// Update offset to the end of the last complete line only after inserts succeed.
	if err := s.db.SetOffset(path, newOffset); err != nil {
		return count, err
	}

	return count, nil
}
