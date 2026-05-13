package lan

import (
	"reflect"
	"testing"
)

func TestParseTailscalePeerIPv4s_OnlyOnlineIPv4Sorted(t *testing.T) {
	peers := map[string]*tailscalePeer{
		"k1": {
			HostName:     "online-mac",
			TailscaleIPs: []string{"100.64.0.5", "fd7a:115c:a1e0::5"},
			Online:       true,
		},
		"k2": {
			HostName:     "offline-linux",
			TailscaleIPs: []string{"100.64.0.99"},
			Online:       false,
		},
		"k3": {
			HostName:     "online-windows",
			TailscaleIPs: []string{"100.64.0.2"},
			Online:       true,
		},
		"k4": nil,
	}

	got := parseTailscalePeerIPv4s(peers)
	want := []string{"100.64.0.2", "100.64.0.5"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseTailscalePeerIPv4s = %v, want %v", got, want)
	}
}

func TestParseTailscalePeerIPv4s_EmptyAndNil(t *testing.T) {
	if got := parseTailscalePeerIPv4s(nil); got != nil {
		t.Errorf("nil peers should return nil, got %v", got)
	}
	if got := parseTailscalePeerIPv4s(map[string]*tailscalePeer{}); got != nil {
		t.Errorf("empty peers should return nil, got %v", got)
	}
}

func TestParseTailscalePeerIPv4s_Dedup(t *testing.T) {
	peers := map[string]*tailscalePeer{
		"a": {TailscaleIPs: []string{"100.64.0.1"}, Online: true},
		"b": {TailscaleIPs: []string{"100.64.0.1"}, Online: true},
	}
	got := parseTailscalePeerIPv4s(peers)
	if len(got) != 1 || got[0] != "100.64.0.1" {
		t.Errorf("expected single deduped entry, got %v", got)
	}
}

func TestNormalizeExtraHost(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"   ", ""},
		{"100.64.0.5", "100.64.0.5"},
		{" 100.64.0.5 ", "100.64.0.5"},
		{"http://100.64.0.5:19100", "100.64.0.5"},
		{"https://mac-mini.tailnet.ts.net/path", "mac-mini.tailnet.ts.net"},
		{"mac-mini.tailnet.ts.net:19100", "mac-mini.tailnet.ts.net"},
		{"mac-mini.tailnet.ts.net", "mac-mini.tailnet.ts.net"},
		{"mac-mini.tailnet.ts.net.", "mac-mini.tailnet.ts.net"},
		{"http://", ""},
	}
	for _, c := range cases {
		if got := normalizeExtraHost(c.in); got != c.want {
			t.Errorf("normalizeExtraHost(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeExtraPeerHosts_DedupAndOrder(t *testing.T) {
	in := []string{
		" 100.64.0.5 ",
		"http://100.64.0.5:19100",
		"mac.tailnet.ts.net",
		"",
		"MAC.tailnet.ts.net", // case-sensitive: kept as distinct entry
	}
	got := NormalizeExtraPeerHosts(in)
	want := []string{"100.64.0.5", "mac.tailnet.ts.net", "MAC.tailnet.ts.net"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSetExtraPeerHosts_RoundTrip(t *testing.T) {
	l := New("dev", DefaultHTTPPort)
	l.SetExtraPeerHosts([]string{"100.64.0.5", "  http://10.0.0.2:19100/foo  ", "", "100.64.0.5"})
	l.SetTailscaleDiscovery(true)
	static, ts := l.extraHostsSnapshot()
	if !ts {
		t.Errorf("tailscale flag should be true")
	}
	want := []string{"100.64.0.5", "10.0.0.2"}
	if !reflect.DeepEqual(static, want) {
		t.Errorf("static hosts = %v, want %v", static, want)
	}
}

func TestDiscoveryHosts_MergesAndDeduplicates(t *testing.T) {
	l := New("dev", DefaultHTTPPort)
	l.SetExtraPeerHosts([]string{"100.64.0.5", "10.99.99.99"})
	// Leave tailscale discovery off so the test does not shell out to a
	// real CLI; we only verify subnet+static merging here.
	l.SetTailscaleDiscovery(false)

	hosts := l.discoveryHosts(nil)

	// Static hosts must be present.
	seen := map[string]bool{}
	for _, h := range hosts {
		seen[h] = true
	}
	if !seen["100.64.0.5"] || !seen["10.99.99.99"] {
		t.Errorf("expected static hosts in discovery list, got %v", hosts)
	}

	// Deduplication: pass the same static again and confirm count stable.
	l.SetExtraPeerHosts([]string{"100.64.0.5", "100.64.0.5"})
	hosts2 := l.discoveryHosts(nil)
	count := 0
	for _, h := range hosts2 {
		if h == "100.64.0.5" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("static host duplicated in discovery list, count=%d", count)
	}
}
