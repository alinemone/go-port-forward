//go:build !windows

package manager

import "syscall"

// ساخت process group جدید برای یونیکس
func newProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// کشتن process group در یونیکس
func killUnixProcessGroup(pid int) {
	syscall.Kill(-pid, syscall.SIGKILL)
}
