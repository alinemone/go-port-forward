//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// elevateHostsWatch elevates via sudo and launches the hidden `__hosts watch`
// helper as root. Credentials are refreshed synchronously up front (`sudo -v`
// prompts on the terminal, which is still ours because this runs before the TUI
// takes the screen); the watcher is then started non-interactively in its own
// session so it keeps reconciling in the background and isn't killed by a Ctrl-C
// sent to pf. Returns false when sudo is unavailable or the prompt is cancelled,
// so the caller falls back to running without aliases.
func elevateHostsWatch(controlFile string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}

	auth := exec.Command("sudo", "-v")
	auth.Stdin, auth.Stdout, auth.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := auth.Run(); err != nil {
		return false
	}

	watch := exec.Command("sudo", "-n", exe, "__hosts", "watch", strconv.Itoa(os.Getpid()), controlFile)
	watch.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return watch.Start() == nil
}

// elevateHostsClear elevates via sudo to strip the managed alias block, used by
// `pf alias clear` when not running as root.
func elevateHostsClear() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	cmd := exec.Command("sudo", exe, "__hosts", "clear")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run() == nil
}

// parentAlive reports whether the process with the given pid is still running,
// probing with signal 0 (which only checks for existence).
func parentAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
