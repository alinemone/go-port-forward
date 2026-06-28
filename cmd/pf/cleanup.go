package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/alinemone/go-port-forward/internal/manager"
	"github.com/alinemone/go-port-forward/internal/storage"
)

func runCleanupCommand(args []string) {
	if cleanupWantsAll(args) {
		if !cleanupWantsYes(args) && !confirm("This will kill ALL kubectl and ssh processes on this machine.") {
			fmt.Println("Aborted.")
			return
		}
		cleanupAllProcesses()
		return
	}

	st := storage.NewStorage()
	ports, err := configuredPorts(st)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if len(ports) == 0 {
		fmt.Println("No configured service ports found.")
		fmt.Println("Use 'pf cleanup --all' to kill ALL kubectl/ssh processes.")
		return
	}

	fmt.Printf("Freeing configured ports: %s\n", strings.Join(ports, ", "))
	for _, port := range ports {
		killed := manager.FreePort(port)
		if len(killed) > 0 {
			fmt.Printf("  • port %s: killed PID(s) %v\n", port, killed)
		} else {
			fmt.Printf("  • port %s: nothing listening\n", port)
		}
	}
	fmt.Println("✓ Cleanup complete")
	fmt.Println("Tip: use 'pf cleanup --all' to kill ALL kubectl/ssh processes.")
}

func cleanupWantsAll(args []string) bool {
	for _, a := range args {
		switch strings.ToLower(strings.TrimSpace(a)) {
		case "all", "--all", "-a":
			return true
		}
	}
	return false
}

func cleanupWantsYes(args []string) bool {
	for _, a := range args {
		switch strings.ToLower(strings.TrimSpace(a)) {
		case "-y", "--yes":
			return true
		}
	}
	return false
}

func confirm(prompt string) bool {
	fmt.Printf("%s Continue? [y/N]: ", prompt)
	var answer string
	fmt.Scanln(&answer)
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

func cleanupAllProcesses() {
	fmt.Println("Cleaning up ALL kubectl and ssh processes...")

	if runtime.GOOS == "windows" {
		exec.Command("taskkill", "/F", "/IM", "kubectl.exe").Run()
		exec.Command("taskkill", "/F", "/IM", "ssh.exe").Run()
	} else {
		exec.Command("pkill", "-9", "kubectl").Run()
		exec.Command("pkill", "-9", "ssh").Run()
	}

	fmt.Println("✓ Cleanup complete")
	fmt.Println("Note: This kills ALL kubectl and ssh processes")
}

func configuredPorts(st *storage.Storage) ([]string, error) {
	services, err := st.LoadServices()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)

	seen := make(map[string]bool)
	ports := make([]string, 0, len(names))
	for _, name := range names {
		local, _ := storage.ParsePortsFromCommand(services[name])
		if local == "" || seen[local] {
			continue
		}
		seen[local] = true
		ports = append(ports, local)
	}

	return ports, nil
}
