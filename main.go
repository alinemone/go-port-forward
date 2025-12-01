package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sort"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	cmd := os.Args[1]

	switch cmd {
	case "a", "add", "-a", "-add", "--a", "--add":
		handleAdd()
	case "l", "list", "-l", "-list", "--l", "--list":
		handleList()
	case "r", "run", "-r", "-run", "--r", "--run":
		handleRun()
	case "d", "delete", "rm", "-d", "-delete", "--d", "--delete":
		handleDelete()
	case "c", "cleanup", "-c", "-cleanup", "--c", "--cleanup":
		handleCleanup()
	case "h", "help", "--help", "-h":
		printHelp()
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		fmt.Println("Run 'pf help' for usage")
		os.Exit(1)
	}
}

func handleAdd() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: pf add <name> <command>")
		fmt.Println("Example: pf add db \"kubectl port-forward service/postgres 5432:5432\"")
		os.Exit(1)
	}

	name := os.Args[2]
	command := strings.Join(os.Args[3:], " ")

	storage := NewStorage()
	if err := storage.Add(name, command); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Service '%s' added\n", name)
}

func handleList() {
	storage := NewStorage()
	services, err := storage.Load()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if len(services) == 0 {
		fmt.Println("No services found")
		return
	}

	fmt.Println("\nServices:")
	fmt.Println()

	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)

	for i, name := range names {
		cmd := services[name]
		if len(cmd) > 70 {
			cmd = cmd[:67] + "..."
		}
		fmt.Printf("  %d. %s\n", i+1, name)
		fmt.Printf("     → %s\n", cmd)
	}
	fmt.Println()
}

func handleRun() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: pf run <name1,name2,...>")
		os.Exit(1)
	}

	serviceNames := strings.Split(os.Args[2], ",")
	for i, name := range serviceNames {
		serviceNames[i] = strings.TrimSpace(name)
	}

	// Create manager
	storage := NewStorage()
	manager := NewManager(storage)

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		cancel()
	}()

	// Validate and start services
	for _, name := range serviceNames {
		if _, err := storage.Get(name); err != nil {
			fmt.Printf("Error: Service '%s' not found\n", name)
			os.Exit(1)
		}
	}

	// Start services
	for _, name := range serviceNames {
		if err := manager.Start(ctx, name); err != nil {
			fmt.Printf("Error starting '%s': %v\n", name, err)
			os.Exit(1)
		}
	}

	// Run TUI
	ui := NewUI(manager)
	p := tea.NewProgram(ui, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Cleanup
	manager.StopAll()
}

func handleDelete() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: pf delete <name>")
		os.Exit(1)
	}

	name := os.Args[2]

	storage := NewStorage()
	if err := storage.Delete(name); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Service '%s' deleted\n", name)
}

func handleCleanup() {
	fmt.Println("Cleaning up kubectl and ssh processes...")

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

func printHelp() {
	help := `
╔════════════════════════════════════════════════════╗
║         Port Forward Manager v2.0                  ║
╚════════════════════════════════════════════════════╝

Usage:
  pf <command> [arguments]

Commands:
  a, add <name> <command>      Add new service
  l, list                      List all services
  r, run <name1,name2,...>     Run services with TUI
  d, delete <name>             Delete service
  c, cleanup                   Kill all kubectl/ssh processes
  h, help                      Show this help

Examples:
  pf add db "kubectl port-forward service/postgres 5432:5432"
  pf run db
  pf run db,redis
  pf delete db

Features:
  • Simple TUI with real-time status
  • Auto-reconnect on failure
  • Error display in terminal
  • Clean shutdown on quit
`
	fmt.Println(help)
}
