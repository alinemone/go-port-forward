package manager

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/alinemone/go-port-forward/internal/storage"
)

// reserveEphemeralPort asks the OS for a free port and releases it so a child
// can bind it moments later.
func reserveEphemeralPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close()
	waitForPortRelease(port, 2*time.Second)
	return port
}

// holdPortCommand builds a shell command (as stored in services.json) that runs
// this test binary as a port holder via the PF_HOLDPORT hook in TestMain. The
// trailing "port:port" token exists so ParsePortsFromCommand sees a mapping,
// exactly like a real `kubectl port-forward ... L:R` command.
func holdPortCommand(t *testing.T, port string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`set PF_HOLDPORT=%s&& "%s" %s:%s`, port, self, port, port)
	}
	return fmt.Sprintf(`PF_HOLDPORT=%s "%s" %s:%s`, port, self, port, port)
}

// TestStopAllServicesReleasesRealPorts is the end-to-end guarantee behind
// quitting the TUI (q / esc / ctrl+c): real services that bind real local TCP
// ports are started through the full StartService path (storage → shell →
// process), and StopAllServices must leave every one of those ports free.
func TestStopAllServicesReleasesRealPorts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	st := storage.NewStorage()
	ports := []string{reserveEphemeralPort(t), reserveEphemeralPort(t)}
	names := make([]string, len(ports))
	for i, port := range ports {
		names[i] = fmt.Sprintf("hold%d", i)
		if err := st.AddService(names[i], holdPortCommand(t, port)); err != nil {
			t.Fatalf("add service: %v", err)
		}
	}

	mgr := NewServiceManager(st)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, name := range names {
		if err := mgr.StartService(ctx, name); err != nil {
			t.Fatalf("start %s: %v", name, err)
		}
	}

	// Wait until every holder has actually bound its port.
	deadline := time.Now().Add(15 * time.Second)
	for _, port := range ports {
		for time.Now().Before(deadline) && isPortFree(port) {
			time.Sleep(50 * time.Millisecond)
		}
		if isPortFree(port) {
			for _, state := range mgr.ListServiceStates() {
				for _, entry := range state.Logs {
					t.Logf("[%s] %s", state.Name, entry.Message)
				}
				t.Logf("[%s] status=%s lastErr=%s", state.Name, state.Status, state.LastError)
			}
			t.Fatalf("service never bound port %s", port)
		}
	}

	mgr.StopAllServices()

	for _, port := range ports {
		waitForPortRelease(port, 5*time.Second)
		if !isPortFree(port) {
			t.Errorf("port %s still held after StopAllServices", port)
		}
	}
	if busy := VerifyPortsReleased(ports); len(busy) != 0 {
		t.Errorf("VerifyPortsReleased reports busy ports after shutdown: %v", busy)
	}
}
