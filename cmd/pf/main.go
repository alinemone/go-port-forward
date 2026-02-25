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

	"github.com/alinemone/go-port-forward/internal/cert"
	"github.com/alinemone/go-port-forward/internal/manager"
	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/stringutil"
	"github.com/alinemone/go-port-forward/internal/ui"
	"github.com/alinemone/go-port-forward/internal/version"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if len(os.Args) < 2 {
		showUsage()
		return
	}

	cmd := stringutil.NormalizeToken(os.Args[1])
	args := os.Args[2:]

	switch cmd {
	case "a", "add":
		runAddCommand(args)
	case "l", "list":
		runListCommand()
	case "k", "kubectl":
		runKubectlCommand(args)
	case "r", "run":
		runStartCommand(args)
	case "ra":
		runStartCommand([]string{"all"})
	case "d", "delete", "rm":
		runDeleteCommand(args)
	case "c", "cleanup":
		runCleanupCommand()
	case "g", "group":
		runGroupCommand(args)
	case "cert":
		runCertCommand(args)
	case "h", "help":
		showUsage()
	case "v", "version":
		runVersionCommand()
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		fmt.Println("Run 'pf help' for usage")
		os.Exit(1)
	}
}

func runAddCommand(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: pf add <name> <command>")
		fmt.Println("Example: pf add db \"kubectl port-forward service/postgres 5432:5432\"")
		os.Exit(1)
	}

	name := args[0]
	command := strings.Join(args[1:], " ")

	st := storage.NewStorage()
	if err := st.AddService(name, command); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Service '%s' added\n", name)
}

func runListCommand() {
	st := storage.NewStorage()
	services, err := st.LoadServices()
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

func runStartCommand(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: pf run <name1,name2,...>")
		fmt.Println("       pf run all")
		fmt.Println("       pf run <group-name>")
		os.Exit(1)
	}

	st := storage.NewStorage()
	serviceNames, err := resolveRunTargets(st, args[0])
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	mgr := manager.NewServiceManager(st)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	for _, name := range serviceNames {
		if _, err := st.GetService(name); err != nil {
			fmt.Printf("Error: Service '%s' not found\n", name)
			os.Exit(1)
		}
	}

	conflicts, err := st.FindPortConflicts(serviceNames)
	if err != nil {
		fmt.Printf("Error checking port conflicts: %v\n", err)
		os.Exit(1)
	}

	if len(conflicts) > 0 {
		fmt.Println("\n⚠️  Port Conflicts Detected:")
		fmt.Println()
		for _, conflict := range conflicts {
			fmt.Printf("  Port %s is used by:\n", conflict.Port)
			for _, svc := range conflict.Services {
				fmt.Printf("    • %s\n", svc)
			}
			fmt.Println()
		}
		fmt.Println("Please fix the port conflicts before running these services together.")
		os.Exit(1)
	}

	for _, name := range serviceNames {
		if err := mgr.StartService(ctx, name); err != nil {
			fmt.Printf("Error starting '%s': %v\n", name, err)
			os.Exit(1)
		}
	}

	u := ui.NewUI(mgr, ctx)
	program := tea.NewProgram(u, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	mgr.StopAllServices()
}

func runDeleteCommand(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: pf delete <name>")
		os.Exit(1)
	}

	name := args[0]
	st := storage.NewStorage()
	if err := st.DeleteService(name); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Service '%s' deleted\n", name)
}

func runCleanupCommand() {
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

func runKubectlCommand(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: pf kubectl <kubectl-args...>")
		fmt.Println("Alias: pf k <kubectl-args...>")
		fmt.Println("Example: pf k get pods -n production")
		os.Exit(1)
	}

	finalArgs := append([]string{}, args...)

	certMgr, err := cert.NewManager()
	if err == nil {
		if certConfig, exists := certMgr.GetCertificate(); exists && !hasKubectlClientCertArgs(finalArgs) {
			certArgs := []string{
				"--client-certificate=" + certConfig.CertPath,
				"--client-key=" + certConfig.KeyPath,
			}
			finalArgs = append(certArgs, finalArgs...)
		}
	}

	cmd := exec.Command("kubectl", finalArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Printf("Error: failed to run kubectl: %v\n", err)
		os.Exit(1)
	}
}

func hasKubectlClientCertArgs(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--client-certificate") || strings.HasPrefix(arg, "--client-key") {
			return true
		}
	}
	return false
}

func runGroupCommand(args []string) {
	if len(args) < 1 {
		showGroupUsage()
		os.Exit(1)
	}

	subCmd := stringutil.NormalizeToken(args[0])
	st := storage.NewStorage()

	switch subCmd {
	case "add", "a":
		runGroupAddCommand(st, args[1:])
	case "list", "ls", "l":
		runGroupListCommand(st)
	case "delete", "rm", "d":
		runGroupDeleteCommand(st, args[1:])
	default:
		fmt.Printf("Unknown group command: %s\n", subCmd)
		showGroupUsage()
		os.Exit(1)
	}
}

func runGroupAddCommand(st *storage.Storage, args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: pf group add <group-name> <service1,service2,...>")
		fmt.Println("Example: pf group add database auth,core,crm")
		os.Exit(1)
	}

	groupName := args[0]
	servicesStr := args[1]

	serviceNames := strings.Split(servicesStr, ",")
	for i, name := range serviceNames {
		serviceNames[i] = strings.TrimSpace(name)
	}

	if err := st.AddGroup(groupName, serviceNames); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Group '%s' created with %d services\n", groupName, len(serviceNames))
}

func runGroupListCommand(st *storage.Storage) {
	groups, err := st.ListGroups()
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

func runGroupDeleteCommand(st *storage.Storage, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: pf group delete <group-name>")
		os.Exit(1)
	}

	groupName := args[0]
	if err := st.DeleteGroup(groupName); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Group '%s' deleted\n", groupName)
}

func runCertCommand(args []string) {
	if len(args) < 1 {
		showCertUsage()
		os.Exit(1)
	}

	subCmd := stringutil.NormalizeToken(args[0])
	certMgr, err := cert.NewManager()
	if err != nil {
		fmt.Printf("Error: Failed to initialize certificate manager: %v\n", err)
		os.Exit(1)
	}

	switch subCmd {
	case "add":
		runCertAddCommand(certMgr, args[1:])
	case "list", "ls":
		runCertListCommand(certMgr)
	case "remove", "rm", "delete":
		runCertRemoveCommand(certMgr)
	default:
		fmt.Printf("Unknown cert command: %s\n", subCmd)
		showCertUsage()
		os.Exit(1)
	}
}

func runCertAddCommand(certMgr *cert.Manager, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: pf cert add <p12-file>")
		fmt.Println("Example: pf cert add company-vpn.p12")
		os.Exit(1)
	}

	p12Path := args[0]
	if _, err := os.Stat(p12Path); os.IsNotExist(err) {
		fmt.Printf("Error: P12 file not found: %s\n", p12Path)
		os.Exit(1)
	}

	var password string
	fmt.Print("🔐 P12 password (press Enter if none): ")
	fmt.Scanln(&password)

	if err := certMgr.AddCertificate(p12Path, password); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Certificate added successfully")
	fmt.Println("  This certificate will be used for all kubectl services")
}

func runCertListCommand(certMgr *cert.Manager) {
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

func runCertRemoveCommand(certMgr *cert.Manager) {
	if err := certMgr.RemoveCertificate(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Certificate removed successfully")
}

func showCertUsage() {
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

func showGroupUsage() {
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

func showUsage() {
	help := `
Usage:
  pf <command> [arguments]

Commands:
  a, add <name> "<command>"    Add new service
  l, list                      List all services
  k, kubectl <args...>         Run kubectl with configured certificate
  r, run <name1,name2,...>     Run services with TUI
  ra, run all                  Run all services
  r, run <group-name>          Run a group of services
  d, delete <name>             Delete service
  g, group <subcommand>        Manage groups (add/list/delete)
  c, cleanup                   Kill all kubectl/ssh processes
  cert <subcommand>            Manage certificate (add/list/remove)
  v, version                   Show build version details
  h, help                      Show this help

Examples:
  pf add db "kubectl port-forward service/postgres 5432:5432"
  pf k get pods -n production
  pf kubectl logs deploy/api -f
  pf k exec -it pod/my-pod -- sh
  pf k describe pod my-pod -n production
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

Kubectl Passthrough:
  pf k <kubectl-args...>      Run any kubectl command with auto cert injection
  pf kubectl <args...>        Same as pf k

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

func runVersionCommand() {
	fmt.Printf("pf %s\n", version.Version)
	fmt.Printf("commit: %s\n", version.Commit)
	fmt.Printf("built: %s\n", version.BuildDate)
}

func resolveRunTargets(st *storage.Storage, input string) ([]string, error) {
	if input == "all" {
		names, err := st.ListServiceNames()
		if err != nil {
			return nil, err
		}
		if len(names) == 0 {
			return nil, fmt.Errorf("no services found")
		}
		fmt.Printf("Running all %d services...\n", len(names))
		return names, nil
	}

	serviceNames := strings.Split(input, ",")
	for i, name := range serviceNames {
		serviceNames[i] = strings.TrimSpace(name)
	}

	if len(serviceNames) == 1 {
		name := serviceNames[0]
		hasConflict, err := st.HasNameConflict(name)
		if err != nil {
			return nil, err
		}
		if hasConflict {
			return nil, fmt.Errorf("name '%s' exists as both service and group", name)
		}

		if _, err := st.GetService(name); err == nil {
			return serviceNames, nil
		}

		groupServices, err := st.GetGroupServices(name)
		if err != nil {
			return nil, fmt.Errorf("service or group '%s' not found", name)
		}

		fmt.Printf("Running group '%s' (%d services)...\n", name, len(groupServices))
		return groupServices, nil
	}

	return serviceNames, nil
}
