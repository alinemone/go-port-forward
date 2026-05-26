//go:build windows

package manager

import (
	"os/exec"
	"strconv"
	"syscall"
)

// ساخت process group جدید برای ویندوز
func newProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// در ویندوز از taskkill استفاده میشه، این تابع فقط برای سازگاری هست
func killUnixProcessGroup(pid int) {
	// no-op on windows
}

// کشتن پروسه‌های listener روی یک پورت در ویندوز با netstat + taskkill
func killListenersOnPort(port string) []int {
	out, err := exec.Command("netstat", "-ano", "-p", "tcp").Output()
	if err != nil {
		return nil
	}

	pids := parseNetstatListeners(string(out), port)
	for _, pid := range pids {
		exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid)).Run()
	}
	return pids
}
