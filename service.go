package main

import (
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func RunLoop(name, command, localPort, remotePort string) {
	if strings.Contains(command, "ssh") {
		if !strings.Contains(command, "ServerAliveInterval") {
			command = strings.Replace(command, "ssh", "ssh -o ServerAliveInterval=5 -o ServerAliveCountMax=1 -o ConnectTimeout=5", 1)
		}
	}

	for {
		mu.Lock()
		statuses[name] = ServiceStatus{
			Name:   name,
			Status: "CONNECTING",
			Local:  localPort,
			Remote: remotePort,
		}
		mu.Unlock()

		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/C", command)
		} else {
			cmd = exec.Command("bash", "-c", command)
		}

		// Start the command
		err := cmd.Start()
		if err != nil {
			// Failed to start, keep status as CONNECTING or set to ERROR
			mu.Lock()
			statuses[name] = ServiceStatus{
				Name:   name,
				Status: "RECONNECTING",
				Local:  localPort,
				Remote: remotePort,
			}
			mu.Unlock()
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Command started successfully, now set to ONLINE
		mu.Lock()
		statuses[name] = ServiceStatus{
			Name:   name,
			Status: "ONLINE",
			Local:  localPort,
			Remote: remotePort,
		}
		mu.Unlock()

		// Wait for command to finish (when connection drops)
		cmd.Wait()

		mu.Lock()
		statuses[name] = ServiceStatus{
			Name:   name,
			Status: "RECONNECTING",
			Local:  localPort,
			Remote: remotePort,
		}
		mu.Unlock()

		time.Sleep(500 * time.Millisecond)
	}
}
