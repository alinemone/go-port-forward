// Package service provides service lifecycle management.
package service

import (
	"context"
	"sync"
	"time"
)

// Status represents the current state of a service.
type Status string

const (
	StatusConnecting   Status = "CONNECTING"
	StatusOnline       Status = "ONLINE"
	StatusReconnecting Status = "RECONNECTING"
	StatusError        Status = "ERROR"
)

// State represents the runtime state of a service.
type State struct {
	Name       string
	Status     Status
	LocalPort  string
	RemotePort string
	Command    string

	LastError  string
	ErrorTime  time.Time
	OnlineTime time.Time

	HealthOK    bool
	LastHealthy time.Time

	// Internal
	cancel context.CancelFunc
	mu     sync.RWMutex
}

// GetStatus safely gets the current status.
func (s *State) GetStatus() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Status
}

// SetStatus safely sets the status.
func (s *State) SetStatus(status Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
	if status == StatusOnline {
		s.OnlineTime = time.Now()
		s.LastHealthy = time.Now()
		s.HealthOK = true
	}
}

// SetError safely sets an error.
func (s *State) SetError(errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastError = errMsg
	s.ErrorTime = time.Now()
	s.Status = StatusError
	s.HealthOK = false
}

// ClearError safely clears the error.
func (s *State) ClearError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastError = ""
	s.ErrorTime = time.Time{}
}

// SetHealth safely sets the health status.
func (s *State) SetHealth(healthy bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.HealthOK = healthy
	if healthy {
		s.LastHealthy = time.Now()
	}
}

// GetSnapshot returns a snapshot of the current state (thread-safe).
func (s *State) GetSnapshot() State {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return State{
		Name:        s.Name,
		Status:      s.Status,
		LocalPort:   s.LocalPort,
		RemotePort:  s.RemotePort,
		Command:     s.Command,
		LastError:   s.LastError,
		ErrorTime:   s.ErrorTime,
		OnlineTime:  s.OnlineTime,
		HealthOK:    s.HealthOK,
		LastHealthy: s.LastHealthy,
	}
}
