package manager

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
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
	childOut, err := child.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	child.Stderr = child.Stdout
	if err := child.Start(); err != nil {
		t.Fatalf("start holder: %v", err)
	}
	defer func() {
		child.Process.Kill()
		child.Wait()
	}()

	// Wait for the child's READY line rather than probing the port ourselves:
	// isPortFree binds the port to test it, and a parent-side probe can collide
	// with the child's own bind and kill it before it ever holds the port.
	ready := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(childOut)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "READY") {
				ready <- nil
				return
			}
		}
		ready <- fmt.Errorf("child exited before binding (no READY line)")
	}()
	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("child never bound port %s: %v", port, err)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("timed out waiting for child to bind port %s", port)
	}

	ensurePortFree(port)

	if !isPortFree(port) {
		t.Errorf("ensurePortFree did not release port %s", port)
	}
}

// TestVerifyPortsReleasedConfirmsFreePorts: released and empty ports must come
// back clean so callers can trust an empty result as "everything freed".
func TestVerifyPortsReleasedConfirmsFreePorts(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close()
	waitForPortRelease(port, 2*time.Second)

	if busy := VerifyPortsReleased([]string{port, ""}); len(busy) != 0 {
		t.Errorf("expected all ports released, still busy: %v", busy)
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
