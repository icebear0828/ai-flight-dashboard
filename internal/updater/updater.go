package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/minio/selfupdate"
)

// GitHubRepo is the repository to check for releases.
// Override at build time via ldflags if needed.
var GitHubRepo = "icebear0828/ai-flight-dashboard"

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"url"` // API URL for downloading
}

type Release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	Body    string  `json:"body"`
	Assets  []Asset `json:"assets"`
}

func CheckForUpdates(currentVersion, token string) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", GitHubRepo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
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

// ApplyUpdate attempts to download the matching asset and replace the running binary
func ApplyUpdate(release *Release, token string) error {
	// Look for asset name like "dashboard-darwin-arm64" or "dashboard-windows-amd64.exe"
	// Also accept "ai-flight-dashboard-..."
	targetSubstrings := []string{runtime.GOOS, runtime.GOARCH}
	
	var downloadURL string
	for _, asset := range release.Assets {
		// Ignore compressed bundles for in-place binary updates
		if strings.HasSuffix(asset.Name, ".zip") || strings.HasSuffix(asset.Name, ".dmg") || strings.HasSuffix(asset.Name, ".tar.gz") {
			continue
		}
		
		matches := true
		for _, sub := range targetSubstrings {
			if !strings.Contains(asset.Name, sub) {
				matches = false
				break
			}
		}
		if matches {
			downloadURL = asset.URL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no suitable binary asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}

	// Request raw binary from GitHub API
	req.Header.Set("Accept", "application/octet-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// Timeout long enough for a binary download
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code downloading update: %d", resp.StatusCode)
	}

	// Apply the update in-place
	err = selfupdate.Apply(resp.Body, selfupdate.Options{})
	if err != nil {
		return fmt.Errorf("failed to apply update: %w", err)
	}

	return nil
}
