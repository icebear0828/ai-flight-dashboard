package web

import (
	"net"
	"net/http"
	"strings"
)

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
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
