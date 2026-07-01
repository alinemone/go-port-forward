package manager

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alinemone/go-port-forward/internal/cert"
	"github.com/alinemone/go-port-forward/internal/model"
	"github.com/alinemone/go-port-forward/internal/storage"
)

type runningService struct {
	name          string
	command       string
	localPort     string
	mainPort      string
	iconEnabled   bool
	iconGlyph     string
	iconColor     string
	status        string
	lastError     string
	startTime     time.Time
	restartCount  int
	healthySince  time.Time
	lastHealthy   time.Time
	lastRunStable bool
	logs          []model.LogEntry
	cancel        context.CancelFunc
	done          chan struct{}
	process       *os.Process
	mu            sync.RWMutex

	// bulkKill is set before cancelling during StopAllServices so the per-run
	// ctx.Done watcher skips its own taskkill — the whole fleet is killed in one
	// batched call instead of one spawn per service.
	bulkKill atomic.Bool
}

func (s *runningService) markHealthy() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status != model.StatusHealthy {
		s.status = model.StatusHealthy
		s.lastError = ""
	}
	now := time.Now()
	if s.healthySince.IsZero() {
		s.healthySince = now
	}
	s.lastHealthy = now
}

func (s *runningService) snapshot() model.Service {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logsCopy := make([]model.LogEntry, len(s.logs))
	copy(logsCopy, s.logs)

	return model.Service{
		Name:         s.name,
		Command:      s.command,
		LocalPort:    s.localPort,
		MainPort:     s.mainPort,
		IconEnabled:  s.iconEnabled,
		IconGlyph:    s.iconGlyph,
		IconColor:    s.iconColor,
		Status:       s.status,
		LastError:    s.lastError,
		StartTime:    s.startTime,
		RestartCount: s.restartCount,
		Logs:         logsCopy,
	}
}

func (s *runningService) setError(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastError = message
	s.status = model.StatusError
}

func (s *runningService) appendLog(message string, isError bool) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.logs = append(s.logs, model.LogEntry{
		Time:    time.Now(),
		Message: message,
		IsError: isError,
	})

	if len(s.logs) > 120 {
		s.logs = s.logs[len(s.logs)-120:]
	}
}

type ServiceManager struct {
	services    map[string]*runningService
	storage     *storage.Storage
	certManager *cert.Manager
	mu          sync.RWMutex
}

func NewServiceManager(st *storage.Storage) *ServiceManager {
	certMgr, err := cert.NewManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to initialize certificate manager: %v\n", err)
		certMgr = nil
	}

	return &ServiceManager{
		services:    make(map[string]*runningService),
		storage:     st,
		certManager: certMgr,
	}
}

func ValidateServiceName(name string) error {
	return ensureValidServiceName(name)
}

func ValidateCommand(command string) error {
	return ensureValidCommand(command)
}

func ensureValidServiceName(name string) error {
	if name == "" {
		return fmt.Errorf("service name cannot be empty")
	}
	if len(name) > 50 {
		return fmt.Errorf("service name too long (max 50 characters)")
	}

	dangerousChars := []string{"..", "/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range dangerousChars {
		if strings.Contains(name, char) {
			return fmt.Errorf("service name contains invalid character: %s", char)
		}
	}

	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, name)
	if !matched {
		return fmt.Errorf("service name can only contain letters, numbers, hyphens, and underscores")
	}

	return nil
}

func ensureValidCommand(command string) error {
	if command == "" {
		return fmt.Errorf("command cannot be empty")
	}
	if len(command) > 1000 {
		return fmt.Errorf("command too long (max 1000 characters)")
	}

	dangerousPatterns := []string{
		`rm\s+-rf`,
		`dd\s+if=`,
		`mkfs`,
		`format`,
		`del\s+/f`,
		`shutdown`,
		`reboot`,
		`halt`,
		`poweroff`,
	}

	lower := strings.ToLower(command)
	for _, pattern := range dangerousPatterns {
		matched, _ := regexp.MatchString(pattern, lower)
		if matched {
			return fmt.Errorf("command contains potentially dangerous operation: %s", pattern)
		}
	}

	return nil
}

func (m *ServiceManager) StartService(ctx context.Context, name string) error {
	if err := ensureValidServiceName(name); err != nil {
		return fmt.Errorf("invalid service name: %v", err)
	}

	command, err := m.storage.GetService(name)
	if err != nil {
		return err
	}

	if err := ensureValidCommand(command); err != nil {
		return fmt.Errorf("invalid command for service '%s': %v", name, err)
	}

	localPort, mainPort := storage.ParsePortsFromCommand(command)
	if localPort == "" {
		return fmt.Errorf("could not extract ports from command")
	}
	if mainPort == "" {
		mainPort = localPort
	}
	iconSet, iconEnabled, err := m.storage.IconSet()
	if err != nil {
		return err
	}
	icon := iconSet.ForPort(mainPort)

	svcCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	svc := &runningService{
		name:         name,
		command:      command,
		localPort:    localPort,
		mainPort:     mainPort,
		iconEnabled:  iconEnabled,
		iconGlyph:    icon.Glyph,
		iconColor:    icon.Color,
		status:       model.StatusConnecting,
		startTime:    time.Now(),
		restartCount: 0,
		logs:         make([]model.LogEntry, 0),
		cancel:       cancel,
		done:         done,
	}

	m.mu.Lock()
	m.services[name] = svc
	m.mu.Unlock()

	go func() {
		defer close(done)
		m.runServiceLoop(svcCtx, svc)
	}()

	return nil
}

func (m *ServiceManager) runServiceLoop(ctx context.Context, svc *runningService) {
	const baseBackoff = 2 * time.Second
	const maxBackoff = 30 * time.Second

	isFirstRun := true

	for {
		select {
		case <-ctx.Done():
			return
		default:
			if !isFirstRun {
				svc.mu.Lock()
				svc.restartCount = nextRestartCount(svc.restartCount, svc.lastRunStable)
				restartCount := svc.restartCount
				svc.mu.Unlock()

				backoff := baseBackoff * time.Duration(1<<uint(restartCount-1))
				if backoff > maxBackoff {
					backoff = maxBackoff
				}

				jitter := time.Duration(float64(backoff) * 0.1 * (rand.Float64()*2 - 1))
				backoff += jitter

				svc.appendLog(
					fmt.Sprintf("━━━━ RECONNECTING (attempt #%d) in %.1fs ━━━━", restartCount, backoff.Seconds()),
					false,
				)

				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
			}
			isFirstRun = false
			m.runServiceOnce(ctx, svc)
		}
	}
}

func (m *ServiceManager) runServiceOnce(ctx context.Context, svc *runningService) {
	svc.mu.Lock()
	svc.status = model.StatusConnecting
	svc.lastError = ""
	svc.healthySince = time.Time{}
	svc.mu.Unlock()

	commandStr := svc.command
	if m.certManager != nil {
		if certConfig, exists := m.certManager.GetCertificate(); exists {
			if strings.Contains(commandStr, "kubectl") {
				commandStr = addKubectlCertFlags(commandStr, certConfig.CertPath, certConfig.KeyPath)
			}
		}
	}

	cmd := newShellCommand(commandStr)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		message := fmt.Sprintf("Failed to create stdout pipe: %v", err)
		svc.setError(message)
		svc.appendLog(message, true)
		return
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		message := fmt.Sprintf("Failed to create stderr pipe: %v", err)
		svc.setError(message)
		svc.appendLog(message, true)
		return
	}

	if err := cmd.Start(); err != nil {
		message := fmt.Sprintf("Start failed: %v", err)
		svc.setError(message)
		if isStderrLoggingEnabled() {
			fmt.Fprintf(os.Stderr, "[%s] ERROR: %v\n", svc.name, err)
		}
		return
	}

	svc.mu.Lock()
	svc.process = cmd.Process
	svc.mu.Unlock()

	go func() {
		<-ctx.Done()
		// During a bulk shutdown, StopAllServices force-kills every process tree
		// in one batched call, so skip the per-service kill here.
		if svc.bulkKill.Load() {
			return
		}
		killProcessTree(cmd.Process)
	}()

	go m.streamOutput(svc, stdoutPipe, false)
	go m.streamOutput(svc, stderrPipe, true)

	err = cmd.Wait()

	svc.mu.Lock()
	svc.lastRunStable = !svc.healthySince.IsZero() && time.Since(svc.healthySince) >= healthyResetThreshold
	svc.process = nil
	svc.mu.Unlock()

	if err != nil && ctx.Err() == nil {
		message := fmt.Sprintf("Process died: %v", err)
		svc.setError(message)
	}
}

const healthyResetThreshold = 30 * time.Second

func nextRestartCount(prev int, lastRunStable bool) int {
	if lastRunStable {
		return 1
	}
	return prev + 1
}

func addKubectlCertFlags(command, certPath, keyPath string) string {
	if !strings.Contains(command, "kubectl ") {
		return command
	}

	certFlags := fmt.Sprintf(`--client-certificate="%s" --client-key="%s" `, certPath, keyPath)
	parts := strings.Split(command, "kubectl ")
	if len(parts) < 2 {
		return command
	}

	var out strings.Builder
	out.Grow(len(command) + len(certFlags)*(len(parts)-1))
	out.WriteString(parts[0])

	for _, part := range parts[1:] {
		trimmed := strings.TrimLeft(part, " \t")
		out.WriteString("kubectl ")
		if !(strings.HasPrefix(trimmed, "--client-certificate") || strings.HasPrefix(trimmed, "--client-key")) {
			out.WriteString(certFlags)
		}
		out.WriteString(part)
	}

	return out.String()
}

func killProcessTree(proc *os.Process) {
	if proc == nil {
		return
	}

	if runtime.GOOS == "windows" {
		pid := strconv.Itoa(proc.Pid)
		exec.Command("taskkill", "/F", "/T", "/PID", pid).Run()
	} else {
		killUnixProcessGroup(proc.Pid)
	}
}

func waitForPortRelease(port string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ln, err := net.Listen("tcp", "127.0.0.1:"+port)
		if err == nil {
			ln.Close()
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// shutdownGraceTimeout is how long a cancelled service is given to exit on its
// own before its process tree is force-killed. It's a var so tests can shrink it.
var shutdownGraceTimeout = 5 * time.Second

// awaitStopOrKill waits for a cancelled service's loop to finish on its own
// (svc.done closes once the process has exited), and force-kills the process
// tree if it doesn't within the grace period. The caller must have already
// invoked svc.cancel().
func awaitStopOrKill(svc *runningService) {
	if svc.done == nil {
		return
	}
	select {
	case <-svc.done:
	case <-time.After(shutdownGraceTimeout):
		svc.mu.RLock()
		proc := svc.process
		svc.mu.RUnlock()
		killProcessTree(proc)
	}
}

func (m *ServiceManager) StopService(name string) {
	m.mu.Lock()
	svc, exists := m.services[name]
	if !exists {
		m.mu.Unlock()
		return
	}
	delete(m.services, name)
	m.mu.Unlock()

	if svc.cancel != nil {
		svc.cancel()
	}

	awaitStopOrKill(svc)
}

func (m *ServiceManager) restartInPlace(ctx context.Context, name string) {
	m.mu.RLock()
	svc, exists := m.services[name]
	m.mu.RUnlock()

	if !exists {
		return
	}

	if svc.cancel != nil {
		svc.cancel()
	}

	if svc.done != nil {
		select {
		case <-svc.done:
		case <-time.After(5 * time.Second):
			svc.mu.RLock()
			proc := svc.process
			svc.mu.RUnlock()
			killProcessTree(proc)
			select {
			case <-svc.done:
			case <-time.After(2 * time.Second):
			}
		}
	}

	waitForPortRelease(svc.localPort, 5*time.Second)

	svcCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	svc.mu.Lock()
	svc.status = model.StatusConnecting
	svc.lastError = ""
	svc.startTime = time.Now()
	svc.restartCount = 0
	svc.cancel = cancel
	svc.done = done
	svc.mu.Unlock()

	go func() {
		defer close(done)
		m.runServiceLoop(svcCtx, svc)
	}()
}

func (m *ServiceManager) RestartService(ctx context.Context, name string) error {
	go m.restartInPlace(ctx, name)
	return nil
}

func (m *ServiceManager) RestartAllServices(ctx context.Context) {
	m.mu.RLock()
	names := make([]string, 0, len(m.services))
	for name := range m.services {
		names = append(names, name)
	}
	m.mu.RUnlock()

	for _, name := range names {
		go m.restartInPlace(ctx, name)
	}
}

func (m *ServiceManager) StartStoredService(ctx context.Context, name string) error {
	m.mu.RLock()
	_, exists := m.services[name]
	m.mu.RUnlock()

	if exists {
		return fmt.Errorf("service '%s' is already running", name)
	}

	if _, err := m.storage.GetService(name); err != nil {
		return fmt.Errorf("service '%s' not found in storage", name)
	}

	return m.StartService(ctx, name)
}

// StopAllServices tears down every running service as fast as possible. It marks
// each service for bulk kill and cancels it (so the loops stop and their own
// ctx.Done watchers stand down), then force-kills all their process trees in a
// single batched call. On Windows that means one taskkill spawn for the whole
// fleet instead of one per service; on Unix each kill is a direct syscall.
func (m *ServiceManager) StopAllServices() {
	m.mu.Lock()
	services := make([]*runningService, 0, len(m.services))
	for _, svc := range m.services {
		services = append(services, svc)
		svc.bulkKill.Store(true)
		if svc.cancel != nil {
			svc.cancel()
		}
	}
	m.services = make(map[string]*runningService)
	m.mu.Unlock()

	procs := make([]*os.Process, 0, len(services))
	for _, svc := range services {
		svc.mu.RLock()
		proc := svc.process
		svc.mu.RUnlock()
		if proc != nil {
			procs = append(procs, proc)
		}
	}

	killProcessTrees(procs)
}

func (m *ServiceManager) ListServiceStates() []model.Service {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make([]model.Service, 0, len(m.services))
	for _, svc := range m.services {
		states = append(states, svc.snapshot())
	}

	sort.Slice(states, func(i, j int) bool {
		return states[i].Name < states[j].Name
	})

	return states
}

func (m *ServiceManager) streamOutput(svc *runningService, reader io.Reader, isError bool) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		svc.appendLog(line, isError)

		switch classifyOutputLine(line, isError) {
		case lineKindHealthy:
			svc.markHealthy()
		case lineKindFatalError:
			message := normalizeErrorLine(line)
			svc.setError(message)
			if isStderrLoggingEnabled() {
				fmt.Fprintf(os.Stderr, "[%s] ERROR: %s\n", svc.name, message)
			}
		}
	}
}
