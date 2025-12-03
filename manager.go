package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alinemone/go-port-forward/cert"
)

// Service status
const (
	StatusConnecting = "connecting"
	StatusHealthy    = "healthy"
	StatusError      = "error"
)

// ErrorEntry represents a single error event
type ErrorEntry struct {
	Time    time.Time
	Message string
}

// LogEntry represents a log message (stdout/stderr)
type LogEntry struct {
	Time    time.Time
	Message string
	IsError bool
}

// Service represents a running service
type Service struct {
	Name           string
	Command        string
	LocalPort      string
	RemotePort     string
	Status         string
	Error          string
	StartTime      time.Time
	ReconnectCount int
	ErrorHistory   []ErrorEntry
	LogHistory     []LogEntry
	cancel         context.CancelFunc
	mu             sync.RWMutex
}

// GetSnapshot returns a copy of service state
func (s *Service) GetSnapshot() Service {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Copy error history
	errorHistoryCopy := make([]ErrorEntry, len(s.ErrorHistory))
	copy(errorHistoryCopy, s.ErrorHistory)

	// Copy log history
	logHistoryCopy := make([]LogEntry, len(s.LogHistory))
	copy(logHistoryCopy, s.LogHistory)

	return Service{
		Name:           s.Name,
		Command:        s.Command,
		LocalPort:      s.LocalPort,
		RemotePort:     s.RemotePort,
		Status:         s.Status,
		Error:          s.Error,
		StartTime:      s.StartTime,
		ReconnectCount: s.ReconnectCount,
		ErrorHistory:   errorHistoryCopy,
		LogHistory:     logHistoryCopy,
	}
}

// addError adds an error to history (keeps last 10)
func (s *Service) addError(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := ErrorEntry{
		Time:    time.Now(),
		Message: msg,
	}

	s.ErrorHistory = append(s.ErrorHistory, entry)

	// Keep only last 10 errors
	if len(s.ErrorHistory) > 10 {
		s.ErrorHistory = s.ErrorHistory[len(s.ErrorHistory)-10:]
	}

	s.Error = msg
	s.Status = StatusError
}

// addLog adds a log entry to history (keeps last 100)
func (s *Service) addLog(msg string, isError bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Skip empty messages
	if strings.TrimSpace(msg) == "" {
		return
	}

	entry := LogEntry{
		Time:    time.Now(),
		Message: msg,
		IsError: isError,
	}

	s.LogHistory = append(s.LogHistory, entry)

	// Keep last 100 logs for full transparency
	if len(s.LogHistory) > 100 {
		s.LogHistory = s.LogHistory[len(s.LogHistory)-100:]
	}
}

// Manager manages multiple services
type Manager struct {
	services    map[string]*Service
	storage     *Storage
	certManager *cert.Manager
	mu          sync.RWMutex
}

// NewManager creates a new service manager
func NewManager(storage *Storage) *Manager {
	certMgr, err := cert.NewManager()
	if err != nil {
		// Log error but continue (certificates are optional)
		fmt.Fprintf(os.Stderr, "Warning: Failed to initialize certificate manager: %v\n", err)
		certMgr = nil
	}

	return &Manager{
		services:    make(map[string]*Service),
		storage:     storage,
		certManager: certMgr,
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
		Name:           name,
		Command:        command,
		LocalPort:      localPort,
		RemotePort:     remotePort,
		Status:         StatusConnecting,
		StartTime:      time.Now(),
		ReconnectCount: 0,
		ErrorHistory:   make([]ErrorEntry, 0),
		LogHistory:     make([]LogEntry, 0),
		cancel:         cancel,
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
	isFirstRun := true

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Increment reconnect count (except first run)
			if !isFirstRun {
				svc.mu.Lock()
				svc.ReconnectCount++
				svc.mu.Unlock()

				// Add reconnect log for transparency
				svc.addLog(fmt.Sprintf("━━━━ RECONNECTING (attempt #%d) ━━━━", svc.ReconnectCount), false)
			}
			isFirstRun = false

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

	// Prepare command with certificate if available
	commandStr := svc.Command
	if m.certManager != nil {
		if certConfig, exists := m.certManager.GetCertificate(); exists {
			// Inject certificate flags for kubectl commands
			if strings.Contains(commandStr, "kubectl") {
				commandStr = injectKubectlCert(commandStr, certConfig.CertPath, certConfig.KeyPath)
			}
		}
	}

	// Create command
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", commandStr)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", commandStr)
	}

	// Capture output
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	// Start process
	if err := cmd.Start(); err != nil {
		errorMsg := fmt.Sprintf("Start failed: %v", err)
		svc.addError(errorMsg)
		fmt.Fprintf(os.Stderr, "[%s] ERROR: %v\n", svc.Name, err)
		return
	}

	// Monitor output in background
	go m.monitorOutput(svc, stdoutPipe, stderrPipe, false) // stdout
	go m.monitorOutput(svc, stderrPipe, nil, true)         // stderr

	// Wait for process to exit
	err := cmd.Wait()
	if err != nil && ctx.Err() == nil {
		errorMsg := fmt.Sprintf("Process died: %v", err)
		svc.addError(errorMsg)
		fmt.Fprintf(os.Stderr, "[%s] ERROR: Process died: %v\n", svc.Name, err)
	}
}

// injectKubectlCert injects certificate flags into kubectl command
func injectKubectlCert(command, certPath, keyPath string) string {
	// Check if cert flags already exist
	if strings.Contains(command, "--client-certificate") {
		return command
	}

	// Find position after "kubectl" to inject flags
	re := regexp.MustCompile(`(kubectl\s+)`)
	if !re.MatchString(command) {
		return command
	}

	// Inject certificate and key flags
	certFlags := fmt.Sprintf("--client-certificate=%s --client-key=%s ", certPath, keyPath)
	result := re.ReplaceAllString(command, "${1}"+certFlags)

	return result
}

// monitorOutput monitors stdout/stderr and logs messages
func (m *Manager) monitorOutput(svc *Service, pipe interface{}, _ interface{}, isError bool) {
	if pipe == nil {
		return
	}

	buf := make([]byte, 8192)
	reader := pipe.(interface{ Read([]byte) (int, error) })

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			output := strings.TrimSpace(string(buf[:n]))

			// Split by lines to handle multiple messages
			lines := strings.Split(output, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}

				// Add to log history
				svc.addLog(line, isError)

				// Check if "Forwarding from" appears - means service is healthy
				if strings.Contains(line, "Forwarding from") {
					svc.mu.Lock()
					if svc.Status != StatusHealthy {
						svc.Status = StatusHealthy
						svc.Error = ""
					}
					svc.mu.Unlock()
				}

				// Check if it's an error (for stderr)
				if isError {
					lowerOutput := strings.ToLower(line)
					isErrorMsg := strings.Contains(lowerOutput, "error") ||
						strings.Contains(lowerOutput, "failed") ||
						strings.Contains(lowerOutput, "unable to") ||
						strings.Contains(lowerOutput, "cannot") ||
						strings.Contains(lowerOutput, "denied") ||
						strings.Contains(lowerOutput, "refused") ||
						strings.Contains(lowerOutput, "not found") ||
						strings.Contains(lowerOutput, "lost connection")

					if isErrorMsg {
						errorMsg := extractErrorMessage(line)
						svc.addError(errorMsg)
						fmt.Fprintf(os.Stderr, "[%s] ERROR: %s\n", svc.Name, errorMsg)
					}
				}
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

// Restart restarts a service
func (m *Manager) Restart(ctx context.Context, name string) error {
	// Stop the service
	m.Stop(name)

	// Wait a bit
	time.Sleep(500 * time.Millisecond)

	// Start again
	return m.Start(ctx, name)
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
