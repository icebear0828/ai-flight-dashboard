package web

import "testing"

func TestIsPrivateLANRemote_AcceptsTailnetRanges(t *testing.T) {
	cases := []struct {
		name string
		addr string
		want bool
	}{
		// RFC 1918 + loopback + link-local (existing behavior).
		{"loopback v4", "127.0.0.1:1234", true},
		{"loopback v6", "[::1]:1234", true},
		{"rfc1918 10/8", "10.0.0.5:1234", true},
		{"rfc1918 192.168/16", "192.168.1.5:1234", true},
		{"rfc1918 172.16/12", "172.16.0.5:1234", true},
		{"link-local v4", "169.254.1.5:1234", true},

		// New: Tailscale CGNAT (RFC 6598) and Tailscale ULA must count as LAN.
		{"tailscale cgnat low", "100.64.0.1:1234", true},
		{"tailscale cgnat mid", "100.96.42.7:1234", true},
		{"tailscale cgnat high", "100.127.255.254:1234", true},
		{"tailscale ula v6", "[fd7a:115c:a1e0::1]:1234", true},
		{"tailscale ula v6 with zone", "[fd7a:115c:a1e0::5%utun3]:1234", true},

		// Public ranges must still be rejected.
		{"public v4", "1.1.1.1:1234", false},
		{"just outside cgnat low", "100.63.255.255:1234", false},
		{"just outside cgnat high", "100.128.0.1:1234", false},
		{"public v6", "[2606:4700:4700::1111]:1234", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isPrivateLANRemote(c.addr); got != c.want {
				t.Errorf("isPrivateLANRemote(%q) = %v, want %v", c.addr, got, c.want)
			}
		})
	}
}
