package model

import "time"

// وضعیت سرویس‌ها
const (
	StatusConnecting = "connecting"
	StatusHealthy    = "healthy"
	StatusError      = "error"
)

// ورودی لاگ برای نمایش در UI
type LogEntry struct {
	Time    time.Time
	Message string
	IsError bool
}

// مدل وضعیت سرویس برای نمایش در UI (فقط data، بدون runtime state)
type Service struct {
	Name         string
	Command      string
	LocalPort    string
	Status       string
	LastError    string
	StartTime    time.Time
	RestartCount int
	Logs         []LogEntry
}

// ساختار گزارش تداخل پورت
type PortConflict struct {
	Port     string
	Services []string
}
