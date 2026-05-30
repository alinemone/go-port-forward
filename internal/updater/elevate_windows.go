//go:build windows

package updater

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// ShellExecuteW from shell32.dll — used to launch ourselves with the "runas"
// verb, which triggers a UAC elevation prompt.
var (
	shell32          = syscall.NewLazyDLL("shell32.dll")
	procShellExecute = shell32.NewProc("ShellExecuteW")
)

const swShowNormal = 1

// tryElevate re-launches this binary via ShellExecuteW with verb "runas",
// causing Windows to show a UAC prompt. On success the elevated copy runs in
// a new console window and this process exits.
func tryElevate() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate self for re-exec: %w", err)
	}

	args := strings.Join(elevatedArgs(), " ")
	cwd, _ := os.Getwd()

	verbPtr, _ := syscall.UTF16PtrFromString("runas")
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	argsPtr, _ := syscall.UTF16PtrFromString(args)
	cwdPtr, _ := syscall.UTF16PtrFromString(cwd)

	ret, _, callErr := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(exePtr)),
		uintptr(unsafe.Pointer(argsPtr)),
		uintptr(unsafe.Pointer(cwdPtr)),
		uintptr(swShowNormal),
	)

	// ShellExecuteW returns an HINSTANCE; values > 32 mean success.
	if ret <= 32 {
		return fmt.Errorf("ShellExecuteW failed (code=%d): %v", ret, callErr)
	}

	fmt.Println("✓ Elevated update started in a new window.")
	fmt.Println("  This window will exit; check the new window for progress.")
	os.Exit(0)
	return nil
}
