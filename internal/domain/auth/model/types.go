package model

import "time"

// ClientInfo captures authentication metadata persisted by the store.
type ClientInfo struct {
	ClientID  string            `json:"client_id"`
	Username  string            `json:"username"`
	Password  string            `json:"password"`
	IP        string            `json:"ip,omitempty"`
	DeviceID  string            `json:"device_id,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt *time.Time        `json:"expires_at,omitempty"`
	Metadata  map[string]any    `json:"metadata,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"` // optional tags for cross-store filters
}

// Logger provides the minimal logging contract required by the auth domain.
type Logger interface {
	Debug(format string, args ...any)
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
}
