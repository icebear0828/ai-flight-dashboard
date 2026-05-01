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
	peerTTL         = 30 * time.Second
)

var (
	activePeers   = make(map[string]time.Time)
	peerMutex     sync.RWMutex
	localDeviceID string // set by StartListener, used to filter self from peers
)

// StartPinger periodically sends ping packets to the LAN.
// It reuses a single UDP connection across ticks.
func StartPinger(deviceID string) {
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
		DeviceID: deviceID,
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
func Ping(deviceID string) {
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
		DeviceID: deviceID,
		Type:     "ping",
	}
	data, err := json.Marshal(payload)
	if err == nil {
		conn.Write(data)
	}
}

// GetActivePeers returns a list of recently seen remote device IDs.
// Excludes the local device and evicts stale entries.
func GetActivePeers() []string {
	peerMutex.Lock()
	defer peerMutex.Unlock()
	var peers []string
	now := time.Now()
	for id, lastSeen := range activePeers {
		if now.Sub(lastSeen) >= peerTTL {
			delete(activePeers, id) // evict stale
			continue
		}
		if id != localDeviceID {
			peers = append(peers, id)
		}
	}
	return peers
}

// StartBroadcaster listens to a channel and multicasts token usage to the LAN
func StartBroadcaster(usageChan <-chan model.TokenUsage, deviceID string) {
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
			DeviceID: deviceID,
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
func StartListener(outChan chan<- model.TokenUsage, localID string) {
	localDeviceID = localID // store for GetActivePeers filtering

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
		if payload.DeviceID == localID {
			continue
		}

		// Update active peers (remote only)
		peerMutex.Lock()
		activePeers[payload.DeviceID] = time.Now()
		peerMutex.Unlock()

		if payload.Type == "ping" {
			continue // just a presence announce
		}

		// Push to the channel for processing (DB, TUI, etc.)
		outChan <- payload.Usage
	}
}
