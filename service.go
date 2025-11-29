package main

import (
	"os/exec"
	"runtime"
	"time"
)

func RunLoop(name, command, localPort, remotePort string) {
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

		mu.Lock()
		statuses[name] = ServiceStatus{
			Name:   name,
			Status: "ONLINE",
			Local:  localPort,
			Remote: remotePort,
		}
		mu.Unlock()

		cmd.Run()

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
