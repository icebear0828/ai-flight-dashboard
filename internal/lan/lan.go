package lan

import (
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"

	"ai-flight-dashboard/internal/model"
)

const (
	MulticastAddr   = "224.0.0.123:9101"
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
// It reuses a single UDP connection across ticks.
func (l *LAN) StartPinger() {
	addr, err := net.ResolveUDPAddr("udp", MulticastAddr)
	if err != nil {
		log.Printf("LAN Pinger failed to resolve UDP addr: %v", err)
		return
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Printf("LAN Pinger failed to dial UDP: %v", err)
		return
	}
	defer conn.Close()

	payload := model.TrackPayload{
		DeviceID: l.DeviceID,
		Type:     "ping",
	}
	data, _ := json.Marshal(payload)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if _, err := conn.Write(data); err != nil {
			log.Printf("LAN Pinger write error: %v", err)
		}
	}
}

// Ping sends a single ping packet to announce presence (for ad-hoc use).
func (l *LAN) Ping() {
	addr, err := net.ResolveUDPAddr("udp", MulticastAddr)
	if err != nil {
		return
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()

	payload := model.TrackPayload{
		DeviceID: l.DeviceID,
		Type:     "ping",
	}
	data, err := json.Marshal(payload)
	if err == nil {
		conn.Write(data)
	}
}

// StartBroadcaster listens to a channel and multicasts token usage to the LAN
func (l *LAN) StartBroadcaster(usageChan <-chan model.TokenUsage) {
	addr, err := net.ResolveUDPAddr("udp", MulticastAddr)
	if err != nil {
		log.Printf("LAN Broadcaster failed to resolve UDP addr: %v", err)
		return
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Printf("LAN Broadcaster failed to dial UDP: %v", err)
		return
	}
	defer conn.Close()

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

		// Fire and forget
		_, err = conn.Write(data)
		if err != nil {
			// Just log, don't block
			log.Printf("LAN Broadcaster failed to send packet: %v", err)
		}
	}
}

// StartListener joins the multicast group and forwards received usages to outChan
func (l *LAN) StartListener(outChan chan<- model.TokenUsage) {
	addr, err := net.ResolveUDPAddr("udp", MulticastAddr)
	if err != nil {
		log.Printf("LAN Listener failed to resolve UDP addr: %v", err)
		return
	}

	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		log.Printf("LAN Listener failed to listen: %v", err)
		return
	}
	defer conn.Close()

	conn.SetReadBuffer(MaxDatagramSize)

	buf := make([]byte, MaxDatagramSize)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("LAN Listener read error: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		var payload model.TrackPayload
		if err := json.Unmarshal(buf[:n], &payload); err != nil {
			continue
		}

		// Skip recording our own packets
		if payload.DeviceID == l.DeviceID {
			continue
		}

		// Update active peers (remote only)
		l.RecordPeer(payload.DeviceID)

		if payload.Type == "ping" {
			continue // just a presence announce
		}

		// Push to the channel for processing (DB, TUI, etc.)
		outChan <- payload.Usage
	}
}
