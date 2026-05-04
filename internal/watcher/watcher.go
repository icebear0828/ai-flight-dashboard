package watcher

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ai-flight-dashboard/internal/model"

	"github.com/fsnotify/fsnotify"
)

// TokenUsage is an alias for model.TokenUsage.
// Retained for backward compatibility with existing consumers.
type TokenUsage = model.TokenUsage

type Watcher struct {
	fw            *fsnotify.Watcher
	offsets       map[string]int64
	mu            sync.Mutex
	UsageChan     chan TokenUsage
	BroadcastChan chan TokenUsage // For LAN broadcasting of real-time events
	done          chan struct{}
	recursiveDirs map[string]bool // tracks dirs registered for recursive watching
	DeviceID      string
	paused        atomic.Bool
}

func New(deviceID string) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	
	w := &Watcher{
		fw:            fw,
		offsets:       make(map[string]int64),
		UsageChan:     make(chan TokenUsage, 100),
		BroadcastChan: make(chan TokenUsage, 100),
		done:          make(chan struct{}),
		recursiveDirs: make(map[string]bool),
		DeviceID:      deviceID,
	}
	
	go w.listen()
	return w, nil
}

func (w *Watcher) IsPaused() bool {
	return w.paused.Load()
}

func (w *Watcher) SetPaused(p bool) {
	w.paused.Store(p)
}

func (w *Watcher) WatchDir(dir string) error {
	return w.fw.Add(dir)
}

// WatchDirRecursive walks all existing subdirs and watches them.
// New subdirs created later are auto-registered in listen().
func (w *Watcher) WatchDirRecursive(dir string) error {
	w.mu.Lock()
	w.recursiveDirs[dir] = true
	w.mu.Unlock()

	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible dirs
		}
		if d.IsDir() {
			return w.fw.Add(path)
		}
		return nil
	})
}

// UnwatchDir removes a directory and all its existing subdirectories from the watcher.
func (w *Watcher) UnwatchDir(dir string) error {
	w.mu.Lock()
	delete(w.recursiveDirs, dir)
	w.mu.Unlock()

	if err := w.fw.Remove(dir); err != nil {
		log.Printf("watcher: failed to unwatch %s: %v", dir, err)
	}

	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			if rmErr := w.fw.Remove(path); rmErr != nil {
				log.Printf("watcher: failed to unwatch %s: %v", path, rmErr)
			}
		}
		return nil
	})
}

// WatchKnownDirs registers specific directories without recursive walk.
// Used with cached directory lists for fast startup.
func (w *Watcher) WatchKnownDirs(dirs []string) {
	for _, dir := range dirs {
		w.fw.Add(dir)
	}
}

func (w *Watcher) Close() error {
	close(w.done)
	return w.fw.Close()
}

func (w *Watcher) listen() {
	for {
		select {
		case <-w.done:
			return
		case event, ok := <-w.fw.Events:
			if !ok {
				return
			}
			// Auto-register new subdirectories for recursive watches
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if w.isUnderRecursiveRoot(event.Name) {
						w.fw.Add(event.Name)
					}
				}
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				w.processFile(event.Name)
			}
		case err, ok := <-w.fw.Errors:
			if !ok {
				return
			}
			log.Println("watcher error:", err)
		}
	}
}

// isUnderRecursiveRoot checks if path falls under any recursiveDirs root.
func (w *Watcher) isUnderRecursiveRoot(path string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for root := range w.recursiveDirs {
		if strings.HasPrefix(path, root) {
			return true
		}
	}
	return false
}

// processFile reads the new lines of the file
func (w *Watcher) processFile(path string) {
	if !strings.HasSuffix(path, ".jsonl") {
		return // only process jsonl for now
	}

	w.mu.Lock()
	offset := w.offsets[path]
	w.mu.Unlock()

	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return
	}
	
	if info.Size() < offset {
		// file truncated or rotated
		offset = 0
	}

	_, err = file.Seek(offset, 0)
	if err != nil {
		return
	}

	projectName := ExtractProjectName(path)

	reader := bufio.NewReader(file)
	newOffset := offset

	for {
		lineBytes, err := reader.ReadBytes('\n')
		if len(lineBytes) > 0 && lineBytes[len(lineBytes)-1] == '\n' {
			// Complete line
			newOffset += int64(len(lineBytes))
			line := string(lineBytes)
			if w.IsPaused() {
				continue // skip parsing and recording while paused
			}
			if u, ok := ParseLine(line); ok {
				u.Project = projectName
				w.UsageChan <- u
				select {
				case w.BroadcastChan <- u:
				default:
				}
			}
		} else {
			// Partial line at EOF
			break
		}
		if err != nil {
			break
		}
	}

	// Update offset
	w.mu.Lock()
	w.offsets[path] = newOffset
	w.mu.Unlock()
}

// ParseLine parses a single JSONL line and returns a TokenUsage if it contains usage data.
// Exported so Scanner can reuse it.
func ParseLine(line string) (TokenUsage, bool) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line), &data); err != nil {
		return TokenUsage{}, false
	}

	ts := parseTimestamp(data)

	// Real Claude Code format: {"message": {"type": "message", "model": "...", "usage": {...}}}
	if msg, ok := data["message"].(map[string]interface{}); ok {
		if t, ok := msg["type"].(string); ok && t == "message" {
			if usage, ok := msg["usage"].(map[string]interface{}); ok {
				u := parseClaudeUsage(msg, usage)
				if isNoiseRecord(u) {
					return TokenUsage{}, false
				}
				if ts.IsZero() {
					ts = parseTimestamp(msg)
				}
				u.Timestamp = ts
				if uuid, ok := data["uuid"].(string); ok {
					u.UUID = uuid
				}
				return u, true
			}
		}
	}

	// Legacy/manual format: {"type": "assistant", "usage": {...}}
	if t, ok := data["type"].(string); ok && t == "assistant" {
		if usage, ok := data["usage"].(map[string]interface{}); ok {
			u := parseClaudeUsage(data, usage)
			u.Timestamp = ts
			return u, true
		}
	}
	
	// Gemini log check
	if t, ok := data["type"].(string); ok && t == "gemini" {
		if tokens, ok := data["tokens"].(map[string]interface{}); ok {
			in := toInt(tokens["input"])
			out := toInt(tokens["output"])
			cached := toInt(tokens["cached"])
			thoughts := toInt(tokens["thoughts"])
			tool := toInt(tokens["tool"])
			model := "gemini-2.5-pro"
			if m, ok := data["model"].(string); ok {
				model = m
			}
			u := TokenUsage{
				Source:       "Gemini CLI",
				Model:        model,
				InputTokens:  in,
				OutputTokens: out + thoughts + tool,
				CachedTokens: cached,
				Thoughts:     thoughts,
				Timestamp:    ts,
			}
			if id, ok := data["id"].(string); ok {
				u.UUID = id
			}
			return u, true
		}
	}

	return TokenUsage{}, false
}

func parseClaudeUsage(container map[string]interface{}, usage map[string]interface{}) TokenUsage {
	model := "claude-3-7-sonnet-20250219"
	if m, ok := container["model"].(string); ok {
		model = m
	}
	// Auto-detect source: gemini models called by Claude subagents should be classified as Gemini
	source := "Claude Code"
	if strings.HasPrefix(model, "gemini") {
		source = "Gemini CLI"
	}
	// Claude API: input_tokens and cache_read_input_tokens are INDEPENDENT fields.
	// (Unlike Gemini, where input INCLUDES cached.)
	// We store InputTokens = input + cache_read so that Calculator's formula
	// (baseInput = InputTokens - CachedTokens) yields the correct new-input cost.
	cached := toInt(usage["cache_read_input_tokens"])
	cacheCreation := toInt(usage["cache_creation_input_tokens"])
	return TokenUsage{
		Source:              source,
		Model:               model,
		InputTokens:         toInt(usage["input_tokens"]) + cached + cacheCreation,
		OutputTokens:        toInt(usage["output_tokens"]),
		CachedTokens:        cached,
		CacheCreationTokens: cacheCreation,
	}
}

// isNoiseRecord filters out synthetic/empty model records
func isNoiseRecord(u TokenUsage) bool {
	if strings.HasPrefix(u.Model, "<") {
		return true // <synthetic> etc
	}
	if u.InputTokens == 0 && u.OutputTokens == 0 && u.CachedTokens == 0 && u.CacheCreationTokens == 0 {
		return true
	}
	return false
}

func parseTimestamp(data map[string]interface{}) time.Time {
	if ts, ok := data["timestamp"].(string); ok {
		// Try ISO8601 / RFC3339
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			return t
		}
		// Try ISO8601 with milliseconds (e.g. Gemini logs)
		if t, err := time.Parse("2006-01-02T15:04:05.000Z", ts); err == nil {
			return t
		}
		// Try RFC3339Nano for nanosecond precision
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			return t
		}
	}
	return time.Time{}
}

func toInt(v interface{}) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}

func ExtractProjectName(path string) string {
	path = filepath.ToSlash(path) // Normalize to forward slashes for Windows support
	parts := strings.Split(path, ".claude/projects/")
	if len(parts) > 1 {
		folderName := strings.Split(parts[1], "/")[0]
		if strings.HasPrefix(folderName, "-Users-c-") {
			return folderName[9:]
		}
		if strings.HasPrefix(folderName, "Users-c-") {
			return folderName[8:]
		}
		return folderName
	}
	
	// Gemini CLI: ~/.gemini/tmp/<project-name>/chats/session-*.jsonl
	if parts := strings.Split(path, ".gemini/tmp/"); len(parts) > 1 {
		segments := strings.Split(parts[1], "/")
		if len(segments) > 0 && segments[0] != "" {
			return segments[0]
		}
	}
	if strings.Contains(path, ".gemini/") {
		return "Gemini"
	}

	return "Default"
}
