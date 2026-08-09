package models

import "time"

// User represents a user configuration for sing-box inbounds.
type User struct {
	// SubId is the subscription ID (primary identifier).
	SubID string `json:"subId"`

	// UUID is the protocol-specific UUID (e.g., VLESS/VMess).
	UUID string `json:"uuid,omitempty"`

	// InboundTag is the inbound this user belongs to.
	InboundTag string `json:"inboundTag"`

	// Email is the user's email (used for logging/tracking).
	Email string `json:"email,omitempty"`

	// Enabled indicates if the user is active.
	Enabled bool `json:"enabled"`

	// Flow is the VLESS flow (e.g., "xtls-rprx-vision").
	Flow string `json:"flow,omitempty"`

	// LimitIP limits the number of concurrent connections.
	LimitIP int `json:"limitIp,omitempty"`

	// UploadLimit is the upload limit in bytes.
	UploadLimit uint64 `json:"uploadLimit,omitempty"`

	// DownloadLimit is the download limit in bytes.
	DownloadLimit uint64 `json:"downloadLimit,omitempty"`

	// ResetAt is the timestamp when counters were last reset.
	ResetAt *time.Time `json:"resetAt,omitempty"`
}

// Inbound represents an inbound configuration for sing-box.
type Inbound struct {
	// Tag is the unique identifier for this inbound.
	Tag string `json:"tag"`

	// Type is the inbound protocol type (e.g., "vless", "hysteria2").
	Type string `json:"type"`

	// Listen is the listen address.
	Listen string `json:"listen,omitempty"`

	// Port is the listen port.
	Port int `json:"port,omitempty"`

	// Options are protocol-specific options.
	Options map[string]interface{} `json:"options,omitempty"`
}

// DesiredState represents the desired state pushed from the API.
type DesiredState struct {
	// Version is a monotonically increasing version number for conflict resolution.
	Version int `json:"version"`

	// Inbounds is the list of desired inbound configurations.
	Inbounds []Inbound `json:"inbounds,omitempty"`

	// Users is the list of desired user configurations.
	Users []User `json:"users,omitempty"`

	// Timestamp is when this desired state was generated.
	Timestamp time.Time `json:"timestamp"`
}

// SyncResult represents the result of a sync operation.
type SyncResult struct {
	// Success indicates if the sync was successful.
	Success bool `json:"success"`

	// ChangesApplied is the number of changes applied.
	ChangesApplied int `json:"changesApplied"`

	// Version is the version that was synced.
	Version int `json:"version"`

	// Error holds any error that occurred.
	Error string `json:"error,omitempty"`

	// Timestamp is when the sync completed.
	Timestamp time.Time `json:"timestamp"`
}

// TrafficStats represents traffic statistics.
type TrafficStats struct {
	// SubId is the subscription ID.
	SubID string `json:"subId"`

	// Upload is the cumulative upload bytes.
	Upload uint64 `json:"upload"`

	// Download is the cumulative download bytes.
	Download uint64 `json:"download"`

	// ResetAt is when counters were last reset.
	ResetAt *time.Time `json:"resetAt,omitempty"`
}

// TrafficInboundStat represents traffic statistics for a single inbound.
type TrafficInboundStat struct {
	Tag       string `json:"tag"`
	Type      string `json:"type,omitempty"`
	UpBytes   uint64 `json:"up_bytes"`
	DownBytes uint64 `json:"down_bytes"`
}

// TrafficUserStat represents cumulative traffic for a single user (keyed by subId).
type TrafficUserStat struct {
	SubID     string `json:"subId"`
	Inbound   string `json:"inbound,omitempty"`
	UpBytes   uint64 `json:"up"`
	DownBytes uint64 `json:"down"`
}

// OnlineUser represents a currently connected user.
type OnlineUser struct {
	SubID       string    `json:"subId"`
	Inbound     string    `json:"inbound"`
	ConnectedAt time.Time `json:"connected_at"`
}
