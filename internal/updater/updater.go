package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Release struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
}

func CheckForUpdates(currentVersion string) (*Release, error) {
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/icebear0828/ai-flight-dashboard/releases/latest", nil)
	
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

	// Simple check, in reality we'd use semver
	if rel.TagName != "" && rel.TagName != currentVersion {
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
