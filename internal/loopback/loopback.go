// Package loopback hands out dedicated loopback IPs so several forwards that
// share a LOCAL port can run side by side, each on its own 127.x address (and
// reachable through its own hosts-file hostname).
//
// Windows and Linux route the whole 127.0.0.0/8 block to the loopback interface,
// so any address in it can be bound with no setup. macOS only configures
// 127.0.0.1 on lo0; there every extra address needs an `ifconfig lo0 alias`
// (root), which Configure / Unconfigure perform.
package loopback

import (
	"context"
	"hash/fnv"
	"net"
	"strconv"
	"time"
)

// The pool is 127.77.0.0/16 minus the network/broadcast-looking .0 and .255
// host octets — far away from 127.0.0.1 and from anything commonly in use.
const (
	poolA, poolB = 127, 77
	hostsPerNet  = 254 // host octet 1..254
	poolSize     = 256 * hostsPerNet
)

// ForName returns a stable dedicated loopback IP for a service name, probing
// forward past any address for which taken reports true. The same name maps to
// the same IP across runs unless it collides with another running service.
func ForName(name string, taken func(ip string) bool) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	start := int(h.Sum32() % poolSize)
	for i := 0; i < poolSize; i++ {
		n := (start + i) % poolSize
		ip := net.IPv4(poolA, poolB, byte(n/hostsPerNet), byte(n%hostsPerNet+1)).String()
		if taken == nil || !taken(ip) {
			return ip
		}
	}
	return ""
}

// InPool reports whether ip is an address ForName could have produced. Used to
// validate IPs read from the elevated helper's control file before acting on
// them as root.
func InPool(ip string) bool {
	parsed := net.ParseIP(ip).To4()
	if parsed == nil {
		return false
	}
	return parsed[0] == poolA && parsed[1] == poolB && parsed[3] >= 1 && parsed[3] <= hostsPerNet
}

// WaitReady blocks until ip:0 can be bound (the address exists on the loopback
// interface) or timeout/ctx expires. On Windows/Linux it returns at once; on
// macOS it bridges the short gap until the elevated helper adds the lo0 alias,
// so the first bind doesn't fail and trigger a reconnect.
func WaitReady(ctx context.Context, ip string, timeout time.Duration) {
	if !needsSetup {
		return
	}
	deadline := time.Now().Add(timeout)
	for {
		if ln, err := net.Listen("tcp4", net.JoinHostPort(ip, "0")); err == nil {
			ln.Close()
			return
		}
		if time.Now().After(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(150 * time.Millisecond):
		}
	}
}

// PortFree reports whether port is free on 127.0.0.1 — i.e. a forward could
// still claim it on localhost.
func PortFree(port int) bool {
	ln, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
