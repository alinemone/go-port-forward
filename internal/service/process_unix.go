//go:build !windows
// +build !windows

package service

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
)

// setupProcessGroup sets up the process group for Unix-like systems.
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// killProcessTree kills the process group on Unix-like systems.
func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	// Kill the process group
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGKILL)
	} else {
		// Fallback to killing just the process
		cmd.Process.Kill()
	}
}

// KillProcessUsingPort finds and kills processes using the specified port on Unix-like systems.
func KillProcessUsingPort(port string) error {
	// Try lsof first (more common)
	cmd := exec.Command("lsof", "-ti", fmt.Sprintf(":%s", port))
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		// Parse PIDs and kill them
		pids := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, pid := range pids {
			if pid != "" {
				killCmd := exec.Command("kill", "-9", pid)
				killCmd.Run() // Ignore errors
			}
		}
		return nil
	}

	// Fallback to netstat + grep + awk
	cmd = exec.Command("sh", "-c", fmt.Sprintf("netstat -tlnp 2>/dev/null | grep ':%s ' | awk '{print $7}' | cut -d'/' -f1", port))
	output, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to find processes using port %s: %w", port, err)
	}

	if len(output) > 0 {
		pids := strings.Split(strings.TrimSpace(string(output)), "\n")
		pidPattern := regexp.MustCompile(`^\d+$`)
		for _, pid := range pids {
			pid = strings.TrimSpace(pid)
			if pid != "" && pidPattern.MatchString(pid) {
				killCmd := exec.Command("kill", "-9", pid)
				killCmd.Run() // Ignore errors
			}
		}
	}

	return nil
}
