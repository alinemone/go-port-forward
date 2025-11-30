// Package netutil provides network utility functions.
package netutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// IsPortOpen checks if a TCP port is open and accepting connections.
func IsPortOpen(ctx context.Context, port string, timeout time.Duration) bool {
	address := fmt.Sprintf("localhost:%s", port)

	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// IsHTTPHealthy checks if an HTTP endpoint is healthy.
func IsHTTPHealthy(ctx context.Context, port, path string, timeout time.Duration) bool {
	if path == "" {
		path = "/"
	}

	url := fmt.Sprintf("http://localhost:%s%s", port, path)

	client := &http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Consider 2xx and 3xx as healthy
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

// AutoDetectHealthCheck tries TCP first, then HTTP to determine the best health check method.
// Returns "tcp" or "http" based on what works.
func AutoDetectHealthCheck(ctx context.Context, port string, timeout time.Duration) string {
	// First try TCP
	if !IsPortOpen(ctx, port, timeout) {
		return "tcp" // Default to TCP if port isn't even open yet
	}

	// Port is open, try HTTP
	if IsHTTPHealthy(ctx, port, "/", timeout) {
		return "http"
	}

	// HTTP didn't work, stick with TCP
	return "tcp"
}

// IsPortInUse checks if a port is already bound by listening on it.
// Returns true if the port is in use, false if it's free.
func IsPortInUse(port string) bool {
	address := fmt.Sprintf(":%s", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return true // Port is in use
	}
	listener.Close()
	return false
}
