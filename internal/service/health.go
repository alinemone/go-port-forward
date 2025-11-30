package service

import (
	"context"
	"time"

	"github.com/alinemone/go-port-forward/internal/logger"
	"github.com/alinemone/go-port-forward/pkg/netutil"
)

// HealthChecker monitors service health.
type HealthChecker struct {
	state           *State
	logger          *logger.Logger
	interval        time.Duration
	timeout         time.Duration
	failThreshold   int
	consecutiveFail int
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker(state *State, logger *logger.Logger, interval, timeout time.Duration, failThreshold int) *HealthChecker {
	return &HealthChecker{
		state:         state,
		logger:        logger,
		interval:      interval,
		timeout:       timeout,
		failThreshold: failThreshold,
	}
}

// Start begins health checking in a goroutine.
func (h *HealthChecker) Start(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.check(ctx)
		}
	}
}

func (h *HealthChecker) check(ctx context.Context) {
	// Only check if service is supposedly online
	if h.state.GetStatus() != StatusOnline {
		h.consecutiveFail = 0
		return
	}

	// Check if port is open
	healthy := netutil.IsPortOpen(ctx, h.state.LocalPort, h.timeout)

	if healthy {
		h.consecutiveFail = 0
		h.state.SetHealth(true)
	} else {
		h.consecutiveFail++
		h.state.SetHealth(false)

		if h.consecutiveFail >= h.failThreshold {
			// Mark as error and trigger reconnect
			h.logger.ServiceError(h.state.Name, "Health check failed %d times - port not responding", h.consecutiveFail)
			h.state.SetError("Connection lost - health check failed")
			h.state.SetStatus(StatusReconnecting)
			h.consecutiveFail = 0
		}
	}
}
