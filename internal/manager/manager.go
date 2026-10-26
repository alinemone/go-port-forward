package manager

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alinemone/go-port-forward/internal/cert"
	"github.com/alinemone/go-port-forward/internal/kube"
	"github.com/alinemone/go-port-forward/internal/model"
	"github.com/alinemone/go-port-forward/internal/storage"
)

// shutdownGraceTimeout bounds how long stopping waits for a service's run loop
// to exit. Forwards stop promptly on cancel (their listeners close at once); the
// cap only matters if a tunnel is still mid-dial to an unreachable cluster. A
// var so tests can shrink it.
var shutdownGraceTimeout = 3 * time.Second

// forwardFunc runs one tunnel until ctx is cancelled or it breaks. A field on
// ServiceManager so tests can substitute a fake cluster.
type forwardFunc func(ctx context.Context, spec kube.Spec, opts kube.Options, hooks kube.Hooks) error

type runningService struct {
	name          string
	command       string
	spec          kube.Spec
	localPort     string
	mainPort      string
	iconEnabled   bool
	iconGlyph     string
	iconColor     string
	status        string
	lastError     string
	startTime     time.Time
	restartCount  int
	alias         string
	healthySince  time.Time
	lastRunStable bool
	logs          []model.LogEntry
	pod           string
	namespace     string
	stats         *kube.Stats
	cancel        context.CancelFunc
	done          chan struct{}
	mu            sync.RWMutex
}

func (s *runningService) markHealthy() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.status = model.StatusHealthy
	s.lastError = ""
	if s.healthySince.IsZero() {
		s.healthySince = time.Now()
	}
}

func (s *runningService) snapshot() model.Service {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logsCopy := make([]model.LogEntry, len(s.logs))
	copy(logsCopy, s.logs)

	svc := model.Service{
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
		Alias:        s.alias,
		Logs:         logsCopy,

		Target:         s.spec.Target(),
		Namespace:      s.namespace,
		Pod:            s.pod,
		ConnectedSince: s.healthySince,
	}
	if s.stats != nil {
		svc.ActiveConns = s.stats.ActiveConns.Load()
		svc.TotalConns = s.stats.TotalConns.Load()
		svc.BytesIn = s.stats.BytesIn.Load()
		svc.BytesOut = s.stats.BytesOut.Load()
		svc.LastActivity = s.stats.LastActivity()
		svc.Latency = s.stats.Latency()
	}
	return svc
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

func (s *runningService) clearLogs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = nil
}

// stop cancels the run loop and waits, bounded by shutdownGraceTimeout, for it
// to exit — by which point the tunnel's local listeners are closed.
func (s *runningService) stop() {
	s.mu.RLock()
	cancel, done := s.cancel, s.done
	s.mu.RUnlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(shutdownGraceTimeout):
		}
	}
}

type ServiceManager struct {
	services    map[string]*runningService
	storage     *storage.Storage
	certManager *cert.Manager
	forward     forwardFunc
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
		forward:     kube.Forward,
	}
}

func ValidateServiceName(name string) error {
	return ensureValidServiceName(name)
}

func ValidateCommand(command string) error {
	return ensureValidCommand(command)
}

var serviceNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func ensureValidServiceName(name string) error {
	if name == "" {
		return fmt.Errorf("service name cannot be empty")
	}
	if len(name) > 50 {
		return fmt.Errorf("service name too long (max 50 characters)")
	}
	if !serviceNameRegex.MatchString(name) {
		return fmt.Errorf("service name can only contain letters, numbers, hyphens, and underscores")
	}
	return nil
}

// ensureValidCommand accepts exactly the commands pf can run: a parseable
// `kubectl port-forward`. Nothing is handed to a shell, so there is no command
// blacklist to maintain.
func ensureValidCommand(command string) error {
	if command == "" {
		return fmt.Errorf("command cannot be empty")
	}
	if len(command) > 1000 {
		return fmt.Errorf("command too long (max 1000 characters)")
	}
	_, err := kube.ParseCommand(command)
	return err
}

func (m *ServiceManager) StartService(ctx context.Context, name string) error {
	if err := ensureValidServiceName(name); err != nil {
		return fmt.Errorf("invalid service name: %v", err)
	}

	command, err := m.storage.GetService(name)
	if err != nil {
		return err
	}
	spec, err := kube.ParseCommand(command)
	if err != nil {
		return fmt.Errorf("invalid command for service '%s': %v", name, err)
	}

	localPort := strconv.Itoa(spec.Ports[0].Local)
	mainPort := spec.Ports[0].Remote
	iconSet, iconEnabled, err := m.storage.IconSet()
	if err != nil {
		return err
	}
	icon := iconSet.ForPort(mainPort)

	svcCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	svc := &runningService{
		name:        name,
		command:     command,
		spec:        spec,
		stats:       &kube.Stats{},
		localPort:   localPort,
		mainPort:    mainPort,
		iconEnabled: iconEnabled,
		iconGlyph:   icon.Glyph,
		iconColor:   icon.Color,
		status:      model.StatusConnecting,
		startTime:   time.Now(),
		logs:        make([]model.LogEntry, 0),
		cancel:      cancel,
		done:        done,
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

	for isFirstRun := true; ; isFirstRun = false {
		if ctx.Err() != nil {
			return
		}
		if !isFirstRun {
			svc.mu.Lock()
			svc.restartCount = nextRestartCount(svc.restartCount, svc.lastRunStable)
			restartCount := svc.restartCount
			svc.mu.Unlock()

			backoff := baseBackoff * time.Duration(1<<uint(min(restartCount-1, 5)))
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			backoff += time.Duration(float64(backoff) * 0.1 * (rand.Float64()*2 - 1))

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
		m.runServiceOnce(ctx, svc)
	}
}

func (m *ServiceManager) runServiceOnce(ctx context.Context, svc *runningService) {
	svc.mu.Lock()
	svc.status = model.StatusConnecting
	svc.lastError = ""
	svc.healthySince = time.Time{}
	svc.mu.Unlock()

	err := m.forward(ctx, svc.spec, m.forwardOptions(), kube.Hooks{
		Log:    svc.appendLog,
		Ready:  svc.markHealthy,
		Target: svc.setTarget,
		Stats:  svc.stats,
	})

	svc.mu.Lock()
	svc.lastRunStable = !svc.healthySince.IsZero() && time.Since(svc.healthySince) >= healthyResetThreshold
	svc.healthySince = time.Time{}
	svc.mu.Unlock()

	if ctx.Err() != nil {
		return
	}
	if err == nil {
		err = errors.New("port-forward ended unexpectedly")
	}
	svc.appendLog(err.Error(), true)
	svc.setError(normalizeErrorLine(err.Error()))
}

// forwardOptions reads the configured client certificate on every attempt, so
// a certificate added with `pf cert add` applies on the next reconnect.
func (m *ServiceManager) forwardOptions() kube.Options {
	if m.certManager == nil {
		return kube.Options{}
	}
	certConfig, ok := m.certManager.GetCertificate()
	if !ok {
		return kube.Options{}
	}
	return kube.Options{ClientCert: certConfig.CertPath, ClientKey: certConfig.KeyPath}
}

const healthyResetThreshold = 30 * time.Second

func nextRestartCount(prev int, lastRunStable bool) int {
	if lastRunStable {
		return 1
	}
	return prev + 1
}

func normalizeErrorLine(line string) string {
	line = strings.Join(strings.Fields(line), " ")
	if len(line) > 150 {
		line = line[:147] + "..."
	}
	return line
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

	svc.stop()
}

func (m *ServiceManager) restartInPlace(ctx context.Context, name string) {
	m.mu.RLock()
	svc, exists := m.services[name]
	m.mu.RUnlock()

	if !exists {
		return
	}

	svc.stop()

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

// StopAllServices cancels every running service at once and waits for them
// concurrently, so shutdown takes as long as the slowest single service rather
// than their sum.
func (m *ServiceManager) StopAllServices() {
	m.mu.Lock()
	services := make([]*runningService, 0, len(m.services))
	for _, svc := range m.services {
		services = append(services, svc)
	}
	m.services = make(map[string]*runningService)
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, svc := range services {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.stop()
		}()
	}
	wg.Wait()
}

// NoteAlias surfaces, in a running service's log pane, that it is also reachable
// by one or more cluster/custom hostnames on its local port. Called only after
// the hosts-file entries are actually applied, so it never advertises an alias
// that isn't live. A no-op if the service isn't currently running or hosts is
// empty.
func (m *ServiceManager) NoteAlias(name string, hosts []string, localPort string) {
	if len(hosts) == 0 {
		return
	}
	m.mu.RLock()
	svc, exists := m.services[name]
	m.mu.RUnlock()
	if !exists {
		return
	}

	svc.mu.Lock()
	svc.alias = hosts[0] // primary hostname shown in the table / yanked with `y`
	svc.mu.Unlock()

	reachable := make([]string, len(hosts))
	for i, h := range hosts {
		reachable[i] = h + ":" + localPort
	}
	svc.appendLog("✓ cluster alias active — also reachable at "+strings.Join(reachable, ", "), false)
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

// ClearLogs clears the in-memory log history. An empty service name clears the
// history for every running service; otherwise only the named service is
// affected. Process state and forwarding are left untouched.
func (m *ServiceManager) ClearLogs(serviceName string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if serviceName != "" {
		if svc, ok := m.services[serviceName]; ok {
			svc.clearLogs()
		}
		return
	}

	for _, svc := range m.services {
		svc.clearLogs()
	}
}

func (s *runningService) setTarget(pod, namespace string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pod = pod
	s.namespace = namespace
}
