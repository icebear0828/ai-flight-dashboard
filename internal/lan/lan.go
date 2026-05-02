package lan

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
)

const (
	MulticastAddr   = "224.0.0.123:9101"
	BroadcastAddr   = "255.255.255.255:9101"
	MaxDatagramSize = 8192
	PeerTTL         = 30 * time.Second
)

type PeerInfo struct {
	IP       string
	HTTPPort int
	LastSeen time.Time
}

// LAN manages LAN-based device discovery and usage broadcasting.
// It replaces the former package-level global state with a testable struct.
type LAN struct {
	DeviceID string
	HTTPPort int

	mu          sync.RWMutex
	activePeers map[string]PeerInfo
}

// New creates a new LAN instance for the given device.
func New(deviceID string, httpPort int) *LAN {
	return &LAN{
		DeviceID:    deviceID,
		HTTPPort:    httpPort,
		activePeers: make(map[string]PeerInfo),
	}
}

// GetActivePeers returns a list of recently seen remote device IDs.
// Excludes the local device and evicts stale entries.
func (l *LAN) GetActivePeers() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var peers []string
	now := time.Now()
	for id, peer := range l.activePeers {
		if now.Sub(peer.LastSeen) >= PeerTTL {
			delete(l.activePeers, id) // evict stale
			continue
		}
		if id != l.DeviceID {
			peers = append(peers, id)
		}
	}
	return peers
}

// RecordPeer records a peer as active. Exposed for testing.
func (l *LAN) RecordPeer(deviceID, ip string, httpPort int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.activePeers[deviceID] = PeerInfo{
		IP:       ip,
		HTTPPort: httpPort,
		LastSeen: time.Time{}, // handled below, but time.Now() is what we want
	}
	// Note: We'll set LastSeen correctly
	p := l.activePeers[deviceID]
	p.LastSeen = time.Now()
	l.activePeers[deviceID] = p
}

// RecordPeerAt records a peer with a specific timestamp. Exposed for testing.
func (l *LAN) RecordPeerAt(deviceID, ip string, httpPort int, at time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.activePeers[deviceID] = PeerInfo{
		IP:       ip,
		HTTPPort: httpPort,
		LastSeen: at,
	}
}

// StartPinger periodically sends ping packets to the LAN.
func (l *LAN) StartPinger() {
	payload := model.TrackPayload{
		DeviceID: l.DeviceID,
		HTTPPort: l.HTTPPort,
		Type:     "ping",
	}
	data, _ := json.Marshal(payload)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		l.sendToAll(data)
	}
}

// Ping sends a single ping packet to announce presence (for ad-hoc use).
func (l *LAN) Ping() {
	payload := model.TrackPayload{
		DeviceID: l.DeviceID,
		HTTPPort: l.HTTPPort,
		Type:     "ping",
	}
	data, err := json.Marshal(payload)
	if err == nil {
		l.sendToAll(data)
	}
}

func getBroadcastAddresses() []string {
	var addrs []string
	interfaces, err := net.Interfaces()
	if err != nil {
		return []string{BroadcastAddr} // fallback
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrsList, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrsList {
			if ipnet, ok := a.(*net.IPNet); ok {
				if ipnet.IP.To4() != nil {
					ip := ipnet.IP.To4()
					mask := ipnet.Mask
					if len(mask) == 4 {
						bcast := net.IPv4(
							ip[0]|^mask[0],
							ip[1]|^mask[1],
							ip[2]|^mask[2],
							ip[3]|^mask[3],
						)
						addrs = append(addrs, bcast.String()+":9101")
					}
				}
			}
		}
	}
	
	if len(addrs) == 0 {
		addrs = append(addrs, BroadcastAddr)
	}
	return addrs
}

func (l *LAN) sendToAll(data []byte) {
	// Send to Multicast
	mAddr, err := net.ResolveUDPAddr("udp", MulticastAddr)
	if err == nil {
		if mConn, err := net.DialUDP("udp", nil, mAddr); err == nil {
			mConn.Write(data)
			mConn.Close()
		}
	}
	// Send to computed subnet broadcasts
	for _, bcastStr := range getBroadcastAddresses() {
		bAddr, err := net.ResolveUDPAddr("udp", bcastStr)
		if err == nil {
			if bConn, err := net.DialUDP("udp", nil, bAddr); err == nil {
				bConn.Write(data)
				bConn.Close()
			}
		}
	}
	// Also send to global broadcast just in case
	gAddr, err := net.ResolveUDPAddr("udp", BroadcastAddr)
	if err == nil {
		if gConn, err := net.DialUDP("udp", nil, gAddr); err == nil {
			gConn.Write(data)
			gConn.Close()
		}
	}
}


// StartBroadcaster listens to a channel and multicasts/broadcasts token usage to the LAN
func (l *LAN) StartBroadcaster(usageChan <-chan model.TokenUsage) {
	for usage := range usageChan {
		payload := model.TrackPayload{
			DeviceID: l.DeviceID,
			HTTPPort: l.HTTPPort,
			Type:     "track",
			Usage:    usage,
		}
		data, err := json.Marshal(payload)
		if err != nil {
			continue
		}

		// Fire and forget to both addresses
		l.sendToAll(data)
	}
}

// StartListener joins the multicast group and forwards received usages to outChan
func (l *LAN) StartListener(outChan chan<- model.TokenUsage) {
	var wg sync.WaitGroup
	
	seenUUIDs := make(map[string]time.Time)
	var seenMu sync.Mutex

	// Cleanup old UUIDs
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			seenMu.Lock()
			now := time.Now()
			for k, v := range seenUUIDs {
				if now.Sub(v) > 10*time.Minute {
					delete(seenUUIDs, k)
				}
			}
			seenMu.Unlock()
		}
	}()

	processPayload := func(buf []byte, n int, srcAddr *net.UDPAddr) {
		var payload model.TrackPayload
		if err := json.Unmarshal(buf[:n], &payload); err != nil {
			return
		}

		// Skip recording our own packets
		if payload.DeviceID == l.DeviceID {
			return
		}

		ipStr := ""
		if srcAddr != nil && srcAddr.IP != nil {
			ipStr = srcAddr.IP.String()
		}

		// Update active peers (remote only)
		l.RecordPeer(payload.DeviceID, ipStr, payload.HTTPPort)

		if payload.Type == "ping" {
			return // just a presence announce
		}

		// Deduplicate incoming usage packets
		// We use a combination of UUID and token counts to form a unique state hash.
		// This ensures identical duplicate UDP packets (from multicast+broadcast) are dropped,
		// but legitimate streaming updates (where tokens increase) are allowed through.
		uuid := payload.Usage.UUID
		if uuid != "" {
			stateHash := uuid
			// Append token counts to distinguish stream updates
			if payload.Usage.OutputTokens > 0 || payload.Usage.InputTokens > 0 || payload.Usage.Thoughts > 0 {
				stateHash = fmt.Sprintf("%s-%d-%d-%d", uuid, payload.Usage.InputTokens, payload.Usage.OutputTokens, payload.Usage.Thoughts)
			}
			
			seenMu.Lock()
			if _, exists := seenUUIDs[stateHash]; exists {
				seenMu.Unlock()
				return
			}
			seenUUIDs[stateHash] = time.Now()
			seenMu.Unlock()
		}

		// Push to the channel for processing (DB, TUI, etc.)
		payload.Usage.DeviceID = payload.DeviceID
		outChan <- payload.Usage
	}

	// Multicast Listener
	wg.Add(1)
	go func() {
		defer wg.Done()
		addr, err := net.ResolveUDPAddr("udp", MulticastAddr)
		if err == nil {
			if conn, err := net.ListenMulticastUDP("udp", nil, addr); err == nil {
				defer conn.Close()
				conn.SetReadBuffer(MaxDatagramSize)
				buf := make([]byte, MaxDatagramSize)
				for {
					n, src, err := conn.ReadFromUDP(buf)
					if err != nil {
						time.Sleep(1 * time.Second)
						continue
					}
					processPayload(buf, n, src)
				}
			}
		}
	}()

	// Broadcast Listener
	wg.Add(1)
	go func() {
		defer wg.Done()
		addr, err := net.ResolveUDPAddr("udp", "0.0.0.0:9101")
		if err == nil {
			if conn, err := net.ListenUDP("udp", addr); err == nil {
				defer conn.Close()
				conn.SetReadBuffer(MaxDatagramSize)
				buf := make([]byte, MaxDatagramSize)
				for {
					n, src, err := conn.ReadFromUDP(buf)
					if err != nil {
						time.Sleep(1 * time.Second)
						continue
					}
					processPayload(buf, n, src)
				}
			}
		}
	}()



	// Wait forever
	wg.Wait()
}

// StartAutoSync runs a background loop to pull DB records from active LAN peers via HTTP.
func (l *LAN) StartAutoSync(database *db.DB, token string) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Track the last time we synced with each peer
	lastSync := make(map[string]time.Time)

	for range ticker.C {
		l.mu.RLock()
		peers := make(map[string]PeerInfo)
		for id, peer := range l.activePeers {
			if id != l.DeviceID {
				peers[id] = peer
			}
		}
		l.mu.RUnlock()

		for id, peer := range peers {
			if peer.IP == "" || peer.HTTPPort == 0 {
				continue
			}

			l.syncWithPeer(id, peer, database, token, lastSync)
		}
	}
}

func (l *LAN) syncWithPeer(id string, peer PeerInfo, database *db.DB, token string, lastSync map[string]time.Time) {
	since := lastSync[id]
	url := fmt.Sprintf("http://%s:%d/api/sync/pull", peer.IP, peer.HTTPPort)
	if !since.IsZero() {
		url += fmt.Sprintf("?since=%s", since.Format(time.RFC3339))
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var records []model.SyncRecord
		if err := json.NewDecoder(resp.Body).Decode(&records); err == nil {
			for _, r := range records {
				// InsertUsageWithTime handles UPSERT.
				// r.TokenUsage already has the data. We pass r.DeviceID to preserve the original device.
				if err := database.InsertUsageWithTime(r.TokenUsage, r.CostUSD, r.Timestamp, r.FilePath, r.DeviceID); err != nil {
					log.Printf("LAN sync DB insert error for device %s: %v", r.DeviceID, err)
				}
			}
			// Only update lastSync if we successfully reached them
			lastSync[id] = time.Now()
		}
	}
}
