//go:build windows

package manager

import (
	"os/exec"
	"strconv"
)

func killListenersOnPort(port string) []int {
	// kubectl port-forward listens on both 127.0.0.1 and ::1, and Windows
	// netstat reports those under separate protocols — scan both so an
	// IPv6-only leftover listener cannot survive.
	seen := make(map[int]bool)
	var pids []int
	for _, proto := range []string{"tcp", "tcpv6"} {
		out, err := exec.Command("netstat", "-ano", "-p", proto).Output()
		if err != nil {
			continue
		}
		for _, pid := range parseNetstatListeners(string(out), port) {
			if !seen[pid] {
				seen[pid] = true
				pids = append(pids, pid)
			}
		}
	}
	for _, pid := range pids {
		exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
	}
	return pids
}
