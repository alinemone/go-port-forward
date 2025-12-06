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

	"github.com/alinemone/go-port-forward/cert"
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
	case "g", "group", "-g", "-group", "--g", "--group":
		handleGroup()
	case "cert":
		handleCert()
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
		fmt.Println("       pf run all")
		fmt.Println("       pf run <group-name>")
		os.Exit(1)
	}

	storage := NewStorage()
	input := os.Args[2]
	var serviceNames []string

	// Handle "all" keyword
	if input == "all" {
		names, err := storage.GetAllServiceNames()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		if len(names) == 0 {
			fmt.Println("No services found")
			os.Exit(1)
		}
		serviceNames = names
		fmt.Printf("Running all %d services...\n", len(serviceNames))
	} else {
		// Parse comma-separated list
		serviceNames = strings.Split(input, ",")
		for i, name := range serviceNames {
			serviceNames[i] = strings.TrimSpace(name)
		}

		// If single name, check for conflicts and groups
		if len(serviceNames) == 1 {
			name := serviceNames[0]

			// Check for conflicts first
			hasConflict, err := storage.CheckNameConflict(name)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			if hasConflict {
				fmt.Printf("Error: Name '%s' exists as both service and group\n", name)
				fmt.Printf("Please rename either the service or the group to resolve the conflict\n")
				os.Exit(1)
			}

			// Try as service first (priority)
			if _, err := storage.Get(name); err == nil {
				// It's a service, continue
			} else {
				// Try as group
				if groupServices, err := storage.GetGroup(name); err == nil {
					serviceNames = groupServices
					fmt.Printf("Running group '%s' (%d services)...\n", name, len(serviceNames))
				} else {
					fmt.Printf("Error: Service or group '%s' not found\n", name)
					os.Exit(1)
				}
			}
		}
	}

	// Create manager
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

	// Validate all services exist
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
	ui := NewUI(manager, ctx)
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

func handleGroup() {
	if len(os.Args) < 3 {
		printGroupHelp()
		os.Exit(1)
	}

	subCmd := os.Args[2]
	storage := NewStorage()

	switch subCmd {
	case "add", "a":
		handleGroupAdd(storage)
	case "list", "ls", "l":
		handleGroupList(storage)
	case "delete", "rm", "d":
		handleGroupDelete(storage)
	default:
		fmt.Printf("Unknown group command: %s\n", subCmd)
		printGroupHelp()
		os.Exit(1)
	}
}

func handleGroupAdd(storage *Storage) {
	if len(os.Args) < 5 {
		fmt.Println("Usage: pf group add <group-name> <service1,service2,...>")
		fmt.Println("Example: pf group add database auth,core,crm")
		os.Exit(1)
	}

	groupName := os.Args[3]
	servicesStr := os.Args[4]

	serviceNames := strings.Split(servicesStr, ",")
	for i, name := range serviceNames {
		serviceNames[i] = strings.TrimSpace(name)
	}

	if err := storage.AddGroup(groupName, serviceNames); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Group '%s' created with %d services\n", groupName, len(serviceNames))
}

func handleGroupList(storage *Storage) {
	groups, err := storage.ListGroups()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if len(groups) == 0 {
		fmt.Println("No groups found")
		fmt.Println("Use 'pf group add <name> <services>' to create a group")
		return
	}

	fmt.Println("\nGroups:")
	fmt.Println()

	// Sort group names
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)

	for i, name := range names {
		services := groups[name]
		fmt.Printf("  %d. %s (%d services)\n", i+1, name, len(services))
		fmt.Printf("     → %s\n", strings.Join(services, ", "))
	}
	fmt.Println()
}

func handleGroupDelete(storage *Storage) {
	if len(os.Args) < 4 {
		fmt.Println("Usage: pf group delete <group-name>")
		os.Exit(1)
	}

	groupName := os.Args[3]

	if err := storage.DeleteGroup(groupName); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Group '%s' deleted\n", groupName)
}

func handleCert() {
	if len(os.Args) < 3 {
		printCertHelp()
		os.Exit(1)
	}

	subCmd := os.Args[2]

	certMgr, err := cert.NewManager()
	if err != nil {
		fmt.Printf("Error: Failed to initialize certificate manager: %v\n", err)
		os.Exit(1)
	}

	switch subCmd {
	case "add":
		handleCertAdd(certMgr)
	case "list", "ls":
		handleCertList(certMgr)
	case "remove", "rm", "delete":
		handleCertRemove(certMgr)
	default:
		fmt.Printf("Unknown cert command: %s\n", subCmd)
		printCertHelp()
		os.Exit(1)
	}
}

func handleCertAdd(certMgr *cert.Manager) {
	if len(os.Args) < 4 {
		fmt.Println("Usage: pf cert add <p12-file>")
		fmt.Println("Example: pf cert add company-vpn.p12")
		os.Exit(1)
	}

	p12Path := os.Args[3]

	// Check if P12 file exists
	if _, err := os.Stat(p12Path); os.IsNotExist(err) {
		fmt.Printf("Error: P12 file not found: %s\n", p12Path)
		os.Exit(1)
	}

	// Ask for password
	var password string
	fmt.Print("🔐 P12 password (press Enter if none): ")
	fmt.Scanln(&password)

	// Add global certificate
	if err := certMgr.AddCertificate(p12Path, password); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Certificate added successfully")
	fmt.Println("  This certificate will be used for all kubectl services")
}

func handleCertList(certMgr *cert.Manager) {
	config, exists := certMgr.GetCertificate()

	if !exists {
		fmt.Println("No certificate configured")
		fmt.Println("Use 'pf cert add <p12-file>' to add a certificate")
		return
	}

	fmt.Println("\n📜 Configured Certificate:")
	fmt.Println()
	fmt.Printf("  P12:  %s\n", config.P12Path)
	fmt.Printf("  Cert: %s\n", config.CertPath)
	fmt.Printf("  Key:  %s\n", config.KeyPath)
	fmt.Println()
}

func handleCertRemove(certMgr *cert.Manager) {
	if err := certMgr.RemoveCertificate(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Certificate removed successfully")
}

func printCertHelp() {
	help := `
Certificate Management:
  pf cert add <p12-file>      Add certificate for all services
  pf cert list                Show configured certificate
  pf cert remove              Remove certificate

Examples:
  pf cert add company-vpn.p12
  pf cert list
  pf cert remove

Note: The certificate will be automatically used for all kubectl services.
`
	fmt.Println(help)
}

func printGroupHelp() {
	help := `
Group Management:
  pf group add <name> <svc1,svc2,...>    Create a group
  pf group list                          List all groups
  pf group delete <name>                 Delete a group

Examples:
  pf group add database auth,core,crm
  pf group add redis redis-dastyar,redisearch-dastyar
  pf group list
  pf group delete database
  pf run database                        Run all services in group

Note: Group names must not conflict with service names.
`
	fmt.Println(help)
}

func printHelp() {
	help := `
Usage:
  pf <command> [arguments]

Commands:
  a, add <name> <command>      Add new service
  l, list                      List all services
  r, run <name1,name2,...>     Run services with TUI
  r, run all                   Run all services
  r, run <group-name>          Run a group of services
  d, delete <name>             Delete service
  g, group <subcommand>        Manage groups (add/list/delete)
  c, cleanup                   Kill all kubectl/ssh processes
  cert <subcommand>            Manage certificate (add/list/remove)
  h, help                      Show this help

Examples:
  pf add db "kubectl port-forward service/postgres 5432:5432"
  pf run db
  pf run db,redis
  pf run all
  pf delete db

Group Management:
  pf group add database auth,core,crm
  pf group list
  pf group delete database
  pf run database

Certificate Management:
  pf cert add <p12-file>      Add certificate (used for all kubectl services)
  pf cert list                Show configured certificate
  pf cert remove              Remove certificate

Features:
  • Simple TUI with real-time status
  • Auto-reconnect on failure
  • Certificate support (P12) for secure kubectl connections
  • Group services for easier management
  • Run all services at once
  • Error display in terminal
  • Clean shutdown on quit
`
	fmt.Println(help)
}
