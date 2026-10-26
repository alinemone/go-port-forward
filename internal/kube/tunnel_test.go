package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/tools/portforward"
)

// fakeStream is one half-closable stream: Close ends our writes (the remote
// sees EOF) while reads continue, like a SPDY stream.
type fakeStream struct {
	headers http.Header
	r       *io.PipeReader // remote → us
	w       *io.PipeWriter // us → remote
}

func (s *fakeStream) Read(p []byte) (int, error)  { return s.r.Read(p) }
func (s *fakeStream) Write(p []byte) (int, error) { return s.w.Write(p) }
func (s *fakeStream) Close() error                { return s.w.Close() }
func (s *fakeStream) Reset() error                { s.r.Close(); return s.w.Close() }
func (s *fakeStream) Headers() http.Header        { return s.headers }
func (s *fakeStream) Identifier() uint32          { return 0 }

// fakePod is a port-forward connection to a pod that echoes data in upper case.
// Port failPort reports a kubelet error on the error stream instead.
type fakePod struct {
	failPort string
	closed   chan bool
	once     sync.Once

	mu      sync.Mutex
	pending map[string]chan string // requestID → kubelet error message
}

func newFakePod(failPort string) *fakePod {
	return &fakePod{failPort: failPort, closed: make(chan bool), pending: map[string]chan string{}}
}

func (p *fakePod) errChan(id string) chan string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending[id] == nil {
		p.pending[id] = make(chan string, 1)
	}
	return p.pending[id]
}

func (p *fakePod) CreateStream(headers http.Header) (httpstream.Stream, error) {
	time.Sleep(2 * time.Millisecond) // a measurable round trip
	inR, inW := io.Pipe()            // remote → us
	outR, outW := io.Pipe()          // us → remote
	stream := &fakeStream{headers: headers.Clone(), r: inR, w: outW}
	id := headers.Get(corev1.PortForwardRequestIDHeader)
	port := headers.Get(corev1.PortHeader)

	if headers.Get(corev1.StreamType) == corev1.StreamTypeError {
		go func() {
			io.Copy(io.Discard, outR)
			inW.Write([]byte(<-p.errChan(id)))
			inW.Close()
		}()
		return stream, nil
	}

	go func() {
		if port == p.failPort {
			p.errChan(id) <- "failed to connect to localhost:" + port + " inside namespace: connection refused"
			inW.Close()
			io.Copy(io.Discard, outR)
			return
		}
		buf := make([]byte, 1024)
		for {
			n, err := outR.Read(buf)
			if n > 0 {
				inW.Write([]byte(strings.ToUpper(string(buf[:n]))))
			}
			if err != nil {
				break
			}
		}
		inW.Close()
		p.errChan(id) <- ""
	}()
	return stream, nil
}

func (p *fakePod) Close() error {
	p.once.Do(func() { close(p.closed) })
	return nil
}
func (p *fakePod) CloseChan() <-chan bool             { return p.closed }
func (p *fakePod) SetIdleTimeout(time.Duration)       {}
func (p *fakePod) RemoveStreams(...httpstream.Stream) {}
func (p *fakePod) logs(t *testing.T) func(string, bool) {
	return func(m string, e bool) { t.Logf("[err=%v] %s", e, m) }
}

func startTunnel(t *testing.T, pod *fakePod, ports []forwardedPort) (*Stats, context.CancelFunc, chan error) {
	t.Helper()
	stats := &Stats{}
	tun := &tunnel{conn: pod, stats: stats, log: pod.logs(t)}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- tun.serve(ctx, []string{"localhost"}, ports, func() { close(ready) }) }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("serve ended before ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("tunnel never became ready")
	}
	return stats, cancel, done
}

func roundTrip(t *testing.T, port int, msg string) string {
	t.Helper()
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte(msg))
	conn.(*net.TCPConn).CloseWrite()
	reply, _ := io.ReadAll(conn)
	return string(reply)
}

func TestTunnelForwardsAndCounts(t *testing.T) {
	port := freeTestPort(t)
	stats, cancel, done := startTunnel(t, newFakePod(""), []forwardedPort{{Local: port, Remote: 5432}})

	if got := roundTrip(t, port, "hello"); got != "HELLO" {
		t.Fatalf("reply = %q, want HELLO", got)
	}
	if got := roundTrip(t, port, "abc"); got != "ABC" {
		t.Fatalf("reply = %q, want ABC", got)
	}

	waitUntil(t, func() bool { return stats.ActiveConns.Load() == 0 })
	if got := stats.TotalConns.Load(); got != 2 {
		t.Errorf("TotalConns = %d, want 2", got)
	}
	if in, out := stats.BytesIn.Load(), stats.BytesOut.Load(); in != 8 || out != 8 {
		t.Errorf("bytes in/out = %d/%d, want 8/8", in, out)
	}
	if stats.Latency() <= 0 {
		t.Error("latency not measured")
	}
	if stats.LastActivity().IsZero() {
		t.Error("last activity not recorded")
	}

	cancel()
	if err := <-done; err != nil {
		t.Errorf("serve returned %v after cancel, want nil", err)
	}
	if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port)); err != nil {
		t.Errorf("port still held after stop: %v", err)
	} else {
		ln.Close()
	}
}

func TestTunnelKubeletErrorBreaksTunnel(t *testing.T) {
	port := freeTestPort(t)
	_, cancel, done := startTunnel(t, newFakePod("9999"), []forwardedPort{{Local: port, Remote: 9999}})
	defer cancel()

	roundTrip(t, port, "x")
	select {
	case err := <-done:
		if !errors.Is(err, portforward.ErrLostConnectionToPod) {
			t.Errorf("serve returned %v, want ErrLostConnectionToPod", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a kubelet error did not break the tunnel")
	}
}

func TestTunnelPortInUse(t *testing.T) {
	held, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	port := held.Addr().(*net.TCPAddr).Port

	// Explicit address: the busy port is an error.
	tun := &tunnel{conn: newFakePod(""), stats: &Stats{}, log: func(string, bool) {}}
	err = tun.serve(context.Background(), []string{"127.0.0.1"}, []forwardedPort{{Local: port, Remote: 1}}, nil)
	if err == nil || !strings.Contains(err.Error(), "unable to listen") {
		t.Errorf("serve error = %v, want unable to listen", err)
	}
}

func freeTestPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
