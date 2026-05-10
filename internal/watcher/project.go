package watcher

import (
	"encoding/json"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

// ExtractProjectNameFromLine returns a cwd-derived project from any JSONL row.
// It is used to carry project metadata from non-usage rows to later usage rows
// in the same file.
func ExtractProjectNameFromLine(line string) string {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line), &data); err != nil {
		return ""
	}

	containers := []map[string]interface{}{data}
	if msg, ok := data["message"].(map[string]interface{}); ok {
		containers = append(containers, msg)
	}
	return projectNameFromCWDFields(containers...)
}

func ExtractProjectName(path string) string {
	path = filepath.ToSlash(path)
	path = strings.ReplaceAll(path, "\\", "/")
	parts := strings.Split(path, ".claude/projects/")
	if len(parts) > 1 {
		folderName := strings.Split(parts[1], "/")[0]
		return normalizeClaudeProjectFolder(folderName)
	}

	if parts := strings.Split(path, ".gemini/tmp/"); len(parts) > 1 {
		segments := strings.Split(parts[1], "/")
		if len(segments) > 0 && segments[0] != "" {
			if project := geminiProjectFromRootFile(path, segments[0]); project != "" {
				return project
			}
			return segments[0]
		}
	}
	if strings.Contains(path, ".gemini/") {
		return "Gemini"
	}

	return "Default"
}

func ExtractProjectNameFromCWD(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "Default"
	}
	normalized := strings.ReplaceAll(filepath.ToSlash(cwd), "\\", "/")
	clean := pathpkg.Clean(normalized)
	base := pathpkg.Base(clean)
	if base == "." || base == "/" || strings.HasSuffix(base, ":") {
		return "Default"
	}
	return base
}

func projectNameFromCWDFields(containers ...map[string]interface{}) string {
	for _, container := range containers {
		cwd, ok := container["cwd"].(string)
		if !ok || strings.TrimSpace(cwd) == "" {
			continue
		}
		project := ExtractProjectNameFromCWD(cwd)
		if project != "" && project != "Default" {
			return project
		}
	}
	return ""
}

func geminiProjectFromRootFile(filePath string, tmpProject string) string {
	normalized := strings.ReplaceAll(filepath.ToSlash(filePath), "\\", "/")
	parts := strings.Split(normalized, ".gemini/tmp/")
	if len(parts) < 2 {
		return ""
	}
	prefix := parts[0] + ".gemini/tmp/" + tmpProject
	rootFile := filepath.FromSlash(prefix + "/.project_root")
	data, err := os.ReadFile(rootFile)
	if err != nil {
		return ""
	}
	project := ExtractProjectNameFromCWD(string(data))
	if project == "Default" {
		return ""
	}
	return project
}

func normalizeClaudeProjectFolder(folderName string) string {
	if folderName == "" {
		return "Default"
	}
	trimmed := strings.TrimPrefix(folderName, "-")
	segments := strings.Split(trimmed, "-")
	if len(segments) < 2 {
		return folderName
	}

	switch segments[0] {
	case "Users", "home":
		if len(segments) == 2 {
			return segments[1]
		}
		return strings.Join(segments[2:], "-")
	default:
		if strings.HasSuffix(segments[0], ":") && segments[1] == "Users" {
			if len(segments) == 3 {
				return segments[2]
			}
			if len(segments) > 3 {
				return strings.Join(segments[3:], "-")
			}
		}
	}

	return folderName
}
