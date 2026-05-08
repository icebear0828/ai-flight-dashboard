package lan

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"ai-flight-dashboard/internal/model"
	"ai-flight-dashboard/internal/testutil"
)

func testPeerFromServerURL(t *testing.T, rawURL string) PeerInfo {
	t.Helper()
	host, portStr, err := net.SplitHostPort(rawURL[len("http://"):])
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return PeerInfo{ID: "remote", IP: host, HTTPPort: port, LastSeen: time.Now()}
}

func TestSyncWithPeerUpdatesDatabaseAndStatus(t *testing.T) {
	database, _ := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	records := []model.SyncRecord{
		{
			TokenUsage: model.TokenUsage{
				Source:       "Claude Code",
				Model:        "claude-opus-4-7",
				InputTokens:  1000,
				OutputTokens: 200,
				Timestamp:    now,
				UUID:         "remote-uuid-1",
			},
			CostUSD:   1.25,
			FilePath:  "/remote/session.jsonl",
			DeviceID:  "remote",
			UpdatedAt: now,
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sync/pull" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(model.SyncPullResponse{Records: records, NextUpdatedAt: now, NextAfterID: 1})
	}))
	defer server.Close()

	l := New("local", 19100)
	peer := testPeerFromServerURL(t, server.URL)
	l.RecordPeer(peer.ID, peer.IP, peer.HTTPPort)
	l.syncWithPeer(peer.ID, peer, database, "", map[string]syncCursor{})

	_, input, _, _, output, err := database.QueryPeriodStatsAll("remote", "")
	if err != nil {
		t.Fatal(err)
	}
	if input != 1000 || output != 200 {
		t.Fatalf("expected synced remote tokens, got input=%d output=%d", input, output)
	}

	l.mu.RLock()
	status := l.activePeers["remote"].SyncStatus
	errMsg := l.activePeers["remote"].SyncError
	l.mu.RUnlock()
	if status != "ok" || errMsg != "" {
		t.Fatalf("expected ok sync status, got status=%q error=%q", status, errMsg)
	}
}

func TestSyncWithPeerRecordsUnauthorizedStatus(t *testing.T) {
	database, _ := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	l := New("local", 19100)
	peer := testPeerFromServerURL(t, server.URL)
	l.RecordPeer(peer.ID, peer.IP, peer.HTTPPort)
	l.syncWithPeer(peer.ID, peer, database, "wrong-token", map[string]syncCursor{})

	l.mu.RLock()
	status := l.activePeers["remote"].SyncStatus
	errMsg := l.activePeers["remote"].SyncError
	l.mu.RUnlock()
	if status != "unauthorized" || errMsg == "" {
		t.Fatalf("expected unauthorized sync status, got status=%q error=%q", status, errMsg)
	}
}

func TestSyncWithPeerFollowsPagination(t *testing.T) {
	database, _ := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after_id") == "" {
			_ = json.NewEncoder(w).Encode(model.SyncPullResponse{
				Records: []model.SyncRecord{{
					TokenUsage: model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 100, OutputTokens: 10, Timestamp: now, UUID: "page-1"},
					CostUSD:    1.00,
					DeviceID:   "remote",
					UpdatedAt:  now,
				}},
				NextUpdatedAt: now,
				NextAfterID:   1,
				HasMore:       true,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(model.SyncPullResponse{
			Records: []model.SyncRecord{{
				TokenUsage: model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 200, OutputTokens: 20, Timestamp: now.Add(time.Second), UUID: "page-2"},
				CostUSD:    2.00,
				DeviceID:   "remote",
				UpdatedAt:  now.Add(time.Second),
			}},
		})
	}))
	defer server.Close()

	l := New("local", 19100)
	peer := testPeerFromServerURL(t, server.URL)
	l.RecordPeer(peer.ID, peer.IP, peer.HTTPPort)
	l.syncWithPeer(peer.ID, peer, database, "", map[string]syncCursor{})

	if calls != 2 {
		t.Fatalf("expected two paginated sync calls, got %d", calls)
	}
	_, input, _, _, output, err := database.QueryPeriodStatsAll("remote", "")
	if err != nil {
		t.Fatal(err)
	}
	if input != 300 || output != 30 {
		t.Fatalf("expected both pages to sync, got input=%d output=%d", input, output)
	}
}

func TestSyncWithPeerAcceptsLegacyArrayResponse(t *testing.T) {
	database, _ := testutil.NewTestDBAndCalc(t)
	defer database.Close()

	now := time.Now().UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]model.SyncRecord{{
			TokenUsage: model.TokenUsage{Source: "Claude Code", Model: "claude-opus-4-7", InputTokens: 400, OutputTokens: 40, Timestamp: now, UUID: "legacy-array-1"},
			CostUSD:    4.00,
			DeviceID:   "remote",
			UpdatedAt:  now,
		}})
	}))
	defer server.Close()

	l := New("local", 19100)
	peer := testPeerFromServerURL(t, server.URL)
	l.RecordPeer(peer.ID, peer.IP, peer.HTTPPort)
	l.syncWithPeer(peer.ID, peer, database, "", map[string]syncCursor{})

	_, input, _, _, output, err := database.QueryPeriodStatsAll("remote", "")
	if err != nil {
		t.Fatal(err)
	}
	if input != 400 || output != 40 {
		t.Fatalf("expected legacy array response to sync, got input=%d output=%d", input, output)
	}
}
