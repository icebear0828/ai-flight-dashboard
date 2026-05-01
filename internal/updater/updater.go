package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// GitHubRepo is the repository to check for releases.
// Override at build time via ldflags if needed.
var GitHubRepo = "icebear0828/ai-flight-dashboard"

type Release struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
}

func CheckForUpdates(currentVersion string) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", GitHubRepo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to check for updates: status %d", resp.StatusCode)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}

	// Normalize "v" prefix for comparison (e.g. "v1.2.0" vs "1.2.0")
	remoteVersion := strings.TrimPrefix(rel.TagName, "v")
	localVersion := strings.TrimPrefix(currentVersion, "v")

	if remoteVersion != "" && remoteVersion != localVersion {
		return &rel, nil
	}

	return nil, nil
}

func ApplyUpdate() error {
	// Stub for actual binary replacement (e.g. using minio/selfupdate)
	// In a real scenario, this would download the appropriate asset from the Release
	// and apply the binary patch.
	return fmt.Errorf("OTA updates are not yet implemented for private repositories without a token")
}
