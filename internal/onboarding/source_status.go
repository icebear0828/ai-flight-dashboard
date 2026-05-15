package onboarding

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
)

type Options struct {
	HomeDir  string
	DeviceID string
}

type sourceDefinition struct {
	source      string
	displayName string
	paths       []string
	unsupported bool
	reason      string
}

var errEvidenceFound = errors.New("source evidence found")

func DefaultOptions() Options {
	home, _ := os.UserHomeDir()
	return Options{HomeDir: home}
}

func BuildSourceCoverage(database *db.DB, opts Options) (model.SourceCoverageResponse, error) {
	if strings.TrimSpace(opts.HomeDir) == "" {
		defaults := DefaultOptions()
		opts.HomeDir = defaults.HomeDir
	}

	stats, err := database.QuerySourceCoverageStats(opts.DeviceID)
	if err != nil {
		return model.SourceCoverageResponse{}, err
	}
	statsBySource := make(map[string]db.SourceCoverageStat, len(stats))
	for _, stat := range stats {
		statsBySource[stat.Source] = stat
	}

	sources := make([]model.SourceCoverage, 0, len(sourceDefinitions(opts.HomeDir)))
	for _, def := range sourceDefinitions(opts.HomeDir) {
		sources = append(sources, buildSourceCoverage(def, statsBySource[def.source]))
	}
	sort.SliceStable(sources, func(i, j int) bool {
		return sourceSortOrder(sources[i].Source) < sourceSortOrder(sources[j].Source)
	})

	return model.SourceCoverageResponse{Sources: sources}, nil
}

func sourceDefinitions(home string) []sourceDefinition {
	return []sourceDefinition{
		{
			source:      "Claude Code",
			displayName: "Claude Code",
			paths: []string{
				filepath.Join(home, ".claude", "projects"),
				filepath.Join(home, ".config", "claude", "projects"),
			},
		},
		{
			source:      "Codex",
			displayName: "Codex",
			paths:       []string{filepath.Join(home, ".codex", "sessions")},
		},
		{
			source:      "Gemini CLI",
			displayName: "Gemini CLI",
			paths:       []string{filepath.Join(home, ".gemini", "tmp")},
		},
		{
			source:      "Antigravity",
			displayName: "Antigravity",
			paths:       []string{filepath.Join(home, ".gemini", "antigravity")},
			unsupported: true,
			reason:      "Antigravity does not currently persist token usage in local logs that Token Ray can import.",
		},
	}
}

func buildSourceCoverage(def sourceDefinition, stat db.SourceCoverageStat) model.SourceCoverage {
	dataDir := preferredDataDir(def.paths)
	coverage := model.SourceCoverage{
		Source:      def.source,
		DisplayName: def.displayName,
		DataDir:     dataDir,
		Records:     stat.Records,
		TotalCost:   stat.TotalCost,
	}
	if !stat.LastSeen.IsZero() {
		coverage.LastSeen = &stat.LastSeen
	}

	if def.unsupported {
		coverage.Status = "unsupported"
		coverage.Health = "unsupported"
		coverage.Reason = def.reason
		return coverage
	}

	if stat.Records > 0 {
		coverage.Status = "watching"
		coverage.Health = "complete"
		coverage.Reason = "Usage records are present in the local Token Ray ledger."
		return coverage
	}

	detected, hasEvidence, permissionBlocked := inspectSourcePaths(def.paths)
	if permissionBlocked {
		coverage.Status = "needs_permission"
		coverage.Health = "needs_permission"
		coverage.Reason = "Token Ray cannot read the source data directory."
		return coverage
	}
	if detected && hasEvidence {
		coverage.Status = "detected"
		coverage.Health = "pending_import"
		coverage.Reason = "Source logs were found but have not been imported yet."
		return coverage
	}
	if detected {
		coverage.Status = "no_data"
		coverage.Health = "unavailable"
		coverage.Reason = "Source directory exists but no JSONL usage logs were found."
		return coverage
	}

	coverage.Status = "no_data"
	coverage.Health = "unavailable"
	coverage.Reason = "Default source data directory was not found."
	return coverage
}

func preferredDataDir(paths []string) string {
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

func inspectSourcePaths(paths []string) (detected bool, hasEvidence bool, permissionBlocked bool) {
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsPermission(err) {
				return false, false, true
			}
			continue
		}
		if !info.IsDir() {
			continue
		}
		detected = true
		found, blocked := containsJSONL(path)
		if blocked {
			return true, false, true
		}
		if found {
			return true, true, false
		}
	}
	return detected, false, false
}

func containsJSONL(root string) (bool, bool) {
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsPermission(walkErr) {
				return walkErr
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".jsonl") {
			return errEvidenceFound
		}
		return nil
	})
	if errors.Is(err, errEvidenceFound) {
		return true, false
	}
	if err != nil && os.IsPermission(err) {
		return false, true
	}
	return false, false
}

func sourceSortOrder(source string) int {
	switch source {
	case "Claude Code":
		return 0
	case "Codex":
		return 1
	case "Gemini CLI":
		return 2
	case "Antigravity":
		return 3
	default:
		return 100
	}
}
