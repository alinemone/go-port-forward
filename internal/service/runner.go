package service

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/alinemone/go-port-forward/internal/logger"
)

// Runner manages the execution of a service command.
type Runner struct {
	state  *State
	logger *logger.Logger
	cmd    *exec.Cmd // Track current command for cleanup
}

// NewRunner creates a new service runner.
func NewRunner(state *State, logger *logger.Logger) *Runner {
	return &Runner{
		state:  state,
		logger: logger,
	}
}

// Run starts the service and monitors it in a loop.
func (r *Runner) Run(ctx context.Context) {
	command := r.state.Command

	// Optimize SSH for faster detection
	if strings.Contains(command, "ssh") && !strings.Contains(command, "ServerAliveInterval") {
		command = strings.Replace(command, "ssh",
			"ssh -o ServerAliveInterval=2 -o ServerAliveCountMax=2 -o ConnectTimeout=3", 1)
	}

	for {
		select {
		case <-ctx.Done():
			// Context cancelled - kill current process if running
			r.killProcess()
			r.logger.ServiceEvent(r.state.Name, "Stopped by user")
			return
		default:
			r.runOnce(ctx, command)

			// Wait before reconnecting
			select {
			case <-ctx.Done():
				r.killProcess()
				r.logger.ServiceEvent(r.state.Name, "Stopped by user")
				return
			case <-time.After(2 * time.Second):
				// Continue to next iteration
			}
		}
	}
}

func (r *Runner) runOnce(ctx context.Context, command string) {
	r.state.SetStatus(StatusConnecting)
	r.logger.ServiceEvent(r.state.Name, "Connecting...")

	// Create command with process group
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "bash", "-c", command)
	}

	// Setup process group for proper cleanup
	setupProcessGroup(cmd)
	r.cmd = cmd // Store for cleanup

	// Capture stderr
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	// Capture stdout
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		r.state.SetError(fmt.Sprintf("Failed to create pipe: %v", err))
		r.logger.ServiceError(r.state.Name, "Failed to create pipe: %v", err)
		r.cmd = nil
		return
	}

	// Start command
	if err := cmd.Start(); err != nil {
		fullErrMsg := r.formatError(err, &stderrBuf)

		// Log full error
		r.logger.ServiceError(r.state.Name, "Failed to start: %s", fullErrMsg)

		// Set truncated error for UI
		displayErrMsg := fullErrMsg
		if len(displayErrMsg) > 100 {
			displayErrMsg = displayErrMsg[:97] + "..."
		}
		r.state.SetError(displayErrMsg)

		r.cmd = nil
		return
	}

	// Monitor stdout in goroutine
	go r.monitorOutput(stdoutPipe)

	// Give it a moment to start
	time.Sleep(500 * time.Millisecond)

	// If still connecting, assume it's online
	if r.state.GetStatus() == StatusConnecting {
		r.state.SetStatus(StatusOnline)
		r.logger.ServiceEvent(r.state.Name, "Connected successfully on port %s→%s",
			r.state.LocalPort, r.state.RemotePort)
	}

	// Wait for process to exit
	err = cmd.Wait()
	r.cmd = nil // Clear after exit

	// Check if it was cancelled
	if ctx.Err() != nil {
		return
	}

	// Process exited - get error if any
	fullErrMsg := r.formatError(err, &stderrBuf)

	// Log the full error message (no truncation)
	if fullErrMsg != "" {
		r.logger.ServiceError(r.state.Name, "Connection closed: %s", fullErrMsg)
	} else {
		r.logger.ServiceEvent(r.state.Name, "Connection closed")
	}

	// For UI, use truncated version
	displayErrMsg := fullErrMsg
	if len(displayErrMsg) > 100 {
		displayErrMsg = displayErrMsg[:97] + "..."
	}

	if displayErrMsg != "" {
		r.state.SetError(displayErrMsg)
	} else {
		r.state.SetError("Connection closed")
	}

	r.state.SetStatus(StatusReconnecting)
}

// killProcess kills the current running process and its children.
func (r *Runner) killProcess() {
	killProcessTree(r.cmd)
	r.cmd = nil
}

func (r *Runner) monitorOutput(pipe io.Reader) {
	scanner := bufio.NewScanner(pipe)
	firstLine := true

	for scanner.Scan() {
		line := scanner.Text()

		// First line usually means successful connection
		if firstLine {
			firstLine = false
			r.state.SetStatus(StatusOnline)
			r.logger.ServiceEvent(r.state.Name, "Output received - connection established")
		}

		// Log the output
		r.logger.Debug("[%s] %s", r.state.Name, line)
	}
}

func (r *Runner) formatError(err error, stderrBuf *bytes.Buffer) string {
	errMsg := ""

	if err != nil {
		errMsg = err.Error()
	}

	if stderrBuf.Len() > 0 {
		stderrMsg := strings.TrimSpace(stderrBuf.String())
		if stderrMsg != "" {
			errMsg = stderrMsg
		}
	}

	// Return full error without truncation (truncation happens in caller)
	return errMsg
}
