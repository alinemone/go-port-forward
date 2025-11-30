package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/alinemone/go-port-forward/internal/app"
	"github.com/alinemone/go-port-forward/internal/storage"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	cmd := os.Args[1]

	switch cmd {
	case "a", "add":
		handleAdd()

	case "l", "list":
		handleList()

	case "r", "run":
		handleRun()

	case "d", "delete", "rm":
		handleDelete()

	case "c", "cleanup":
		handleCleanup()

	case "h", "help", "--help", "-h":
		printHelp()

	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		fmt.Println("Run 'pf help' for usage information.")
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
	command := os.Args[3]

	// Join additional args if provided
	if len(os.Args) >= 5 {
		command = command + " " + strings.Join(os.Args[4:], " ")
	}

	stor := storage.New()
	if err := stor.AddService(name, command); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Service '%s' added successfully!\n", name)
}

func handleList() {
	stor := storage.New()
	services, err := stor.LoadServices()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if len(services) == 0 {
		fmt.Println("No services found.")
		return
	}

	fmt.Println("\nSaved Services:\n")

	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)

	for i, name := range names {
		svc := services[name]
		cmd := svc.Command
		if len(cmd) > 60 {
			cmd = cmd[:57] + "..."
		}
		fmt.Printf("  %d. %s\n", i+1, name)
		fmt.Printf("     → %s\n", cmd)
		if svc.Description != "" {
			fmt.Printf("     %s\n", svc.Description)
		}
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

	// Create app
	application, err := app.New()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	defer application.Close()

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down gracefully...")
		cancel()
	}()

	// Validate services exist
	stor := application.GetStorage()
	for _, name := range serviceNames {
		if _, err := stor.GetService(name); err != nil {
			fmt.Printf("Error: Service '%s' not found\n", name)
			os.Exit(1)
		}
	}

	// Run with TUI
	if err := application.Run(ctx, serviceNames); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func handleDelete() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: pf delete <name>")
		os.Exit(1)
	}

	name := os.Args[2]

	stor := storage.New()
	if err := stor.DeleteService(name); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Service '%s' deleted successfully!\n", name)
}

func handleCleanup() {
	fmt.Println("Cleaning up all kubectl and ssh port-forward processes...")

	// Try to kill kubectl processes
	exec.Command("taskkill", "/F", "/IM", "kubectl.exe").Run()

	// Try to kill ssh processes
	exec.Command("taskkill", "/F", "/IM", "ssh.exe").Run()

	fmt.Println("✓ Cleanup complete!")
	fmt.Println("Note: This kills ALL kubectl and ssh processes on your system.")
}

func printHelp() {
	help := `
╔═══════════════════════════════════════════════════════════╗
║              Port Forward Manager v2.0                    ║
╚═══════════════════════════════════════════════════════════╝

Usage:
  pf <command> [arguments]

Commands:
  a, add <name> <command>       Add new service
  l, list                       List all services
  r, run <name1,name2,...>      Run services with TUI
  d, delete <name>              Delete service
  c, cleanup                    Kill all kubectl/ssh processes
  h, help                       Show this help

Examples:
  pf add db "kubectl -n prod port-forward service/postgres 5432:5432"
  pf run db,redis
  pf delete db

Features:
  • Modern TUI with real-time updates
  • Automatic health checking (2-4 seconds detection)
  • Auto-reconnection on failure
  • Error tracking with auto-clear
  • Logging to logs/pf.log

Configuration:
  Edit config.json to customize settings
  Place in same directory as executable

`
	fmt.Println(help)
}
