package lan

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const (
	tailscaleProbeTimeout = 3 * time.Second
)

// tailscaleStatus mirrors the subset of `tailscale status --json` we depend on.
// Field tags match the upstream JSON shape exactly.
type tailscaleStatus struct {
	BackendState string                       `json:"BackendState"`
	Peer         map[string]*tailscalePeer    `json:"Peer"`
}

type tailscalePeer struct {
	HostName     string   `json:"HostName"`
	DNSName      string   `json:"DNSName"`
	TailscaleIPs []string `json:"TailscaleIPs"`
	Online       bool     `json:"Online"`
}

// findTailscaleBinary locates the `tailscale` CLI. PATH wins; otherwise we try
// well-known install locations on macOS (App Store and standalone .pkg).
func findTailscaleBinary() string {
	if p, err := exec.LookPath("tailscale"); err == nil {
		return p
	}
	candidates := []string{
		"/usr/local/bin/tailscale",
		"/opt/homebrew/bin/tailscale",
		"/Applications/Tailscale.app/Contents/MacOS/Tailscale",
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	return ""
}

// TailscalePeerHosts returns IPv4 addresses of online Tailscale peers, or nil
// if Tailscale is not installed / not running / has no online peers.
// Errors are intentionally swallowed: this function is a best-effort augment
// on top of regular subnet discovery and should never break LAN startup.
func TailscalePeerHosts(ctx context.Context) []string {
	bin := findTailscaleBinary()
	if bin == "" {
		return nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, tailscaleProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, bin, "status", "--json", "--peers=true", "--self=false")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var status tailscaleStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return nil
	}
	if !strings.EqualFold(status.BackendState, "Running") {
		return nil
	}

	return parseTailscalePeerIPv4s(status.Peer)
}

func parseTailscalePeerIPv4s(peers map[string]*tailscalePeer) []string {
	if len(peers) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(peers))
	out := make([]string, 0, len(peers))
	for _, p := range peers {
		if p == nil || !p.Online {
			continue
		}
		for _, raw := range p.TailscaleIPs {
			ip := net.ParseIP(strings.TrimSpace(raw))
			if ip == nil || ip.To4() == nil {
				continue
			}
			s := ip.String()
			if seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// normalizeExtraHost trims whitespace and strips an accidental scheme or port.
// Returns "" if the input is unusable (empty, looks like a URL fragment, etc.).
func normalizeExtraHost(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// Strip scheme if user pasted http://host:port
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	// Strip any path
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	// Strip explicit port — discovery always uses the configured probe ports
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	s = strings.Trim(s, ".")
	if s == "" {
		return ""
	}
	return s
}

// NormalizeExtraPeerHosts cleans a user-supplied list (from config) into a
// deduped, ordered slice of hostnames/IPs suitable for HTTP probing.
func NormalizeExtraPeerHosts(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		h := normalizeExtraHost(r)
		if h == "" {
			continue
		}
		if seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	return out
}
