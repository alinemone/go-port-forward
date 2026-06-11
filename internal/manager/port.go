package manager

import (
	"strconv"
	"strings"
)

func FreePort(port string) []int {
	port = strings.TrimSpace(port)
	if port == "" {
		return nil
	}
	return killListenersOnPort(port)
}

func parseNetstatListeners(output, port string) []int {
	suffix := ":" + port
	seen := make(map[int]bool)
	var pids []int

	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		if !strings.EqualFold(fields[3], "LISTENING") {
			continue
		}
		if !strings.HasSuffix(fields[1], suffix) {
			continue
		}
		pid, err := strconv.Atoi(fields[len(fields)-1])
		if err != nil || pid <= 0 {
			continue
		}
		if !seen[pid] {
			seen[pid] = true
			pids = append(pids, pid)
		}
	}

	return pids
}

func parseLsofPIDs(output string) []int {
	seen := make(map[int]bool)
	var pids []int

	for _, field := range strings.Fields(output) {
		pid, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || pid <= 0 {
			continue
		}
		if !seen[pid] {
			seen[pid] = true
			pids = append(pids, pid)
		}
	}

	return pids
}
