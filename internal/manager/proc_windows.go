//go:build windows

package manager

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// newShellCommand builds an *exec.Cmd that runs commandStr through cmd.exe.
//
// It sets the raw command line via SysProcAttr.CmdLine instead of passing
// commandStr as an argument to exec.Command. That matters because commandStr may
// embed quoted paths (e.g. --client-certificate="C:\Users\ali mohammadi\...pem"
// for usernames with spaces). If we let Go escape the argument, cmd.exe would
// then re-parse it and the quotes would survive as literal characters in the
// path, producing "The filename, directory name, or volume label syntax is
// incorrect." Feeding cmd.exe the raw line lets it strip the quotes once, so the
// target program receives the path as a single clean argument.
func newShellCommand(commandStr string) *exec.Cmd {
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		CmdLine:       "cmd /C " + commandStr,
	}
	return cmd
}

// killProcessTrees force-kills several process trees in a single taskkill
// invocation, so bulk shutdown costs one process spawn regardless of how many
// services are running.
func killProcessTrees(procs []*os.Process) {
	args := make([]string, 0, 2+len(procs)*2)
	args = append(args, "/F", "/T")
	for _, p := range procs {
		if p != nil {
			args = append(args, "/PID", strconv.Itoa(p.Pid))
		}
	}
	if len(args) == 2 { // no PIDs collected
		return
	}
	exec.Command("taskkill", args...).Run()
}

func killUnixProcessGroup(pid int) {
	// no-op on windows
}

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
