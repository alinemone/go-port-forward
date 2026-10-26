package manager

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/alinemone/go-port-forward/internal/kube"
	"github.com/alinemone/go-port-forward/internal/model"
	"github.com/alinemone/go-port-forward/internal/storage"
)

// newTestManager returns a manager backed by an isolated config dir whose
// tunnels are run by forward instead of a real cluster.
func newTestManager(t *testing.T, forward forwardFunc, services map[string]string) *ServiceManager {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	st := storage.NewStorage()
	for name, command := range services {
		if err := st.AddService(name, command); err != nil {
			t.Fatalf("AddService(%s): %v", name, err)
		}
	}
	return &ServiceManager{
		services: make(map[string]*runningService),
		storage:  st,
		forward:  forward,
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func portFree(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// listeningForward imitates a healthy tunnel: it binds the spec's local port,
// reports ready, and holds the port until cancelled.
func listeningForward(ctx context.Context, spec kube.Spec, _ kube.Options, hooks kube.Hooks) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", spec.Ports[0].Local))
	if err != nil {
		return err
	}
	defer ln.Close()
	hooks.Log("Forwarding from 127.0.0.1", false)
	hooks.Ready()
	<-ctx.Done()
	return nil
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func serviceState(m *ServiceManager, name string) model.Service {
	for _, s := range m.ListServiceStates() {
		if s.Name == name {
			return s
		}
	}
	return model.Service{}
}

func TestStartAndStopServiceReleasesPort(t *testing.T) {
	port := freePort(t)
	m := newTestManager(t, listeningForward, map[string]string{
		"db": fmt.Sprintf("kubectl -n prod port-forward svc/db %d:5432", port),
	})

	if err := m.StartService(context.Background(), "db"); err != nil {
		t.Fatalf("StartService: %v", err)
	}
	waitFor(t, "healthy", func() bool { return serviceState(m, "db").Status == model.StatusHealthy })

	svc := serviceState(m, "db")
	if svc.LocalPort != fmt.Sprint(port) || svc.MainPort != "5432" {
		t.Errorf("ports = %s/%s, want %d/5432", svc.LocalPort, svc.MainPort, port)
	}
	if portFree(port) {
		t.Fatal("port should be held while the service runs")
	}

	m.StopService("db")
	if !portFree(port) {
		t.Error("port still held right after StopService")
	}
	if len(m.ListServiceStates()) != 0 {
		t.Error("stopped service still listed")
	}
}

func TestForwardErrorMarksServiceAndRetries(t *testing.T) {
	attempts := make(chan struct{}, 10)
	failing := func(ctx context.Context, _ kube.Spec, _ kube.Options, _ kube.Hooks) error {
		attempts <- struct{}{}
		return errors.New("no running pod found for service/db")
	}
	m := newTestManager(t, failing, map[string]string{
		"db": "kubectl port-forward svc/db 15432:5432",
	})

	if err := m.StartService(context.Background(), "db"); err != nil {
		t.Fatalf("StartService: %v", err)
	}
	defer m.StopAllServices()

	waitFor(t, "error status", func() bool { return serviceState(m, "db").Status == model.StatusError })
	svc := serviceState(m, "db")
	if !strings.Contains(svc.LastError, "no running pod") {
		t.Errorf("LastError = %q", svc.LastError)
	}
	waitFor(t, "reconnect log", func() bool {
		for _, l := range serviceState(m, "db").Logs {
			if strings.Contains(l.Message, "RECONNECTING") {
				return true
			}
		}
		return false
	})
}

func TestStopAllServicesBoundedByGrace(t *testing.T) {
	orig := shutdownGraceTimeout
	shutdownGraceTimeout = 150 * time.Millisecond
	defer func() { shutdownGraceTimeout = orig }()

	// A tunnel stuck mid-dial ignores cancellation for a while.
	release := make(chan struct{})
	defer close(release)
	stuck := func(ctx context.Context, _ kube.Spec, _ kube.Options, _ kube.Hooks) error {
		<-release
		return nil
	}

	services := map[string]string{}
	for i := 0; i < 5; i++ {
		services[fmt.Sprintf("svc%d", i)] = fmt.Sprintf("kubectl port-forward svc/s%d %d:80", i, 20000+i)
	}
	m := newTestManager(t, stuck, services)
	for name := range services {
		if err := m.StartService(context.Background(), name); err != nil {
			t.Fatal(err)
		}
	}

	start := time.Now()
	m.StopAllServices()
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("StopAllServices took %v; services must be stopped concurrently and the wait capped", elapsed)
	}
	if len(m.ListServiceStates()) != 0 {
		t.Error("services still listed after StopAllServices")
	}
}

func TestRestartServiceRebindsPort(t *testing.T) {
	port := freePort(t)
	m := newTestManager(t, listeningForward, map[string]string{
		"db": fmt.Sprintf("kubectl port-forward svc/db %d:5432", port),
	})
	if err := m.StartService(context.Background(), "db"); err != nil {
		t.Fatal(err)
	}
	defer m.StopAllServices()
	waitFor(t, "healthy", func() bool { return serviceState(m, "db").Status == model.StatusHealthy })

	m.restartInPlace(context.Background(), "db")
	waitFor(t, "healthy after restart", func() bool { return serviceState(m, "db").Status == model.StatusHealthy })
	if svc := serviceState(m, "db"); svc.LastError != "" {
		t.Errorf("restart left error %q (port not released before rebinding?)", svc.LastError)
	}
}

func TestStartServiceRejectsUnsupportedCommand(t *testing.T) {
	m := newTestManager(t, listeningForward, map[string]string{
		"tunnel": "ssh -L 8080:localhost:80 user@host",
	})
	if err := m.StartService(context.Background(), "tunnel"); err == nil {
		t.Fatal("expected an error for a non-kubectl command")
	}
}

func TestForwardOptionsWithoutCertManager(t *testing.T) {
	m := &ServiceManager{}
	if got := m.forwardOptions(); got != (kube.Options{}) {
		t.Errorf("forwardOptions() = %+v, want empty", got)
	}
}

func TestClearLogsRespectsScope(t *testing.T) {
	first := &runningService{name: "first", logs: []model.LogEntry{{Message: "one"}}}
	second := &runningService{name: "second", logs: []model.LogEntry{{Message: "two"}}}
	mgr := &ServiceManager{services: map[string]*runningService{
		"first": first, "second": second,
	}}

	mgr.ClearLogs("first")
	if got := len(first.snapshot().Logs); got != 0 {
		t.Fatalf("selected service still has %d log(s)", got)
	}
	if got := len(second.snapshot().Logs); got != 1 {
		t.Fatalf("other service logs changed: got %d", got)
	}

	mgr.ClearLogs("")
	if got := len(second.snapshot().Logs); got != 0 {
		t.Fatalf("clear all left %d log(s)", got)
	}
}

func TestNormalizeErrorLine(t *testing.T) {
	if got := normalizeErrorLine("simple error"); got != "simple error" {
		t.Errorf("got %q", got)
	}
	if got := normalizeErrorLine(strings.Repeat("x", 200)); len(got) > 150 {
		t.Errorf("expected truncated, got len %d", len(got))
	}
	if got := normalizeErrorLine("error:   too   many   spaces"); got != "error: too many spaces" {
		t.Errorf("got %q", got)
	}
}

func TestEnsureValidServiceName(t *testing.T) {
	for _, name := range []string{"db", "my-service", "svc_123", "A"} {
		if err := ensureValidServiceName(name); err != nil {
			t.Errorf("ensureValidServiceName(%q) = %v", name, err)
		}
	}
	for _, name := range []string{"", "a/b", "a\\b", "a:b", "a*b", "a..b", "a b", "a<b", strings.Repeat("a", 51)} {
		if err := ensureValidServiceName(name); err == nil {
			t.Errorf("ensureValidServiceName(%q) should fail", name)
		}
	}
}

func TestEnsureValidCommand(t *testing.T) {
	valid := []string{
		"kubectl port-forward svc/db 5432:5432",
		// Names that used to trip the old shell-command blacklist.
		"kubectl -n prod port-forward svc/asphalt-reboot-db 5432:5432",
	}
	for _, cmd := range valid {
		if err := ensureValidCommand(cmd); err != nil {
			t.Errorf("ensureValidCommand(%q) = %v", cmd, err)
		}
	}

	invalid := []string{
		"",
		"ssh -L 8080:localhost:80 user@host",
		"kubectl get pods",
		"kubectl port-forward svc/db",
		strings.Repeat("x", 1001),
	}
	for _, cmd := range invalid {
		if err := ensureValidCommand(cmd); err == nil {
			t.Errorf("ensureValidCommand(%q) should fail", cmd)
		}
	}
}

func TestRunningServiceSnapshotIncludesIconFields(t *testing.T) {
	svc := &runningService{
		name:        "web",
		command:     "kubectl port-forward svc/web 8080:80",
		localPort:   "8080",
		mainPort:    "80",
		iconEnabled: true,
		status:      model.StatusHealthy,
	}

	snapshot := svc.snapshot()
	if snapshot.LocalPort != "8080" {
		t.Fatalf("LocalPort = %q, want 8080", snapshot.LocalPort)
	}
	if snapshot.MainPort != "80" {
		t.Fatalf("MainPort = %q, want 80", snapshot.MainPort)
	}
	if !snapshot.IconEnabled {
		t.Fatal("IconEnabled should be true")
	}
}

func TestNextRestartCount(t *testing.T) {
	if got := nextRestartCount(0, false); got != 1 {
		t.Errorf("nextRestartCount(0,false) = %d, want 1", got)
	}
	if got := nextRestartCount(5, false); got != 6 {
		t.Errorf("nextRestartCount(5,false) = %d, want 6", got)
	}
	if got := nextRestartCount(9, true); got != 1 {
		t.Errorf("nextRestartCount(9,true) = %d, want 1", got)
	}
}
