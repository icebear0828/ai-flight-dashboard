package lan

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
)

const (
	MulticastAddr   = "224.0.0.123:9101"
	BroadcastAddr   = "255.255.255.255:9101"
	DefaultHTTPPort = 19100
	MaxDatagramSize = 8192
	PeerTTL         = 30 * time.Second

	HTTPDiscoveryInterval    = 15 * time.Second
	HTTPDiscoveryTimeout     = 2 * time.Second
	SummaryCacheTTL          = 20 * time.Second
	httpDiscoveryConcurrency = 32
)

type PeerInfo struct {
	ID              string
	IP              string
	HTTPPort        int
	LastSeen        time.Time
	LastSync        time.Time
	LastSyncAttempt time.Time
	SyncStatus      string
	SyncError       string
	Summary         model.TokenSummary
	HasSummary      bool
}

type syncCursor struct {
	updatedAt time.Time
	afterID   int64
}

// LAN manages LAN-based device discovery and usage broadcasting.
// It replaces the former package-level global state with a testable struct.
type LAN struct {
	DeviceID string
	HTTPPort int

	mu              sync.RWMutex
	activePeers     map[string]PeerInfo
	peerUpdates     chan struct{}
	lastDirtySent   time.Time
	summarySource   func() model.TokenSummary
	summaryVersion  uint64
	summaryCacheTTL time.Duration
	cachedSummary   model.TokenSummary
	cachedSummaryAt time.Time
	cachedSummaryOK bool
	httpProbePorts  []int
}

// New creates a new LAN instance for the given device.
func New(deviceID string, httpPort int) *LAN {
	return &LAN{
		DeviceID:        deviceID,
		HTTPPort:        httpPort,
		activePeers:     make(map[string]PeerInfo),
		peerUpdates:     make(chan struct{}, 1),
		summaryCacheTTL: SummaryCacheTTL,
		httpProbePorts:  normalizeHTTPDiscoveryPorts([]int{httpPort}),
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

// GetActivePeerInfos returns recently seen remote peers with sync status.
func (l *LAN) GetActivePeerInfos() []PeerInfo {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	peers := make([]PeerInfo, 0, len(l.activePeers))
	for id, peer := range l.activePeers {
		if now.Sub(peer.LastSeen) >= PeerTTL {
			delete(l.activePeers, id)
			continue
		}
		if id == l.DeviceID {
			continue
		}
		peer.ID = id
		peers = append(peers, peer)
	}
	sort.Slice(peers, func(i, j int) bool {
		return peers[i].ID < peers[j].ID
	})
	return peers
}

// SetSummaryProvider sets the aggregate token summary advertised in LAN packets.
func (l *LAN) SetSummaryProvider(provider func() model.TokenSummary) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.summarySource = provider
	l.summaryVersion++
	l.cachedSummaryOK = false
}

// CurrentSummary returns the current aggregate token summary, if configured.
func (l *LAN) CurrentSummary() (model.TokenSummary, bool) {
	now := time.Now()
	l.mu.RLock()
	provider := l.summarySource
	version := l.summaryVersion
	ttl := l.summaryCacheTTL
	if ttl <= 0 {
		ttl = SummaryCacheTTL
	}
	if l.cachedSummaryOK && now.Sub(l.cachedSummaryAt) < ttl {
		summary := l.cachedSummary
		l.mu.RUnlock()
		return summary, true
	}
	l.mu.RUnlock()
	if provider == nil {
		return model.TokenSummary{}, false
	}
	summary := provider()

	l.mu.Lock()
	if l.summaryVersion == version {
		l.cachedSummary = summary
		l.cachedSummaryAt = now
		l.cachedSummaryOK = true
	}
	l.mu.Unlock()
	return summary, true
}

// SetHTTPDiscoveryPorts configures local HTTP ports to probe for active peers.
func (l *LAN) SetHTTPDiscoveryPorts(ports ...int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.httpProbePorts = normalizeHTTPDiscoveryPorts(ports)
}

func (l *LAN) httpDiscoveryPorts() []int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]int(nil), l.httpProbePorts...)
}

func normalizeHTTPDiscoveryPorts(ports []int) []int {
	seen := make(map[int]bool)
	normalized := make([]int, 0, len(ports)+1)
	for _, port := range ports {
		if port <= 0 || port > 65535 || seen[port] {
			continue
		}
		seen[port] = true
		normalized = append(normalized, port)
	}
	if len(normalized) == 0 {
		normalized = append(normalized, DefaultHTTPPort)
	}
	return normalized
}

// RecordPeer records a peer as active. Exposed for testing.
func (l *LAN) RecordPeer(deviceID, ip string, httpPort int) {
	l.recordPeer(deviceID, ip, httpPort, nil, true)
}

// RecordPeerSummary records a peer and its advertised token summary.
func (l *LAN) RecordPeerSummary(deviceID, ip string, httpPort int, summary model.TokenSummary) {
	l.recordPeer(deviceID, ip, httpPort, &summary, true)
}

func (l *LAN) recordPeer(deviceID, ip string, httpPort int, summary *model.TokenSummary, notify bool) {
	if deviceID == "" {
		return
	}
	l.mu.Lock()
	peer := l.activePeers[deviceID]
	wasNew := peer.ID == ""
	endpointChanged := peer.IP != ip || peer.HTTPPort != httpPort
	peer.ID = deviceID
	peer.IP = ip
	peer.HTTPPort = httpPort
	peer.LastSeen = time.Now()
	if httpPort == 0 {
		peer.SyncStatus = "discovery_only"
		peer.SyncError = ""
	} else if peer.SyncStatus == "discovery_only" {
		peer.SyncStatus = "pending"
	}
	if peer.SyncStatus == "" {
		peer.SyncStatus = "pending"
	}
	if summary != nil {
		peer.Summary = *summary
		peer.HasSummary = true
	}
	l.activePeers[deviceID] = peer
	l.mu.Unlock()

	if notify || wasNew || endpointChanged {
		select {
		case l.peerUpdates <- struct{}{}:
		default:
		}
	}
}

// RecordPeerAt records a peer with a specific timestamp. Exposed for testing.
func (l *LAN) RecordPeerAt(deviceID, ip string, httpPort int, at time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.activePeers[deviceID] = PeerInfo{
		ID:         deviceID,
		IP:         ip,
		HTTPPort:   httpPort,
		LastSeen:   at,
		SyncStatus: "pending",
	}
}

func (l *LAN) newPayload(payloadType string) model.TrackPayload {
	payload := model.TrackPayload{
		DeviceID: l.DeviceID,
		HTTPPort: l.HTTPPort,
		Type:     payloadType,
	}

	if summary, ok := l.CurrentSummary(); ok {
		payload.Summary = &summary
	}
	return payload
}

func (l *LAN) updatePeerSyncAttempt(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	peer := l.activePeers[id]
	peer.ID = id
	peer.LastSyncAttempt = time.Now()
	if peer.SyncStatus == "" || peer.SyncStatus == "pending" {
		peer.SyncStatus = "syncing"
	}
	l.activePeers[id] = peer
}

func (l *LAN) updatePeerSyncResult(id string, status string, errMsg string, syncedAt time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	peer := l.activePeers[id]
	peer.ID = id
	peer.SyncStatus = status
	peer.SyncError = errMsg
	if status == "ok" {
		peer.LastSync = syncedAt
	}
	l.activePeers[id] = peer
}

// StartPinger periodically sends ping packets to the LAN.
func (l *LAN) StartPinger() {
	if data, err := json.Marshal(l.newPayload("ping")); err == nil {
		l.sendToAll(data)
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if data, err := json.Marshal(l.newPayload("ping")); err == nil {
			l.sendToAll(data)
		}
	}
}

// Ping sends a single ping packet to announce presence (for ad-hoc use).
func (l *LAN) Ping() {
	data, err := json.Marshal(l.newPayload("ping"))
	if err == nil {
		l.sendToAll(data)
	}
}

// AnnounceDirty tells peers that local database content changed. Token data is
// still pulled over HTTP; UDP is only a wake-up signal.
func (l *LAN) AnnounceDirty() {
	l.mu.Lock()
	l.cachedSummaryOK = false
	if time.Since(l.lastDirtySent) < 2*time.Second {
		l.mu.Unlock()
		return
	}
	l.lastDirtySent = time.Now()
	l.mu.Unlock()

	data, err := json.Marshal(l.newPayload("dirty"))
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

func localHTTPDiscoveryHosts() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	seen := make(map[string]bool)
	hosts := make([]string, 0)
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrsList, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrsList {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			for _, host := range probeHostsForIPv4(ipnet.IP, ipnet.Mask) {
				if seen[host] {
					continue
				}
				seen[host] = true
				hosts = append(hosts, host)
			}
		}
	}
	sort.Strings(hosts)
	return hosts
}

func probeHostsForIPv4(ip net.IP, mask net.IPMask) []string {
	v4 := ip.To4()
	if v4 == nil {
		return nil
	}
	ones, bits := mask.Size()
	if bits != 32 {
		return nil
	}
	if ones < 24 {
		mask = net.CIDRMask(24, 32)
	}

	ipInt := binary.BigEndian.Uint32(v4)
	maskInt := binary.BigEndian.Uint32(mask)
	network := ipInt & maskInt
	broadcast := network | ^maskInt
	if broadcast <= network+1 {
		return nil
	}

	hosts := make([]string, 0, broadcast-network-1)
	for candidate := network + 1; candidate < broadcast; candidate++ {
		if candidate == ipInt {
			continue
		}
		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], candidate)
		hosts = append(hosts, net.IP(buf[:]).String())
	}
	return hosts
}

// StartHTTPDiscovery periodically probes local subnet HTTP endpoints for peers.
func (l *LAN) StartHTTPDiscovery(ctx context.Context) {
	l.ScanHTTPPeers(ctx, nil, nil)

	ticker := time.NewTicker(HTTPDiscoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.ScanHTTPPeers(ctx, nil, nil)
		}
	}
}

// ScanHTTPPeers actively discovers LAN nodes by probing their self endpoint.
func (l *LAN) ScanHTTPPeers(ctx context.Context, hosts []string, ports []int) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(hosts) == 0 {
		hosts = localHTTPDiscoveryHosts()
	}
	if len(ports) == 0 {
		ports = l.httpDiscoveryPorts()
	} else {
		ports = normalizeHTTPDiscoveryPorts(ports)
	}
	if len(hosts) == 0 || len(ports) == 0 {
		return
	}

	type target struct {
		host string
		port int
	}
	totalTargets := len(hosts) * len(ports)
	workers := httpDiscoveryConcurrency
	if totalTargets < workers {
		workers = totalTargets
	}

	client := &http.Client{Timeout: HTTPDiscoveryTimeout}
	jobs := make(chan target)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				l.probeHTTPPeer(ctx, client, job.host, job.port)
			}
		}()
	}

sendJobs:
	for _, host := range hosts {
		for _, port := range ports {
			select {
			case <-ctx.Done():
				break sendJobs
			case jobs <- target{host: host, port: port}:
			}
		}
	}
	close(jobs)
	wg.Wait()
}

func (l *LAN) probeHTTPPeer(ctx context.Context, client *http.Client, host string, port int) {
	if port <= 0 || port > 65535 {
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, HTTPDiscoveryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, "http://"+net.JoinHostPort(host, strconv.Itoa(port))+"/api/lan/self", nil)
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	var self model.LANSelfResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, MaxDatagramSize)).Decode(&self); err != nil {
		return
	}
	if self.DeviceID == "" || self.DeviceID == l.DeviceID {
		return
	}
	httpPort := self.HTTPPort
	if httpPort < 0 || httpPort > 65535 {
		httpPort = 0
	}
	l.recordPeer(self.DeviceID, host, httpPort, self.Summary, true)
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

// StartBroadcaster listens to local usage events and broadcasts dirty signals.
// Token records are synchronized over HTTP, not UDP.
func (l *LAN) StartBroadcaster(usageChan <-chan model.TokenUsage) {
	for range usageChan {
		l.AnnounceDirty()
	}
}

// StartListener joins the multicast group and records discovered peers.
// Token data is not trusted from UDP; peers pull records through HTTP sync.
func (l *LAN) StartListener(outChan chan<- model.TokenUsage) {
	l.StartListenerContext(context.Background(), outChan)
}

// StartListenerContext joins the multicast group and records discovered peers
// until ctx is canceled.
func (l *LAN) StartListenerContext(ctx context.Context, outChan chan<- model.TokenUsage) {
	_ = outChan
	var wg sync.WaitGroup

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
		l.recordPeer(payload.DeviceID, ipStr, payload.HTTPPort, payload.Summary, payload.Type == "dirty")
	}

	readLoop := func(conn *net.UDPConn) {
		defer conn.Close()
		go func() {
			<-ctx.Done()
			_ = conn.Close()
		}()
		conn.SetReadBuffer(MaxDatagramSize)
		buf := make([]byte, MaxDatagramSize)
		for {
			n, src, err := conn.ReadFromUDP(buf)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
				}
				time.Sleep(1 * time.Second)
				continue
			}
			processPayload(buf, n, src)
		}
	}

	// Multicast Listener
	wg.Add(1)
	go func() {
		defer wg.Done()
		addr, err := net.ResolveUDPAddr("udp", MulticastAddr)
		if err == nil {
			if conn, err := net.ListenMulticastUDP("udp", nil, addr); err == nil {
				readLoop(conn)
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
				readLoop(conn)
			}
		}
	}()

	// Wait forever
	wg.Wait()
}

// StartAutoSync runs a background loop to pull DB records from active LAN peers via HTTP.
func (l *LAN) StartAutoSync(database *db.DB, token string) {
	l.StartAutoSyncContext(context.Background(), database, token)
}

// StartAutoSyncContext runs a background loop to pull DB records until ctx ends.
func (l *LAN) StartAutoSyncContext(ctx context.Context, database *db.DB, token string) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Track the last cursor we synced with each peer.
	lastSync := make(map[string]syncCursor)

	syncOnce := func() {
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
				l.updatePeerSyncResult(id, "discovery_only", "", time.Time{})
				continue
			}

			l.syncWithPeer(id, peer, database, token, lastSync)
		}
	}

	syncOnce()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncOnce()
		case <-l.peerUpdates:
			syncOnce()
		}
	}
}

func (l *LAN) syncWithPeer(id string, peer PeerInfo, database *db.DB, token string, lastSync map[string]syncCursor) {
	l.updatePeerSyncAttempt(id)

	peerCursorKey := id + "\x00peer"
	if !l.syncWithPeerScope(id, peer, database, token, lastSync, peerCursorKey, id) {
		return
	}
	if !l.syncWithPeerScope(id, peer, database, token, lastSync, id, "") {
		return
	}

	l.updatePeerSyncResult(id, "ok", "", time.Now().UTC())
}

func (l *LAN) syncWithPeerScope(id string, peer PeerInfo, database *db.DB, token string, lastSync map[string]syncCursor, cursorKey string, deviceID string) bool {
	baseURL := "http://" + net.JoinHostPort(peer.IP, strconv.Itoa(peer.HTTPPort)) + "/api/sync/pull"
	cursor := lastSync[cursorKey]

	client := &http.Client{Timeout: 10 * time.Second}
	for {
		values := url.Values{}
		values.Set("limit", "1000")
		if deviceID != "" {
			values.Set("device_id", deviceID)
		}
		if !cursor.updatedAt.IsZero() {
			values.Set("since", cursor.updatedAt.Format(time.RFC3339Nano))
		}
		if cursor.afterID > 0 {
			values.Set("after_id", strconv.FormatInt(cursor.afterID, 10))
		}

		req, err := http.NewRequest("GET", baseURL+"?"+values.Encode(), nil)
		if err != nil {
			l.updatePeerSyncResult(id, "error", err.Error(), time.Time{})
			return false
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := client.Do(req)
		if err != nil {
			l.updatePeerSyncResult(id, "unreachable", err.Error(), time.Time{})
			return false
		}

		if resp.StatusCode == http.StatusUnauthorized {
			resp.Body.Close()
			l.updatePeerSyncResult(id, "unauthorized", "remote rejected sync token", time.Time{})
			return false
		}
		if resp.StatusCode != http.StatusOK {
			status := resp.StatusCode
			resp.Body.Close()
			l.updatePeerSyncResult(id, "http_error", fmt.Sprintf("remote returned HTTP %d", status), time.Time{})
			return false
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			l.updatePeerSyncResult(id, "error", err.Error(), time.Time{})
			return false
		}

		var page model.SyncPullResponse
		if err := json.Unmarshal(body, &page); err != nil {
			var legacyRecords []model.SyncRecord
			if legacyErr := json.Unmarshal(body, &legacyRecords); legacyErr != nil {
				l.updatePeerSyncResult(id, "error", err.Error(), time.Time{})
				return false
			}
			page.Records = legacyRecords
			page.HasMore = false
		}

		for _, r := range page.Records {
			if err := database.UpsertSyncRecord(r); err != nil {
				errMsg := fmt.Sprintf("DB insert error for device %s: %v", r.DeviceID, err)
				log.Printf("LAN sync %s", errMsg)
				l.updatePeerSyncResult(id, "error", errMsg, time.Time{})
				return false
			}
		}
		if !page.NextUpdatedAt.IsZero() {
			cursor.updatedAt = page.NextUpdatedAt
			cursor.afterID = page.NextAfterID
		}
		if !page.HasMore {
			break
		}
		if page.NextUpdatedAt.IsZero() || page.NextAfterID == 0 {
			l.updatePeerSyncResult(id, "error", "remote returned incomplete sync cursor", time.Time{})
			return false
		}
	}

	if cursor.updatedAt.IsZero() {
		cursor.updatedAt = time.Now().UTC().Add(-5 * time.Minute)
	}
	lastSync[cursorKey] = cursor
	return true
}
