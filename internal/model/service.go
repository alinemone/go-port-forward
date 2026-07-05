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
}

type PortConflict struct {
	Port     string
	Services []string
}
