package manager

import (
	"net"
	"os"
	"sync"
	"time"
)

// shutdownGraceTimeout is how long a cancelled service is given to exit on its
// own before its process tree is force-killed. portReleaseTimeout bounds how
// long we wait for the local port to actually free up after a kill. Both are
// vars so tests can shrink them.
var (
	shutdownGraceTimeout = 5 * time.Second
	portReleaseTimeout   = 5 * time.Second
)

// currentProcess returns the service's live OS process under the lock, or nil if
// it isn't currently running.
func (s *runningService) currentProcess() *os.Process {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.process
}

// isPortFree reports whether nothing is listening on the given local port. An
// empty port is treated as free (nothing to release).
func isPortFree(port string) bool {
	if port == "" {
		return true
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// waitForPortRelease blocks until the port is free or the timeout elapses.
func waitForPortRelease(port string, timeout time.Duration) {
	if port == "" {
		return
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isPortFree(port) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// ensurePortFree is the reliable fallback that guarantees a service's local port
// is actually released. It is safe: it only kills whatever is listening after
// confirming the port is still held (which, right after a tree kill, means our
// own leaked forwarder rather than an unrelated process). A free or empty port
// is a no-op.
func ensurePortFree(port string) {
	if port == "" || isPortFree(port) {
		return
	}
	killListenersOnPort(port)
	waitForPortRelease(port, portReleaseTimeout)
}

// teardownService is THE single, reliable path to stop one service. It cancels
// the run loop, force-kills the process tree if the loop doesn't exit within the
// grace period, then guarantees the local port is released even when the tree
// kill missed a descendant (e.g. taskkill /T failing to reap kubectl.exe). It is
// idempotent and nil-safe.
func teardownService(svc *runningService) {
	if svc == nil {
		return
	}

	if svc.cancel != nil {
		svc.cancel()
	}

	if svc.done != nil {
		select {
		case <-svc.done:
		case <-time.After(shutdownGraceTimeout):
			killProcessTree(svc.currentProcess())
		}
	}

	ensurePortFree(svc.localPort)
}

// teardownServices tears down many services concurrently — one goroutine each —
// so bulk shutdown is bounded by the slowest single teardown, not their sum.
func teardownServices(svcs []*runningService) {
	var wg sync.WaitGroup
	wg.Add(len(svcs))
	for _, svc := range svcs {
		go func(s *runningService) {
			defer wg.Done()
			teardownService(s)
		}(svc)
	}
	wg.Wait()
}

// ensurePortsFree runs ensurePortFree for many ports concurrently. Used by the
// bulk-shutdown fast path, where process trees are already killed in one batched
// call and only port verification/fallback remains.
func ensurePortsFree(ports []string) {
	var wg sync.WaitGroup
	wg.Add(len(ports))
	for _, port := range ports {
		go func(p string) {
			defer wg.Done()
			ensurePortFree(p)
		}(port)
	}
	wg.Wait()
}
