package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/alinemone/go-port-forward/internal/config"
	"github.com/alinemone/go-port-forward/internal/logger"
	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/pkg/netutil"
)

// Manager coordinates multiple services.
type Manager struct {
	services map[string]*State
	storage  *storage.Storage
	logger   *logger.Logger
	config   *config.Config
	mu       sync.RWMutex
}

// NewManager creates a new service manager.
func NewManager(storage *storage.Storage, logger *logger.Logger, cfg *config.Config) *Manager {
	return &Manager{
		services: make(map[string]*State),
		storage:  storage,
		logger:   logger,
		config:   cfg,
	}
}

// Start starts a service by name.
func (m *Manager) Start(ctx context.Context, name string) error {
	// Load service definition
	svcDef, err := m.storage.GetService(name)
	if err != nil {
		return err
	}

	// Extract ports
	localPort, remotePort, ok := storage.ExtractPorts(svcDef.Command)
	if !ok {
		return fmt.Errorf("failed to extract ports from command: %s", svcDef.Command)
	}

	// Aggressively cleanup any processes using this port (retry up to 3 times)
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if !netutil.IsPortInUse(localPort) {
			break // Port is free
		}

		m.logger.Warn("Port %s is in use (attempt %d/%d), killing processes...", localPort, attempt, maxRetries)

		// Kill processes using this port
		if err := KillProcessUsingPort(localPort); err != nil {
			m.logger.Error("Failed to kill processes on port %s: %v", localPort, err)
		}

		// Wait longer on each retry
		waitTime := time.Duration(attempt) * 500 * time.Millisecond
		time.Sleep(waitTime)

		// If this was the last attempt and port is still in use, fail
		if attempt == maxRetries && netutil.IsPortInUse(localPort) {
			return fmt.Errorf("port %s is still in use after %d cleanup attempts - please manually kill processes using this port", localPort, maxRetries)
		}
	}

	// Create service context
	svcCtx, cancel := context.WithCancel(ctx)

	// Create state
	state := &State{
		Name:       name,
		Status:     StatusConnecting,
		LocalPort:  localPort,
		RemotePort: remotePort,
		Command:    svcDef.Command,
		cancel:     cancel,
	}

	// Store state
	m.mu.Lock()
	m.services[name] = state
	m.mu.Unlock()

	// Start runner
	runner := NewRunner(state, m.logger)
	go runner.Run(svcCtx)

	// Start health checker
	healthChecker := NewHealthChecker(
		state,
		m.logger,
		m.config.HealthCheckInterval,
		m.config.HealthCheckTimeout,
		m.config.HealthCheckFailCount,
	)
	go healthChecker.Start(svcCtx)

	m.logger.ServiceEvent(name, "Service started")

	return nil
}

// Stop stops a service by name.
func (m *Manager) Stop(name string) error {
	m.mu.Lock()
	state, exists := m.services[name]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("service %q is not running", name)
	}

	localPort := state.LocalPort
	m.mu.Unlock()

	// Cancel the context to stop the service
	if state.cancel != nil {
		state.cancel()
	}

	// Wait a moment for process to die
	time.Sleep(300 * time.Millisecond)

	// Clean up any lingering processes on this port
	if err := KillProcessUsingPort(localPort); err != nil {
		m.logger.Error("Failed to cleanup port %s after stopping service %s: %v", localPort, name, err)
	}

	// Remove from map
	m.mu.Lock()
	delete(m.services, name)
	m.mu.Unlock()

	m.logger.ServiceEvent(name, "Service stopped and port %s cleaned up", localPort)

	return nil
}

// StopAll stops all running services.
func (m *Manager) StopAll() {
	m.mu.Lock()
	servicesToStop := make([]struct {
		name string
		port string
	}, 0, len(m.services))

	for name, state := range m.services {
		if state.cancel != nil {
			state.cancel()
		}
		servicesToStop = append(servicesToStop, struct {
			name string
			port string
		}{name, state.LocalPort})
		m.logger.ServiceEvent(name, "Service stopped")
	}

	m.services = make(map[string]*State)
	m.mu.Unlock()

	// Wait for processes to die
	time.Sleep(300 * time.Millisecond)

	// Clean up ports
	for _, svc := range servicesToStop {
		if err := KillProcessUsingPort(svc.port); err != nil {
			m.logger.Error("Failed to cleanup port %s for service %s: %v", svc.port, svc.name, err)
		}
	}
}

// GetStates returns snapshots of all service states.
func (m *Manager) GetStates() []State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make([]State, 0, len(m.services))
	for _, state := range m.services {
		states = append(states, state.GetSnapshot())
	}

	return states
}

// GetState returns a snapshot of a specific service state.
func (m *Manager) GetState(name string) (State, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.services[name]
	if !exists {
		return State{}, false
	}

	return state.GetSnapshot(), true
}

// IsRunning checks if a service is currently running.
func (m *Manager) IsRunning(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.services[name]
	return exists
}
