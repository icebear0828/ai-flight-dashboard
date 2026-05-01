//go:build e2e

// E2E test for LAN multicast ping/discovery.
// Run with: go test -tags=e2e -v ./test/e2e/
package e2e

import (
	"testing"
	"time"

	"ai-flight-dashboard/internal/lan"
	"ai-flight-dashboard/internal/model"
)

func TestLANPingDiscovery(t *testing.T) {
	t.Log("Starting E2E LAN Validation...")

	outChan := make(chan model.TokenUsage, 10)

	listener := lan.New("local-test-device")
	sender := lan.New("device-10-5")

	// Start listener
	go listener.StartListener(outChan)
	time.Sleep(1 * time.Second) // wait for listen

	// Try sending ping using current implementation
	sender.Ping()
	t.Log("Ping sent.")

	// Check if activePeers got it
	time.Sleep(1 * time.Second)
	peers := listener.GetActivePeers()
	t.Logf("Active peers detected: %v", peers)

	if len(peers) > 0 && peers[0] == "device-10-5" {
		t.Log("E2E Test Passed! Listener received the ping.")
	} else {
		t.Errorf("E2E Test Failed! Expected peer 'device-10-5', got: %v", peers)
	}
}
