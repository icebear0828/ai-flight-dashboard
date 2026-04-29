package watcher

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type TokenUsage struct {
	Source       string
	Model        string
	InputTokens  int
	CachedTokens int
	OutputTokens int
	Thoughts     int
	Timestamp    time.Time
	UUID         string // for dedup: Claude writes snapshots with same uuid
}

type Watcher struct {
	fw            *fsnotify.Watcher
	offsets       map[string]int64
	mu            sync.Mutex
	UsageChan     chan TokenUsage
	done          chan struct{}
	recursiveDirs map[string]bool // tracks dirs registered for recursive watching
	DeviceID      string
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
		done:          make(chan struct{}),
		recursiveDirs: make(map[string]bool),
		DeviceID:      deviceID,
	}
	
	go w.listen()
	return w, nil
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

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if u, ok := ParseLine(line); ok {
			w.UsageChan <- u
		}
	}

	// Update offset
	newOffset, _ := file.Seek(0, 1)
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
			model := "gemini-2.5-pro"
			if m, ok := data["model"].(string); ok {
				model = m
			}
			return TokenUsage{
				Source:       "Gemini CLI",
				Model:        model,
				InputTokens:  in,
				OutputTokens: out,
				CachedTokens: cached,
				Timestamp:    ts,
			}, true
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
	return TokenUsage{
		Source:       source,
		Model:        model,
		InputTokens:  toInt(usage["input_tokens"]),
		OutputTokens: toInt(usage["output_tokens"]),
		CachedTokens: toInt(usage["cache_read_input_tokens"]),
	}
}

// isNoiseRecord filters out synthetic/empty/non-existent model records
func isNoiseRecord(u TokenUsage) bool {
	if strings.HasPrefix(u.Model, "<") {
		return true // <synthetic> etc
	}
	if u.InputTokens == 0 && u.OutputTokens == 0 && u.CachedTokens == 0 {
		return true
	}
	// Non-existent model names that appear in logs
	switch u.Model {
	case "claude-sonnet-4-20250514", "gemini-3-flash":
		return true
	}
	return false
}

func parseTimestamp(data map[string]interface{}) time.Time {
	if ts, ok := data["timestamp"].(string); ok {
		// Try ISO8601 with Z
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			return t
		}
		// Try ISO8601 with milliseconds
		if t, err := time.Parse("2006-01-02T15:04:05.000Z", ts); err == nil {
			return t
		}
		if t, err := time.Parse("2006-01-02T15:04:05.999Z", ts); err == nil {
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
