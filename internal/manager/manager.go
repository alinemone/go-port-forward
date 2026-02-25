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
	"time"

	"github.com/alinemone/go-port-forward/internal/cert"
	"github.com/alinemone/go-port-forward/internal/model"
	"github.com/alinemone/go-port-forward/internal/storage"
)

// runningService نگهداری state اجرایی سرویس (جدا از model.Service)
type runningService struct {
	name         string
	command      string
	localPort    string
	status       string
	lastError    string
	startTime    time.Time
	restartCount int
	logs         []model.LogEntry
	cancel       context.CancelFunc
	done         chan struct{}
	process      *os.Process
	mu           sync.RWMutex
}

// تهیه کپی امن از وضعیت سرویس برای UI
func (s *runningService) snapshot() model.Service {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logsCopy := make([]model.LogEntry, len(s.logs))
	copy(logsCopy, s.logs)

	return model.Service{
		Name:         s.name,
		Command:      s.command,
		LocalPort:    s.localPort,
		Status:       s.status,
		LastError:    s.lastError,
		StartTime:    s.startTime,
		RestartCount: s.restartCount,
		Logs:         logsCopy,
	}
}

// ثبت خطای سرویس و به‌روزرسانی وضعیت
func (s *runningService) setError(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastError = message
	s.status = model.StatusError
}

// افزودن لاگ به سرویس با نگه‌داری آخرین N پیام
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

// مدیر اجرای چند سرویس هم‌زمان
type ServiceManager struct {
	services    map[string]*runningService
	storage     *storage.Storage
	certManager *cert.Manager
	mu          sync.RWMutex
}

// ساخت مدیر سرویس‌ها با پشتیبانی گواهی
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

	localPort, _ := storage.ParsePortsFromCommand(command)
	if localPort == "" {
		return fmt.Errorf("could not extract ports from command")
	}

	svcCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	svc := &runningService{
		name:         name,
		command:      command,
		localPort:    localPort,
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

// حلقه اجرای سرویس با قابلیت اتصال مجدد
func (m *ServiceManager) runServiceLoop(ctx context.Context, svc *runningService) {
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
				svc.restartCount++
				restartCount := svc.restartCount
				svc.mu.Unlock()

				if restartCount >= maxReconnects {
					svc.mu.Lock()
					svc.status = model.StatusError
					svc.lastError = fmt.Sprintf("Max reconnect attempts (%d) exceeded", maxReconnects)
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
func (m *ServiceManager) runServiceOnce(ctx context.Context, svc *runningService) {
	svc.mu.Lock()
	svc.status = model.StatusConnecting
	svc.lastError = ""
	svc.mu.Unlock()

	commandStr := svc.command
	if m.certManager != nil {
		if certConfig, exists := m.certManager.GetCertificate(); exists {
			if strings.Contains(commandStr, "kubectl") {
				commandStr = addKubectlCertFlags(commandStr, certConfig.CertPath, certConfig.KeyPath)
			}
		}
	}

	// ساخت command بدون CommandContext — خودمون kill رو مدیریت میکنیم
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", commandStr)
	} else {
		cmd = exec.Command("sh", "-c", commandStr)
	}
	// ساخت process group جدید تا بتونیم کل درخت پروسه رو بکشیم
	cmd.SysProcAttr = newProcessGroupAttr()

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

	// ذخیره پروسه برای kill امن
	svc.mu.Lock()
	svc.process = cmd.Process
	svc.mu.Unlock()

	// goroutine برای kill کردن پروسه وقتی context لغو بشه
	go func() {
		<-ctx.Done()
		killProcessTree(cmd.Process)
	}()

	go m.streamOutput(svc, stdoutPipe, false)
	go m.streamOutput(svc, stderrPipe, true)

	err = cmd.Wait()

	// پاک‌سازی رفرنس پروسه
	svc.mu.Lock()
	svc.process = nil
	svc.mu.Unlock()

	if err != nil && ctx.Err() == nil {
		message := fmt.Sprintf("Process died: %v", err)
		svc.setError(message)
	}
}

// افزودن فلگ‌های گواهی به فرمان kubectl
func addKubectlCertFlags(command, certPath, keyPath string) string {
	if strings.Contains(command, "--client-certificate") {
		return command
	}

	certFlags := fmt.Sprintf("--client-certificate=%s --client-key=%s ", certPath, keyPath)

	idx := strings.Index(command, "kubectl ")
	if idx == -1 {
		return command
	}

	insertPos := idx + len("kubectl ")
	return command[:insertPos] + certFlags + command[insertPos:]
}

// کشتن کل درخت پروسه (شامل child process‌ها)
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

// صبر تا پورت آزاد بشه
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

	// منتظر پایان واقعی goroutine
	if svc.done != nil {
		select {
		case <-svc.done:
		case <-time.After(5 * time.Second):
			svc.mu.RLock()
			proc := svc.process
			svc.mu.RUnlock()
			killProcessTree(proc)
		}
	}
}

// راه‌اندازی مجدد یک سرویس بدون حذف از map (برای جلوگیری از پرش UI)
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

// RestartService راه‌اندازی مجدد یک سرویس
func (m *ServiceManager) RestartService(ctx context.Context, name string) error {
	go m.restartInPlace(ctx, name)
	return nil
}

// RestartAllServices راه‌اندازی مجدد همه سرویس‌های در حال اجرا
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

// StartStoredService افزودن سرویس ذخیره‌شده به اجرای فعلی
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

// StopAllServices توقف همه سرویس‌ها و انتظار برای پایان واقعی
func (m *ServiceManager) StopAllServices() {
	m.mu.Lock()
	services := make([]*runningService, 0, len(m.services))
	for _, svc := range m.services {
		services = append(services, svc)
		if svc.cancel != nil {
			svc.cancel()
		}
	}
	m.services = make(map[string]*runningService)
	m.mu.Unlock()

	for _, svc := range services {
		if svc.done != nil {
			select {
			case <-svc.done:
			case <-time.After(5 * time.Second):
				svc.mu.RLock()
				proc := svc.process
				svc.mu.RUnlock()
				killProcessTree(proc)
			}
		}
	}
}

// ListServiceStates دریافت وضعیت همه سرویس‌ها برای نمایش
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

// خواندن خطوط خروجی و ثبت در لاگ سرویس
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
			svc.mu.Lock()
			if svc.status != model.StatusHealthy {
				svc.status = model.StatusHealthy
				svc.lastError = ""
			}
			svc.mu.Unlock()
		case lineKindFatalError:
			message := normalizeErrorLine(line)
			svc.setError(message)
			if isStderrLoggingEnabled() {
				fmt.Fprintf(os.Stderr, "[%s] ERROR: %s\n", svc.name, message)
			}
		}
	}
}
