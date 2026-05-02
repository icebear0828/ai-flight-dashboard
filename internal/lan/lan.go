package lan

import (
	"encoding/json"
	"net"
	"sync"
	"time"

	"ai-flight-dashboard/internal/model"
)

const (
	MulticastAddr   = "224.0.0.123:9101"
	BroadcastAddr   = "255.255.255.255:9101"
	MaxDatagramSize = 8192
	PeerTTL         = 30 * time.Second
)

// LAN manages LAN-based device discovery and usage broadcasting.
// It replaces the former package-level global state with a testable struct.
type LAN struct {
	DeviceID string

	mu          sync.RWMutex
	activePeers map[string]time.Time
}

// New creates a new LAN instance for the given device.
func New(deviceID string) *LAN {
	return &LAN{
		DeviceID:    deviceID,
		activePeers: make(map[string]time.Time),
	}
}

// GetActivePeers returns a list of recently seen remote device IDs.
// Excludes the local device and evicts stale entries.
func (l *LAN) GetActivePeers() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var peers []string
	now := time.Now()
	for id, lastSeen := range l.activePeers {
		if now.Sub(lastSeen) >= PeerTTL {
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
func (l *LAN) RecordPeer(deviceID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.activePeers[deviceID] = time.Now()
}

// RecordPeerAt records a peer with a specific timestamp. Exposed for testing.
func (l *LAN) RecordPeerAt(deviceID string, at time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.activePeers[deviceID] = at
}

// StartPinger periodically sends ping packets to the LAN.
func (l *LAN) StartPinger() {
	payload := model.TrackPayload{
		DeviceID: l.DeviceID,
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

	processPayload := func(buf []byte, n int) {
		var payload model.TrackPayload
		if err := json.Unmarshal(buf[:n], &payload); err != nil {
			return
		}

		// Skip recording our own packets
		if payload.DeviceID == l.DeviceID {
			return
		}

		// Update active peers (remote only)
		l.RecordPeer(payload.DeviceID)

		if payload.Type == "ping" {
			return // just a presence announce
		}

		// Deduplicate incoming usage
		uuid := payload.Usage.UUID
		if uuid != "" {
			seenMu.Lock()
			if _, exists := seenUUIDs[uuid]; exists {
				seenMu.Unlock()
				return
			}
			seenUUIDs[uuid] = time.Now()
			seenMu.Unlock()
		}

		// Push to the channel for processing (DB, TUI, etc.)
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
					n, _, err := conn.ReadFromUDP(buf)
					if err != nil {
						time.Sleep(1 * time.Second)
						continue
					}
					processPayload(buf, n)
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
					n, _, err := conn.ReadFromUDP(buf)
					if err != nil {
						time.Sleep(1 * time.Second)
						continue
					}
					processPayload(buf, n)
				}
			}
		}
	}()

	// Wait forever
	wg.Wait()
}
