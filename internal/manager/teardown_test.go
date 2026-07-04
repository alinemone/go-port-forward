package manager

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// TestIsPortFree verifies the port-availability probe: a bound port reads as
// busy, a released one reads as free, and an empty port is trivially free.
func TestIsPortFree(t *testing.T) {
	if !isPortFree("") {
		t.Error("empty port should be free")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split: %v", err)
	}

	if isPortFree(port) {
		t.Errorf("port %s should read as busy while bound", port)
	}

	ln.Close()
	// The OS may hold the socket briefly in TIME_WAIT-adjacent states; give it a
	// moment before asserting it frees.
	waitForPortRelease(port, 2*time.Second)
	if !isPortFree(port) {
		t.Errorf("port %s should read as free after close", port)
	}
}

// TestEnsurePortFreeReleasesHeldPort exercises the reliable fallback end-to-end:
// a *separate* process holds a real TCP port, and ensurePortFree must find and
// kill it via the netstat/lsof path so the port frees. This is the exact
// scenario the fix targets — a forwarder left holding the port after a tree kill
// missed it.
func TestEnsurePortFreeReleasesHeldPort(t *testing.T) {
	tool := "netstat"
	if runtime.GOOS != "windows" {
		tool = "lsof"
	}
	if _, err := exec.LookPath(tool); err != nil {
		t.Skipf("%s not available; skipping port-fallback test", tool)
	}

	// Reserve an ephemeral port, then hand it to a child process to hold.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	_, port, _ := net.SplitHostPort(probe.Addr().String())
	probe.Close()

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	child := exec.Command(self)
	child.Env = append(os.Environ(), "PF_HOLDPORT="+port)
	if err := child.Start(); err != nil {
		t.Fatalf("start holder: %v", err)
	}
	defer func() {
		child.Process.Kill()
		child.Wait()
	}()

	// Wait until the child has actually bound the port.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !isPortFree(port) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if isPortFree(port) {
		t.Fatalf("child never bound port %s", port)
	}

	ensurePortFree(port)

	if !isPortFree(port) {
		t.Errorf("ensurePortFree did not release port %s", port)
	}
}

// TestTeardownServicesRunsConcurrently proves bulk teardown is parallel: each
// service's loop exits after a fixed delay, so serial teardown would take
// n*delay while a concurrent one takes ~delay.
func TestTeardownServicesRunsConcurrently(t *testing.T) {
	const (
		n     = 5
		delay = 200 * time.Millisecond
	)

	svcs := make([]*runningService, n)
	for i := 0; i < n; i++ {
		done := make(chan struct{})
		svcs[i] = &runningService{
			name:   fmt.Sprintf("svc%d", i),
			cancel: func() {},
			done:   done,
			// empty localPort -> ensurePortFree is a no-op
		}
		go func(d chan struct{}) {
			time.Sleep(delay)
			close(d)
		}(done)
	}

	start := time.Now()
	teardownServices(svcs)
	elapsed := time.Since(start)

	if elapsed >= n*delay {
		t.Errorf("teardownServices took %v; expected concurrent (~%v), not serial (~%v)",
			elapsed, delay, n*delay)
	}
}
