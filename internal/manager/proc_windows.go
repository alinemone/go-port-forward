//go:build windows

package manager

import "syscall"

// ساخت process group جدید برای ویندوز
func newProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// در ویندوز از taskkill استفاده میشه، این تابع فقط برای سازگاری هست
func killUnixProcessGroup(pid int) {
	// no-op on windows
}
