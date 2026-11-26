//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// elevateHostsWatch elevates via sudo and launches the hidden `__hosts watch`
// helper as root. Credentials are refreshed synchronously up front (`sudo -v`
// prompts on the terminal, which is still ours because this runs before the TUI
// takes the screen); the watcher is then started non-interactively with
// `sudo -n`. Returns false when sudo is unavailable, the prompt is cancelled,
// or the helper didn't come up, so the caller falls back to running without
// aliases and says so.
//
// The helper must NOT be started in a new session: sudo's cached credentials
// are tied to the controlling terminal (timestamp_type=tty, the default on most
// distros), so `sudo -n` without our tty can't see the ticket `sudo -v` just
// created and fails with "a password is required". The helper detaches itself
// once it runs as root (see detachHelper).
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
	watch.Stderr = os.Stderr // surface sudo's own error, if any, before the TUI starts
	if err := watch.Start(); err != nil {
		return false
	}

	// Start() only proves sudo launched. A refused `sudo -n` exits almost at
	// once, while the real helper keeps running until we exit — so treat an
	// early exit as failure.
	exited := make(chan struct{})
	go func() {
		_ = watch.Wait()
		close(exited)
	}()
	select {
	case <-exited:
		return false
	case <-time.After(1500 * time.Millisecond):
		return true
	}
}

// detachHelper runs inside the elevated `__hosts watch` helper. It moves the
// helper into its own session and ignores terminal signals, so a Ctrl-C or a
// closed terminal can't kill it before it removes the aliases; it exits on its
// own once the parent pf process is gone.
func detachHelper() {
	signal.Ignore(syscall.SIGINT, syscall.SIGHUP, syscall.SIGQUIT)
	_, _ = syscall.Setsid()
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
