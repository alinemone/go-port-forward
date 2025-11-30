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

		err := cmd.Start()
		if err != nil {
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

		mu.Lock()
		statuses[name] = ServiceStatus{
			Name:   name,
			Status: "ONLINE",
			Local:  localPort,
			Remote: remotePort,
		}
		mu.Unlock()

		_ = cmd.Wait()

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
