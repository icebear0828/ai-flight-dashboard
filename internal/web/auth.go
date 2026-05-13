package web

import (
	"net"
	"net/http"
	"strings"
)

// tailscaleCGNAT is RFC 6598 Shared Address Space, which Tailscale uses by
// default for its tailnet (100.64.0.0/10). Go's net.IP.IsPrivate() follows
// RFC 1918 strictly and excludes CGNAT, so peers on a tailnet would be
// rejected by isPrivateLANRemote without this explicit allowlist.
var (
	tailscaleCGNATv4 = mustCIDR("100.64.0.0/10")
	tailscaleULAv6   = mustCIDR("fd7a:115c:a1e0::/48")
)

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

func authMiddleware(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			authHeader := r.Header.Get("Authorization")
			if authHeader != "Bearer "+token {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func syncAuthMiddleware(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			if !isPrivateLANRemote(r.RemoteAddr) {
				http.Error(w, "LAN sync token required", http.StatusUnauthorized)
				return
			}
			next(w, r)
			return
		}
		authMiddleware(token, next)(w, r)
	}
}

func isPrivateLANRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	if zoneAt := strings.IndexByte(host, '%'); zoneAt >= 0 {
		host = host[:zoneAt]
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	// Tailscale and other tailnets use RFC 6598 100.64.0.0/10 (CGNAT) and
	// Tailscale-specific IPv6 ULA fd7a:115c:a1e0::/48. Go's IsPrivate() does
	// not recognize either, so zero-config sync between two tailnet peers
	// is rejected with 401 unless we accept these ranges explicitly.
	if v4 := ip.To4(); v4 != nil {
		return tailscaleCGNATv4.Contains(v4)
	}
	return tailscaleULAv6.Contains(ip)
}
