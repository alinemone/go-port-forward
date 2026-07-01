package manager

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alinemone/go-port-forward/internal/model"
)

// TestMain lets the test binary double as a tiny "arg printer": when
// PF_ARGPRINT=1 it prints its arguments joined by "|" and exits. This is used by
// TestNewShellCommandPreservesQuotedSpacedPath to observe exactly what arguments
// a program receives after commandStr passes through the OS shell.
func TestMain(m *testing.M) {
	if os.Getenv("PF_ARGPRINT") == "1" {
		fmt.Println(strings.Join(os.Args[1:], "|"))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestNewShellCommandPreservesQuotedSpacedPath guards the fix for cert paths
// under usernames containing spaces (e.g. C:\Users\ali mohammadi\...). A quoted
// path in commandStr must reach the target program as one clean argument with no
// literal quote characters.
func TestNewShellCommandPreservesQuotedSpacedPath(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	prog := self
	if strings.Contains(prog, " ") {
		prog = `"` + prog + `"`
	}

	spacedPath := `C:\Users\ali mohammadi\.pf\certs\client-cert.pem`
	commandStr := fmt.Sprintf(`%s --client-certificate="%s" end`, prog, spacedPath)

	cmd := newShellCommand(commandStr)
	cmd.Env = append(os.Environ(), "PF_ARGPRINT=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run failed: %v (output: %q)", err, out)
	}

	got := strings.TrimRight(string(out), "\r\n")
	want := "--client-certificate=" + spacedPath + "|end"
	if got != want {
		t.Errorf("target received wrong args:\n got  %q\n want %q", got, want)
	}
}

// TestStopAllServicesDoesNotWait guards that bulk shutdown never blocks on a
// service's graceful-exit channel: it cancels + force-kills and returns. Here
// the services have no live process and a done channel that never closes, so a
// correct implementation must still return effectively immediately.
func TestStopAllServicesDoesNotWait(t *testing.T) {
	const n = 8
	m := &ServiceManager{services: make(map[string]*runningService)}
	cancelled := make([]bool, n)
	for i := 0; i < n; i++ {
		i := i
		m.services[fmt.Sprintf("svc%d", i)] = &runningService{
			name:   fmt.Sprintf("svc%d", i),
			cancel: func() { cancelled[i] = true },
			done:   make(chan struct{}), // never closed
			// process is nil -> killProcessTrees is a no-op
		}
	}

	svcs := make([]*runningService, 0, n)
	for _, s := range m.services {
		svcs = append(svcs, s)
	}

	start := time.Now()
	m.StopAllServices()
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("StopAllServices blocked for %v; it must not wait on done", elapsed)
	}
	for i := range cancelled {
		if !cancelled[i] {
			t.Errorf("service %d was not cancelled", i)
		}
	}
	for _, s := range svcs {
		if !s.bulkKill.Load() {
			t.Errorf("service %q not marked for bulk kill", s.name)
		}
	}
}

// TestKillProcessTreesTerminatesAll spawns several real shell processes and
// verifies the batched kill terminates every one of them.
func TestKillProcessTreesTerminatesAll(t *testing.T) {
	sleepCmd := "sleep 60"
	if runtime.GOOS == "windows" {
		sleepCmd = "ping -n 60 127.0.0.1 >NUL"
	}

	const n = 3
	cmds := make([]*exec.Cmd, n)
	procs := make([]*os.Process, n)
	for i := 0; i < n; i++ {
		c := newShellCommand(sleepCmd)
		if err := c.Start(); err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
		cmds[i] = c
		procs[i] = c.Process
	}

	killProcessTrees(procs)

	for i, c := range cmds {
		waited := make(chan struct{})
		go func(c *exec.Cmd) { c.Wait(); close(waited) }(c)
		select {
		case <-waited:
		case <-time.After(5 * time.Second):
			c.Process.Kill()
			t.Errorf("process %d was not terminated by killProcessTrees", i)
		}
	}
}

func TestClassifyOutputLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		isError bool
		want    lineKind
	}{
		{"healthy forwarding", "Forwarding from 127.0.0.1:8080 -> 80", false, lineKindHealthy},
		{"healthy handling", "Handling connection for 8080", false, lineKindHealthy},
		{"healthy on stderr", "Forwarding from 127.0.0.1:8080 -> 80", true, lineKindHealthy},
		{"normal stdout", "some normal output", false, lineKindInfo},
		{"fatal error", "error: unable to forward port", true, lineKindFatalError},
		{"transient error", "an existing connection was forcibly closed by the remote host", true, lineKindTransientError},
		{"connection reset", "connection reset by peer", true, lineKindTransientError},
		{"broken pipe", "broken pipe", true, lineKindTransientError},
		{"stderr info", "I0101 some info log", true, lineKindInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyOutputLine(tt.line, tt.isError)
			if got != tt.want {
				t.Errorf("classifyOutputLine(%q, %v) = %v, want %v", tt.line, tt.isError, got, tt.want)
			}
		})
	}
}

func TestIndicatesHealthyPortForward(t *testing.T) {
	if !indicatesHealthyPortForward("Forwarding from 127.0.0.1:8080 -> 80") {
		t.Error("should detect forwarding")
	}
	if !indicatesHealthyPortForward("Handling connection for 8080") {
		t.Error("should detect handling")
	}
	if indicatesHealthyPortForward("some random line") {
		t.Error("should not detect random line")
	}
}

func TestLooksLikeError(t *testing.T) {
	errorLines := []string{
		"Error: something went wrong",
		"failed to connect",
		"unable to forward port",
		"cannot resolve host",
		"access denied",
		"connection refused",
		"service not found",
		"lost connection to pod",
	}
	for _, line := range errorLines {
		if !looksLikeError(line) {
			t.Errorf("looksLikeError(%q) = false, want true", line)
		}
	}

	nonErrorLines := []string{
		"Forwarding from 127.0.0.1:8080",
		"Handling connection for 8080",
		"I0101 some info log",
		"pod is running",
	}
	for _, line := range nonErrorLines {
		if looksLikeError(line) {
			t.Errorf("looksLikeError(%q) = true, want false", line)
		}
	}
}

func TestIsTransientPortForwardError(t *testing.T) {
	transient := []string{
		"an existing connection was forcibly closed by the remote host",
		"connection reset by peer",
		"broken pipe",
		"use of closed network connection",
		"E0101 unhandled error: error copying from remote stream to local connection",
		"E0101 unhandled error: error copying from local connection to remote stream",
	}
	for _, line := range transient {
		if !isTransientPortForwardError(line) {
			t.Errorf("isTransientPortForwardError(%q) = false, want true", line)
		}
	}

	nonTransient := []string{
		"error: unable to forward port",
		"failed to connect",
		"some random error",
	}
	for _, line := range nonTransient {
		if isTransientPortForwardError(line) {
			t.Errorf("isTransientPortForwardError(%q) = true, want false", line)
		}
	}
}

func TestNormalizeErrorLine(t *testing.T) {
	// Short line stays the same
	got := normalizeErrorLine("simple error")
	if got != "simple error" {
		t.Errorf("got %q", got)
	}

	// Long line gets truncated
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'x'
	}
	got = normalizeErrorLine(string(long))
	if len(got) > 150 {
		t.Errorf("expected truncated, got len %d", len(got))
	}

	// Multiple spaces get normalized
	got = normalizeErrorLine("error:   too   many   spaces")
	if got != "error: too many spaces" {
		t.Errorf("got %q", got)
	}
}

func TestEnsureValidServiceName(t *testing.T) {
	valid := []string{"db", "my-service", "svc_123", "A"}
	for _, name := range valid {
		if err := ensureValidServiceName(name); err != nil {
			t.Errorf("ensureValidServiceName(%q) = %v", name, err)
		}
	}

	invalid := []string{
		"",
		"a/b",
		"a\\b",
		"a:b",
		"a*b",
		"a..b",
		"a b",
		"a<b",
	}
	for _, name := range invalid {
		if err := ensureValidServiceName(name); err == nil {
			t.Errorf("ensureValidServiceName(%q) should fail", name)
		}
	}

	// Too long
	longName := make([]byte, 51)
	for i := range longName {
		longName[i] = 'a'
	}
	if err := ensureValidServiceName(string(longName)); err == nil {
		t.Error("should reject name > 50 chars")
	}
}

func TestEnsureValidCommand(t *testing.T) {
	valid := []string{
		"kubectl port-forward svc/db 5432:5432",
		"ssh -L 8080:localhost:80 user@host",
	}
	for _, cmd := range valid {
		if err := ensureValidCommand(cmd); err != nil {
			t.Errorf("ensureValidCommand(%q) = %v", cmd, err)
		}
	}

	invalid := []string{
		"",
		"rm -rf /",
		"dd if=/dev/zero",
		"shutdown now",
		"reboot",
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

func TestAddKubectlCertFlags(t *testing.T) {
	cmd := "kubectl port-forward svc/db 5432:5432"
	result := addKubectlCertFlags(cmd, "/tmp/cert.pem", "/tmp/key.pem")

	if result == cmd {
		t.Error("expected flags to be added")
	}
	if !contains(result, `--client-certificate="/tmp/cert.pem"`) {
		t.Errorf("missing cert flag in %q", result)
	}
	if !contains(result, `--client-key="/tmp/key.pem"`) {
		t.Errorf("missing key flag in %q", result)
	}

	// Already has flags — should not add again
	cmdWithFlags := "kubectl --client-certificate=x port-forward svc/db 5432:5432"
	result2 := addKubectlCertFlags(cmdWithFlags, "/tmp/cert.pem", "/tmp/key.pem")
	if result2 != cmdWithFlags {
		t.Errorf("should not modify command that already has cert flags")
	}

	// No kubectl in command
	noKubectl := "ssh -L 8080:localhost:80 user@host"
	result3 := addKubectlCertFlags(noKubectl, "/tmp/cert.pem", "/tmp/key.pem")
	if result3 != noKubectl {
		t.Errorf("should not modify non-kubectl command")
	}

	// Multiple kubectl invocations in one command
	multi := "kubectl config use-context production && kubectl -n prod port-forward svc/db 5432:5432"
	result4 := addKubectlCertFlags(multi, "/tmp/cert.pem", "/tmp/key.pem")
	if strings.Count(result4, `--client-certificate="/tmp/cert.pem"`) != 2 {
		t.Errorf("expected cert flag on both kubectl commands, got %q", result4)
	}
	if strings.Count(result4, `--client-key="/tmp/key.pem"`) != 2 {
		t.Errorf("expected key flag on both kubectl commands, got %q", result4)
	}

	// Paths containing spaces (e.g. C:\Users\ali mohammadi\...) must be quoted
	spaced := addKubectlCertFlags(cmd, `C:\Users\ali mohammadi\cert.pem`, `C:\Users\ali mohammadi\key.pem`)
	if !contains(spaced, `--client-certificate="C:\Users\ali mohammadi\cert.pem"`) {
		t.Errorf("cert path with spaces not quoted in %q", spaced)
	}
	if !contains(spaced, `--client-key="C:\Users\ali mohammadi\key.pem"`) {
		t.Errorf("key path with spaces not quoted in %q", spaced)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
