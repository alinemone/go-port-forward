//go:build !windows

package manager

import (
	"os/exec"
	"syscall"
)

func newProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func killUnixProcessGroup(pid int) {
	syscall.Kill(-pid, syscall.SIGKILL)
}

func killListenersOnPort(port string) []int {
	out, err := exec.Command("lsof", "-ti", "tcp:"+port, "-sTCP:LISTEN").Output()
	if err != nil {
		return nil
	}

	pids := parseLsofPIDs(string(out))
	for _, pid := range pids {
		syscall.Kill(pid, syscall.SIGKILL)
	}
	return pids
}
