package manager

import (
	"strconv"
	"strings"
)

// FreePort پروسه‌های listenerِ روی یک پورت TCP محلی را می‌کشد و PIDهای کشته‌شده را برمی‌گرداند.
func FreePort(port string) []int {
	port = strings.TrimSpace(port)
	if port == "" {
		return nil
	}
	return killListenersOnPort(port)
}

// parseNetstatListeners از خروجی `netstat -ano -p tcp` ویندوز،
// PIDهای پروسه‌هایی که روی پورت داده‌شده LISTENING هستند را استخراج می‌کند.
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

// parseLsofPIDs از خروجی `lsof -ti` یونیکس (هر خط یک PID) لیست یکتای PIDها را می‌سازد.
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
