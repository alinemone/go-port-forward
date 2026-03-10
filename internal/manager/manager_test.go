package manager

import (
	"strings"
	"testing"
)

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

func TestAddKubectlCertFlags(t *testing.T) {
	cmd := "kubectl port-forward svc/db 5432:5432"
	result := addKubectlCertFlags(cmd, "/tmp/cert.pem", "/tmp/key.pem")

	if result == cmd {
		t.Error("expected flags to be added")
	}
	if !contains(result, "--client-certificate=/tmp/cert.pem") {
		t.Errorf("missing cert flag in %q", result)
	}
	if !contains(result, "--client-key=/tmp/key.pem") {
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
	if strings.Count(result4, "--client-certificate=/tmp/cert.pem") != 2 {
		t.Errorf("expected cert flag on both kubectl commands, got %q", result4)
	}
	if strings.Count(result4, "--client-key=/tmp/key.pem") != 2 {
		t.Errorf("expected key flag on both kubectl commands, got %q", result4)
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
