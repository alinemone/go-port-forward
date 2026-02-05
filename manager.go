package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand"
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

// مدل وضعیت سرویس در حال اجرا
type Service struct {
	Name         string
	Command      string
	LocalPort    string
	Status       string
	LastError    string
	StartTime    time.Time
	RestartCount int
	Logs         []LogEntry
	cancel       context.CancelFunc
	mu           sync.RWMutex
}

// تهیه کپی امن از وضعیت سرویس برای UI
func (s *Service) Snapshot() Service {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logsCopy := make([]LogEntry, len(s.Logs))
	copy(logsCopy, s.Logs)

	return Service{
		Name:         s.Name,
		Command:      s.Command,
		LocalPort:    s.LocalPort,
		Status:       s.Status,
		LastError:    s.LastError,
		StartTime:    s.StartTime,
		RestartCount: s.RestartCount,
		Logs:         logsCopy,
	}
}

// ثبت خطای سرویس و به‌روزرسانی وضعیت
func (s *Service) setError(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.LastError = message
	s.Status = StatusError
}

// افزودن لاگ به سرویس با نگه‌داری آخرین N پیام
func (s *Service) appendLog(message string, isError bool) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.Logs = append(s.Logs, LogEntry{
		Time:    time.Now(),
		Message: message,
		IsError: isError,
	})

	if len(s.Logs) > 120 {
		s.Logs = s.Logs[len(s.Logs)-120:]
	}
}

// مدیر اجرای چند سرویس هم‌زمان
type ServiceManager struct {
	services    map[string]*Service
	storage     *Storage
	certManager *cert.Manager
	mu          sync.RWMutex
}

// ساخت مدیر سرویس‌ها با پشتیبانی گواهی
func NewServiceManager(storage *Storage) *ServiceManager {
	certMgr, err := cert.NewManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to initialize certificate manager: %v\n", err)
		certMgr = nil
	}

	return &ServiceManager{
		services:    make(map[string]*Service),
		storage:     storage,
		certManager: certMgr,
	}
}

// اعتبارسنجی نام سرویس برای امنیت
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

// اعتبارسنجی فرمان اجرا برای جلوگیری از عملیات خطرناک
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

// شروع اجرای سرویس و ثبت در مدیر
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

	localPort, _ := parsePortsFromCommand(command)
	if localPort == "" {
		return fmt.Errorf("could not extract ports from command")
	}

	if err := releasePort(localPort); err != nil {
		fmt.Printf("Warning: Failed to clean up port %s: %v\n", localPort, err)
	}
	time.Sleep(300 * time.Millisecond)

	svcCtx, cancel := context.WithCancel(ctx)
	svc := &Service{
		Name:         name,
		Command:      command,
		LocalPort:    localPort,
		Status:       StatusConnecting,
		StartTime:    time.Now(),
		RestartCount: 0,
		Logs:         make([]LogEntry, 0),
		cancel:       cancel,
	}

	m.mu.Lock()
	m.services[name] = svc
	m.mu.Unlock()

	go m.runServiceLoop(svcCtx, svc)

	return nil
}

// حلقه اجرای سرویس با قابلیت اتصال مجدد
func (m *ServiceManager) runServiceLoop(ctx context.Context, svc *Service) {
	const maxReconnects = 10
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
				svc.RestartCount++
				restartCount := svc.RestartCount
				svc.mu.Unlock()

				if restartCount >= maxReconnects {
					svc.mu.Lock()
					svc.Status = StatusError
					svc.LastError = fmt.Sprintf("Max reconnect attempts (%d) exceeded", maxReconnects)
					svc.mu.Unlock()
					svc.appendLog("MAXIMUM RECONNECT ATTEMPTS REACHED - GIVING UP", true)
					return
				}

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

// اجرای یک‌باره فرمان سرویس و پایش خروجی‌ها
func (m *ServiceManager) runServiceOnce(ctx context.Context, svc *Service) {
	svc.mu.Lock()
	svc.Status = StatusConnecting
	svc.LastError = ""
	svc.mu.Unlock()

	commandStr := svc.Command
	if m.certManager != nil {
		if certConfig, exists := m.certManager.GetCertificate(); exists {
			if strings.Contains(commandStr, "kubectl") {
				commandStr = addKubectlCertFlags(commandStr, certConfig.CertPath, certConfig.KeyPath)
			}
		}
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", commandStr)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", commandStr)
	}

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
			fmt.Fprintf(os.Stderr, "[%s] ERROR: %v\n", svc.Name, err)
		}
		return
	}

	go m.streamOutput(svc, stdoutPipe, false)
	go m.streamOutput(svc, stderrPipe, true)

	err = cmd.Wait()
	if err != nil && ctx.Err() == nil {
		message := fmt.Sprintf("Process died: %v", err)
		svc.setError(message)
		if isStderrLoggingEnabled() {
			fmt.Fprintf(os.Stderr, "[%s] ERROR: Process died: %v\n", svc.Name, err)
		}
	}
}

// افزودن فلگ‌های گواهی به فرمان kubectl
func addKubectlCertFlags(command, certPath, keyPath string) string {
	if strings.Contains(command, "--client-certificate") {
		return command
	}

	re := regexp.MustCompile(`(kubectl\s+)`)
	if !re.MatchString(command) {
		return command
	}

	certFlags := fmt.Sprintf("--client-certificate=%s --client-key=%s ", certPath, keyPath)
	return re.ReplaceAllString(command, "${1}"+certFlags)
}

// خواندن خطوط خروجی و ثبت در لاگ سرویس
func (m *ServiceManager) streamOutput(svc *Service, reader io.Reader, isError bool) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		svc.appendLog(line, isError)

		if strings.Contains(line, "Forwarding from") {
			svc.mu.Lock()
			if svc.Status != StatusHealthy {
				svc.Status = StatusHealthy
				svc.LastError = ""
			}
			svc.mu.Unlock()
		}

		if isError {
			if looksLikeError(line) {
				message := normalizeErrorLine(line)
				svc.setError(message)
				if isStderrLoggingEnabled() {
					fmt.Fprintf(os.Stderr, "[%s] ERROR: %s\n", svc.Name, message)
				}
			}
		}
	}
}

// تشخیص ساده خطا از روی متن خروجی
func looksLikeError(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "unable to") ||
		strings.Contains(lower, "cannot") ||
		strings.Contains(lower, "denied") ||
		strings.Contains(lower, "refused") ||
		strings.Contains(lower, "not found") ||
		strings.Contains(lower, "lost connection")
}

// کوتاه‌سازی پیام خطا برای نمایش
func normalizeErrorLine(line string) string {
	if len(line) > 150 {
		line = line[:147] + "..."
	}
	return strings.Join(strings.Fields(line), " ")
}

// فعال بودن لاگ خطا در ترمینال بر اساس متغیر محیطی
func isStderrLoggingEnabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("PF_STDERR")))
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

// توقف یک سرویس در حال اجرا
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

	time.Sleep(300 * time.Millisecond)
	if err := releasePort(svc.LocalPort); err != nil {
		fmt.Printf("Warning: Failed to clean up port %s during restart: %v\n", svc.LocalPort, err)
	}
}

// راه‌اندازی مجدد یک سرویس
func (m *ServiceManager) RestartService(ctx context.Context, name string) error {
	m.StopService(name)
	time.Sleep(500 * time.Millisecond)
	return m.StartService(ctx, name)
}

// افزودن سرویس ذخیره‌شده به اجرای فعلی
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

// توقف همه سرویس‌ها و پاک‌سازی پورت‌ها
func (m *ServiceManager) StopAllServices() {
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

	time.Sleep(300 * time.Millisecond)
	for _, svc := range services {
		if err := releasePort(svc.LocalPort); err != nil {
			fmt.Printf("Warning: Failed to clean up port %s during cleanup: %v\n", svc.LocalPort, err)
		}
	}
}

// دریافت وضعیت همه سرویس‌ها برای نمایش
func (m *ServiceManager) ListServiceStates() []Service {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make([]Service, 0, len(m.services))
	for _, svc := range m.services {
		states = append(states, svc.Snapshot())
	}

	sort.Slice(states, func(i, j int) bool {
		return states[i].Name < states[j].Name
	})

	return states
}

// آزادسازی پورت از پردازش‌های در حال استفاده
func releasePort(port string) error {
	if runtime.GOOS == "windows" {
		return releasePortWindows(port)
	}
	return releasePortUnix(port)
}

// آزادسازی پورت در ویندوز
func releasePortWindows(port string) error {
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

	for pid := range pids {
		exec.Command("taskkill", "/F", "/T", "/PID", pid).Run()
	}

	return nil
}

// آزادسازی پورت در لینوکس/مک
func releasePortUnix(port string) error {
	cmd := exec.Command("lsof", "-ti", ":"+port)
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		pids := strings.Fields(string(output))
		for _, pid := range pids {
			exec.Command("kill", "-9", pid).Run()
		}
		return nil
	}

	cmd = exec.Command("fuser", "-k", port+"/tcp")
	cmd.Run()
	return nil
}
