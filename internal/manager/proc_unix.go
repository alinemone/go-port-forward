//go:build !windows

package manager

import (
	"os"
	"os/exec"
	"syscall"
)

// newShellCommand builds an *exec.Cmd that runs commandStr through sh -c. On
// Unix, exec passes argv straight to execve without the extra command-line
// re-parsing that cmd.exe does on Windows, so quoted paths inside commandStr are
// handled correctly by the shell as-is.
func newShellCommand(commandStr string) *exec.Cmd {
	cmd := exec.Command("sh", "-c", commandStr)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// killProcessTrees force-kills several process trees. On Unix each kill is a
// direct syscall (no process spawn), so a simple loop is already optimal.
func killProcessTrees(procs []*os.Process) {
	for _, p := range procs {
		if p != nil {
			killUnixProcessGroup(p.Pid)
		}
	}
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
