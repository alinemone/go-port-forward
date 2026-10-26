package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/tools/portforward"
)

// Stats are live counters for one service's tunnels. The manager keeps a single
// Stats per service, so totals survive reconnects while ActiveConns naturally
// drops back to zero when a tunnel goes away.
type Stats struct {
	ActiveConns atomic.Int64
	TotalConns  atomic.Int64
	BytesIn     atomic.Int64 // pod → local client
	BytesOut    atomic.Int64 // local client → pod

	lastActivity atomic.Int64 // unix nanos
	latency      atomic.Int64 // nanos to open the last data stream
}

// LastActivity is when data last moved through any connection (zero if never).
func (s *Stats) LastActivity() time.Time {
	if n := s.lastActivity.Load(); n != 0 {
		return time.Unix(0, n)
	}
	return time.Time{}
}

// Latency is how long the last connection took to open its data stream. Opening
// a stream is a full round trip to the kubelet (the API server replies only once
// the kubelet accepts it), so this tracks the tunnel's real latency. Zero until
// the first connection.
func (s *Stats) Latency() time.Duration {
	return time.Duration(s.latency.Load())
}

func (s *Stats) touch() {
	s.lastActivity.Store(time.Now().UnixNano())
}

// forwardedPort is one resolved LOCAL → container port pair.
type forwardedPort struct {
	Local  int
	Remote int
}

// tunnel serves local listeners over one port-forward connection. It is the
// connection handling of client-go's tools/portforward.PortForwarder, kept close
// to the original, with per-connection counters added — PortForwarder exposes
// no hooks to observe connections or traffic.
type tunnel struct {
	conn  httpstream.Connection
	stats *Stats
	log   func(string, bool)

	requestID atomic.Int64
}

// serve listens on every address/port pair, signals ready once at least one
// port is listening, and forwards connections until ctx is cancelled (nil) or
// the connection to the pod closes (ErrLostConnectionToPod).
func (t *tunnel) serve(ctx context.Context, addresses []string, ports []forwardedPort, ready func()) error {
	addrs, err := parseListenAddresses(addresses)
	if err != nil {
		return err
	}

	var (
		listeners []net.Listener
		wg        sync.WaitGroup
	)
	defer func() {
		for _, ln := range listeners {
			ln.Close()
		}
		wg.Wait()
	}()

	listening := false
	for _, port := range ports {
		lns, err := listenOnPort(addrs, port)
		if err != nil {
			t.log(fmt.Sprintf("Unable to listen on port %d: %v", port.Local, err), true)
			continue
		}
		listening = true
		for _, ln := range lns {
			listeners = append(listeners, ln)
			t.log(fmt.Sprintf("Forwarding from %s -> %d", ln.Addr(), port.Remote), false)
			wg.Add(1)
			go func() {
				defer wg.Done()
				t.accept(ln, port)
			}()
		}
	}
	if !listening {
		return fmt.Errorf("unable to listen on any of the requested ports (another program may hold them — try 'pf cleanup')")
	}

	if ready != nil {
		ready()
	}

	select {
	case <-ctx.Done():
		return nil
	case <-t.conn.CloseChan():
		return portforward.ErrLostConnectionToPod
	}
}

func (t *tunnel) accept(ln net.Listener, port forwardedPort) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if !isClosedNetworkError(err) {
				t.log(fmt.Sprintf("Error accepting connection on port %d: %v", port.Local, err), true)
			}
			return
		}
		go t.handle(conn, port)
	}
}

// handle copies data between one local connection and a pair of streams to the
// pod, exactly as client-go does: an error stream the kubelet reports failures
// on, and a data stream.
func (t *tunnel) handle(conn net.Conn, port forwardedPort) {
	defer conn.Close()

	t.stats.TotalConns.Add(1)
	t.stats.ActiveConns.Add(1)
	defer t.stats.ActiveConns.Add(-1)
	t.stats.touch()

	requestID := t.requestID.Add(1) - 1

	headers := http.Header{}
	headers.Set(corev1.StreamType, corev1.StreamTypeError)
	headers.Set(corev1.PortHeader, strconv.Itoa(port.Remote))
	headers.Set(corev1.PortForwardRequestIDHeader, strconv.FormatInt(requestID, 10))
	errorStream, err := t.conn.CreateStream(headers)
	if err != nil {
		t.log(fmt.Sprintf("Error creating error stream for port %d -> %d: %v", port.Local, port.Remote, err), true)
		return
	}
	// we're not writing to this stream
	errorStream.Close()
	defer t.conn.RemoveStreams(errorStream)

	type readAllResult struct {
		message []byte
		err     error
	}
	errorChan := make(chan readAllResult, 1)
	go func() {
		message, err := io.ReadAll(errorStream)
		errorChan <- readAllResult{message: message, err: err}
	}()

	headers.Set(corev1.StreamType, corev1.StreamTypeData)
	opened := time.Now()
	dataStream, err := t.conn.CreateStream(headers)
	if err != nil {
		t.log(fmt.Sprintf("Error creating forwarding stream for port %d -> %d: %v", port.Local, port.Remote, err), true)
		return
	}
	t.stats.latency.Store(int64(time.Since(opened)))
	defer t.conn.RemoveStreams(dataStream)

	localError := make(chan struct{})
	remoteDone := make(chan struct{})

	go func() {
		// Copy from the remote side to the local port.
		dst := &countingWriter{w: conn, n: &t.stats.BytesIn, stats: t.stats}
		if _, err := io.Copy(dst, dataStream); err != nil && !isClosedNetworkError(err) {
			t.log(fmt.Sprintf("Error copying from remote stream to local connection: %v", err), true)
		}
		// inform the select below that the remote copy is done
		close(remoteDone)
	}()

	go func() {
		// inform server we're not sending any more data after copy unblocks
		defer dataStream.Close()

		// Copy from the local port to the remote side.
		dst := &countingWriter{w: dataStream, n: &t.stats.BytesOut, stats: t.stats}
		if _, err := io.Copy(dst, conn); err != nil && !isClosedNetworkError(err) {
			t.log(fmt.Sprintf("Error copying from local connection to remote stream: %v", err), true)
			// break out of the select below without waiting for the other copy to finish
			close(localError)
		}
	}()

	// wait for either a local->remote error or for copying from remote->local to finish
	select {
	case <-remoteDone:
	case <-localError:
	}

	// Reset dataStream to discard any unsent data before waiting on the error
	// stream; otherwise blocked data can keep errorChan from ever delivering.
	_ = dataStream.Reset()

	// always expect something on errorChan (it may be empty)
	result := <-errorChan
	switch {
	case result.err != nil:
		t.log(fmt.Sprintf("Error reading from error stream for port %d -> %d: %v", port.Local, port.Remote, result.err), true)
		t.conn.Close()
	case len(result.message) > 0:
		t.log(fmt.Sprintf("An error occurred forwarding %d -> %d: %s", port.Local, port.Remote, result.message), true)
		t.conn.Close()
	}
}

type countingWriter struct {
	w     io.Writer
	n     *atomic.Int64
	stats *Stats
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if n > 0 {
		c.n.Add(int64(n))
		c.stats.touch()
	}
	return n, err
}

type listenAddress struct {
	address  string
	protocol string
	// required addresses must succeed; "localhost" expands to 127.0.0.1 and ::1,
	// of which only one needs to (IPv6 may be disabled).
	required bool
}

func parseListenAddresses(addresses []string) ([]listenAddress, error) {
	parsed := make(map[string]listenAddress)
	for _, address := range addresses {
		if address == "localhost" {
			parsed["127.0.0.1"] = listenAddress{address: "127.0.0.1", protocol: "tcp4"}
			parsed["::1"] = listenAddress{address: "::1", protocol: "tcp6"}
			continue
		}
		ip := net.ParseIP(address)
		switch {
		case ip == nil:
			return nil, fmt.Errorf("%s is not a valid IP", address)
		case ip.To4() != nil:
			parsed[address] = listenAddress{address: address, protocol: "tcp4", required: true}
		default:
			parsed[address] = listenAddress{address: address, protocol: "tcp6", required: true}
		}
	}

	out := make([]listenAddress, 0, len(parsed))
	for _, a := range parsed {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].address < out[j].address })
	return out, nil
}

// listenOnPort opens the port on every address, with client-go's failure rules:
// any explicitly requested address failing is an error, and the localhost pair
// fails only if both of its addresses do.
func listenOnPort(addrs []listenAddress, port forwardedPort) ([]net.Listener, error) {
	var (
		listeners      []net.Listener
		errs           []error
		optionalTried  int
		optionalFailed int
	)
	for _, addr := range addrs {
		ln, err := net.Listen(addr.protocol, net.JoinHostPort(addr.address, strconv.Itoa(port.Local)))
		if !addr.required {
			optionalTried++
		}
		if err != nil {
			errs = append(errs, err)
			if addr.required {
				for _, l := range listeners {
					l.Close()
				}
				return nil, errors.Join(errs...)
			}
			optionalFailed++
			continue
		}
		listeners = append(listeners, ln)
	}
	if optionalTried > 0 && optionalFailed == optionalTried {
		for _, l := range listeners {
			l.Close()
		}
		return nil, errors.Join(errs...)
	}
	return listeners, nil
}

func isClosedNetworkError(err error) bool {
	return errors.Is(err, net.ErrClosed) ||
		strings.Contains(strings.ToLower(err.Error()), "use of closed network connection")
}
