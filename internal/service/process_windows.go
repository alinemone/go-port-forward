//go:build windows
// +build windows

package service

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
)

// setupProcessGroup sets up the process group for Windows.
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// killProcessTree kills the process and all its children on Windows.
func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	// Use taskkill to kill the entire process tree
	killCmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", cmd.Process.Pid))
	killCmd.Run() // Ignore errors
}

// KillProcessUsingPort finds and kills processes using the specified port on Windows.
func KillProcessUsingPort(port string) error {
	// Run netstat to find processes using the port
	cmd := exec.Command("netstat", "-ano")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to run netstat: %w", err)
	}

	// Parse output to find PIDs (check all states, not just LISTENING)
	lines := strings.Split(string(output), "\n")
	portPattern := regexp.MustCompile(`:` + regexp.QuoteMeta(port) + `\s`)

	pids := make(map[string]bool)
	for _, line := range lines {
		// Match lines with our port (in any state: LISTENING, ESTABLISHED, TIME_WAIT, etc.)
		if portPattern.MatchString(line) && (strings.Contains(line, "LISTENING") ||
			strings.Contains(line, "ESTABLISHED") ||
			strings.Contains(line, "TIME_WAIT") ||
			strings.Contains(line, "CLOSE_WAIT")) {
			// Extract PID from the end of the line
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				pid := fields[len(fields)-1]
				if pid != "0" {
					pids[pid] = true
				}
			}
		}
	}

	// If no PIDs found through netstat, try to kill kubectl/ssh processes by name
	if len(pids) == 0 {
		// Try to find and kill kubectl processes
		_ = exec.Command("taskkill", "/F", "/IM", "kubectl.exe").Run()
		// Try to find and kill ssh processes
		_ = exec.Command("taskkill", "/F", "/IM", "ssh.exe").Run()
	}

	// Kill each unique PID
	killedAny := false
	for pid := range pids {
		killCmd := exec.Command("taskkill", "/F", "/T", "/PID", pid)
		if err := killCmd.Run(); err == nil {
			killedAny = true
		}
	}

	if !killedAny && len(pids) == 0 {
		// No processes found or killed
		return nil
	}

	return nil
}
