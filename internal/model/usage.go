package model

import "time"

// TokenUsage represents a single AI tool usage event with token counts.
// Shared by watcher, scanner, db, and tui packages.
type TokenUsage struct {
	DeviceID            string    `json:"device_id,omitempty"` // For LAN routing
	Source              string    `json:"source"`
	Model               string    `json:"model"`
	Project             string    `json:"project"`
	InputTokens         int       `json:"input_tokens"`
	CachedTokens        int       `json:"cached_tokens"`
	CacheCreationTokens int       `json:"cache_creation_tokens"`
	OutputTokens        int       `json:"output_tokens"`
	Thoughts            int       `json:"thoughts"`
	Timestamp           time.Time `json:"timestamp"`
	UUID                string    `json:"uuid"` // for dedup: Claude writes snapshots with same uuid
}

// TrackPayload is the JSON payload for sending telemetry data to the remote server.
type TrackPayload struct {
	DeviceID string     `json:"device_id"`
	Type     string     `json:"type,omitempty"` // "ping" or empty/"track"
	Usage    TokenUsage `json:"usage"`
}
