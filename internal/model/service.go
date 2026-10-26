package model

import "time"

const (
	StatusConnecting = "connecting"
	StatusHealthy    = "healthy"
	StatusError      = "error"
)

type LogEntry struct {
	Time    time.Time
	Message string
	IsError bool
}

type Service struct {
	Name         string
	Command      string
	LocalPort    string
	MainPort     string
	IconEnabled  bool
	IconGlyph    string
	IconColor    string
	Status       string
	LastError    string
	StartTime    time.Time
	RestartCount int
	Alias        string // in-cluster hostname aliased to localhost (empty if none active)
	Logs         []LogEntry

	// Live tunnel details.
	Target         string    // e.g. "service/db"
	Namespace      string    // namespace the target resolved in ("" until resolved)
	Pod            string    // pod currently forwarded to ("" until resolved)
	ConnectedSince time.Time // when the current tunnel came up (zero when down)

	// Traffic counters, cumulative over the service's lifetime (they survive
	// reconnects); ActiveConns is the number of open client connections now.
	ActiveConns  int64
	TotalConns   int64
	BytesIn      int64 // pod → local client
	BytesOut     int64 // local client → pod
	LastActivity time.Time
	Latency      time.Duration // round trip of the last stream open
}

type PortConflict struct {
	Port     string
	Services []string
}
