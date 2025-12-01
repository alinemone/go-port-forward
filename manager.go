package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Service status
const (
	StatusConnecting = "connecting"
	StatusHealthy    = "healthy"
	StatusError      = "error"
)

// Service represents a running service
type Service struct {
	Name       string
	Command    string
	LocalPort  string
	RemotePort string
	Status     string
	Error      string
	cancel     context.CancelFunc
	mu         sync.RWMutex
}

// GetSnapshot returns a copy of service state
func (s *Service) GetSnapshot() Service {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Service{
		Name:       s.Name,
		Command:    s.Command,
		LocalPort:  s.LocalPort,
		RemotePort: s.RemotePort,
		Status:     s.Status,
		Error:      s.Error,
	}
}

// Manager manages multiple services
type Manager struct {
	services map[string]*Service
	storage  *Storage
	mu       sync.RWMutex
}

// NewManager creates a new service manager
func NewManager(storage *Storage) *Manager {
	return &Manager{
		services: make(map[string]*Service),
		storage:  storage,
	}
}

// Start starts a service
func (m *Manager) Start(ctx context.Context, name string) error {
	// Load service command
	command, err := m.storage.Get(name)
	if err != nil {
		return err
	}

	// Extract ports
	localPort, remotePort := ExtractPorts(command)
	if localPort == "" {
		return fmt.Errorf("could not extract ports from command")
	}

	// Cleanup port if in use
	_ = killProcessUsingPort(localPort)
	time.Sleep(300 * time.Millisecond)

	// Create service
	svcCtx, cancel := context.WithCancel(ctx)
	svc := &Service{
		Name:       name,
		Command:    command,
		LocalPort:  localPort,
		RemotePort: remotePort,
		Status:     StatusConnecting,
		cancel:     cancel,
	}

	// Store service
	m.mu.Lock()
	m.services[name] = svc
	m.mu.Unlock()

	// Start runner
	go m.runService(svcCtx, svc)

	return nil
}

// runService runs a service with auto-reconnect
func (m *Manager) runService(ctx context.Context, svc *Service) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			m.runOnce(ctx, svc)
			// Wait 2 seconds before reconnecting
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}
}

// runOnce runs the service command once
func (m *Manager) runOnce(ctx context.Context, svc *Service) {
	svc.mu.Lock()
	svc.Status = StatusConnecting
	svc.Error = ""
	svc.mu.Unlock()

	// Create command
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", svc.Command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", svc.Command)
	}

	// Capture output
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	// Start process
	if err := cmd.Start(); err != nil {
		svc.mu.Lock()
		svc.Status = StatusError
		svc.Error = fmt.Sprintf("Start failed: %v", err)
		svc.mu.Unlock()
		fmt.Fprintf(os.Stderr, "[%s] ERROR: %v\n", svc.Name, err)
		return
	}

	// Monitor output in background
	go m.monitorOutput(svc, stdoutPipe, stderrPipe)

	// Start health check
	go m.healthCheck(ctx, svc)

	// Wait for process to exit
	err := cmd.Wait()
	if err != nil && ctx.Err() == nil {
		svc.mu.Lock()
		svc.Status = StatusError
		svc.Error = fmt.Sprintf("Process died: %v", err)
		svc.mu.Unlock()
		fmt.Fprintf(os.Stderr, "[%s] ERROR: Process died: %v\n", svc.Name, err)
	}
}

// monitorOutput monitors stdout/stderr for errors
func (m *Manager) monitorOutput(svc *Service, stdout, stderr interface{}) {
	if stderr == nil {
		return
	}

	buf := make([]byte, 8192)
	reader := stderr.(interface{ Read([]byte) (int, error) })

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			output := strings.TrimSpace(string(buf[:n]))
			lowerOutput := strings.ToLower(output)

			// Check for common error patterns
			isError := strings.Contains(lowerOutput, "error") ||
				strings.Contains(lowerOutput, "failed") ||
				strings.Contains(lowerOutput, "unable to") ||
				strings.Contains(lowerOutput, "cannot") ||
				strings.Contains(lowerOutput, "denied") ||
				strings.Contains(lowerOutput, "refused") ||
				strings.Contains(lowerOutput, "not found")

			if isError {
				// Extract meaningful error message
				errorMsg := extractErrorMessage(output)

				svc.mu.Lock()
				if svc.Status != StatusError {
					svc.Status = StatusError
					svc.Error = errorMsg
					fmt.Fprintf(os.Stderr, "[%s] ERROR: %s\n", svc.Name, errorMsg)
				}
				svc.mu.Unlock()
			}
		}
		if err != nil {
			break
		}
	}
}

// extractErrorMessage extracts a clean error message from output
func extractErrorMessage(output string) string {
	// Limit length
	if len(output) > 150 {
		output = output[:147] + "..."
	}

	// Remove extra whitespace
	output = strings.Join(strings.Fields(output), " ")

	return output
}

// healthCheck checks if port is open
func (m *Manager) healthCheck(ctx context.Context, svc *Service) {
	// Wait a bit for service to start
	time.Sleep(2 * time.Second)

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	failCount := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check if port is open
			conn, err := net.DialTimeout("tcp", "localhost:"+svc.LocalPort, 1*time.Second)
			if err != nil {
				failCount++
				if failCount >= 2 {
					svc.mu.Lock()
					if svc.Status != StatusError {
						svc.Status = StatusError
						svc.Error = "Port not accessible"
						fmt.Fprintf(os.Stderr, "[%s] ERROR: Port %s not accessible\n", svc.Name, svc.LocalPort)
					}
					svc.mu.Unlock()
				}
			} else {
				conn.Close()
				failCount = 0
				svc.mu.Lock()
				if svc.Status != StatusHealthy {
					svc.Status = StatusHealthy
					svc.Error = ""
				}
				svc.mu.Unlock()
			}
		}
	}
}

// Stop stops a service
func (m *Manager) Stop(name string) {
	m.mu.Lock()
	svc, exists := m.services[name]
	if !exists {
		m.mu.Unlock()
		return
	}
	delete(m.services, name)
	m.mu.Unlock()

	// Cancel context
	if svc.cancel != nil {
		svc.cancel()
	}

	// Cleanup port
	time.Sleep(300 * time.Millisecond)
	_ = killProcessUsingPort(svc.LocalPort)
}

// StopAll stops all services
func (m *Manager) StopAll() {
	m.mu.Lock()
	services := make([]*Service, 0, len(m.services))
	for _, svc := range m.services {
		services = append(services, svc)
		if svc.cancel != nil {
			svc.cancel()
		}
	}
	m.services = make(map[string]*Service)
	m.mu.Unlock()

	// Cleanup ports
	time.Sleep(300 * time.Millisecond)
	for _, svc := range services {
		_ = killProcessUsingPort(svc.LocalPort)
	}
}

// GetStates returns all service states sorted by name
func (m *Manager) GetStates() []Service {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make([]Service, 0, len(m.services))
	for _, svc := range m.services {
		states = append(states, svc.GetSnapshot())
	}

	// Sort by name for consistent display
	sort.Slice(states, func(i, j int) bool {
		return states[i].Name < states[j].Name
	})

	return states
}

// killProcessUsingPort kills processes using a port
func killProcessUsingPort(port string) error {
	if runtime.GOOS == "windows" {
		return killProcessUsingPortWindows(port)
	}
	return killProcessUsingPortUnix(port)
}

// killProcessUsingPortWindows kills processes on Windows
func killProcessUsingPortWindows(port string) error {
	cmd := exec.Command("netstat", "-ano")
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	lines := strings.Split(string(output), "\n")
	portPattern := regexp.MustCompile(`:` + regexp.QuoteMeta(port) + `\s`)

	pids := make(map[string]bool)
	for _, line := range lines {
		if portPattern.MatchString(line) {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				pid := fields[len(fields)-1]
				if pid != "0" {
					pids[pid] = true
				}
			}
		}
	}

	// Kill each PID
	for pid := range pids {
		exec.Command("taskkill", "/F", "/T", "/PID", pid).Run()
	}

	return nil
}

// killProcessUsingPortUnix kills processes on Linux/macOS
func killProcessUsingPortUnix(port string) error {
	// Try lsof first
	cmd := exec.Command("lsof", "-ti", ":"+port)
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		pids := strings.Fields(string(output))
		for _, pid := range pids {
			exec.Command("kill", "-9", pid).Run()
		}
		return nil
	}

	// Fallback to fuser
	cmd = exec.Command("fuser", "-k", port+"/tcp")
	cmd.Run()

	return nil
}
