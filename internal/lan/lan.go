package lan

import (
	"encoding/json"
	"log"
	"net"
	"time"

	"ai-flight-dashboard/internal/model"
)

const (
	MulticastAddr = "224.0.0.123:9101"
	MaxDatagramSize = 8192
)

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
func StartListener(outChan chan<- model.TokenUsage, localDeviceID string) {
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

		// Don't process our own broadcast packets
		if payload.DeviceID == localDeviceID {
			continue
		}

		// Push to the channel for processing (DB, TUI, etc.)
		outChan <- payload.Usage
	}
}
