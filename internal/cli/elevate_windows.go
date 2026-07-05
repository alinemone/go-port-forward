//go:build windows

package cli

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/alinemone/go-port-forward/internal/hostsfile"
)

var (
	shell32ELV          = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteExW = shell32ELV.NewProc("ShellExecuteExW")

	kernel32ELV          = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess      = kernel32ELV.NewProc("OpenProcess")
	procWaitForSingleObj = kernel32ELV.NewProc("WaitForSingleObject")
	procCloseHandle      = kernel32ELV.NewProc("CloseHandle")
)

const (
	swHide                = 0
	seeMaskNoCloseProcess = 0x00000040
	synchronize           = 0x00100000
	waitTimeout           = 0x00000102
)

// shellExecuteInfoW mirrors the Win32 SHELLEXECUTEINFOW struct. Field order and
// natural alignment must match the C layout; cbSize is set from unsafe.Sizeof.
type shellExecuteInfoW struct {
	cbSize         uint32
	fMask          uint32
	hwnd           uintptr
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       uintptr
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      uintptr
	dwHotKey       uint32
	hIconOrMonitor uintptr
	hProcess       uintptr
}

// elevateHostsWatch launches a hidden, elevated copy of ourselves running
// `pf __hosts watch <ourPID> <controlFile>`. That helper keeps the hosts-file
// alias block in sync with controlFile (which the unprivileged parent rewrites
// as its running set changes) and removes it once we exit — so a single UAC
// prompt covers the whole session, including services added later, and the TUI
// keeps running unelevated in this terminal. Returns true if the elevated
// process launched (UAC accepted).
func elevateHostsWatch(controlFile string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	params := fmt.Sprintf(`__hosts watch %d "%s"`, os.Getpid(), controlFile)
	return runElevatedHidden(exe, params)
}

// elevateHostsClear launches a hidden, elevated `pf __hosts clear` to strip the
// managed alias block, used by `pf alias clear` when the current process can't
// write the hosts file. Returns true once the block is confirmed gone.
func elevateHostsClear() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if !runElevatedHidden(exe, "__hosts clear") {
		return false
	}
	for i := 0; i < 25; i++ {
		if ok, _ := hostsfile.Verify(nil); !ok {
			return true // beginMarker gone (or file unreadable) → cleared
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// runElevatedHidden re-launches exe with the given parameters via the "runas"
// verb (UAC prompt) in a hidden window. Returns false if the launch failed or
// the user declined the prompt.
func runElevatedHidden(exe, params string) bool {
	verbPtr, _ := syscall.UTF16PtrFromString("runas")
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	paramsPtr, _ := syscall.UTF16PtrFromString(params)

	sei := shellExecuteInfoW{
		fMask:        seeMaskNoCloseProcess,
		lpVerb:       verbPtr,
		lpFile:       exePtr,
		lpParameters: paramsPtr,
		nShow:        swHide,
	}
	sei.cbSize = uint32(unsafe.Sizeof(sei))

	ret, _, _ := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&sei)))
	if ret == 0 {
		return false // launch failed or UAC declined
	}
	if sei.hProcess != 0 {
		procCloseHandle.Call(sei.hProcess)
	}
	return true
}

// parentAlive reports whether the process with the given pid is still running.
// Used by the elevated `__hosts watch` helper to know when to clean up.
func parentAlive(pid int) bool {
	h, _, _ := procOpenProcess.Call(uintptr(synchronize), 0, uintptr(pid))
	if h == 0 {
		return false // can't open → treat as gone
	}
	r, _, _ := procWaitForSingleObj.Call(h, 0)
	procCloseHandle.Call(h)
	return r == waitTimeout // signaled (WAIT_OBJECT_0) means the process exited
}
