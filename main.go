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

// اجرای برنامه و هدایت به فرمان مناسب
func main() {
	if len(os.Args) < 2 {
		showUsage()
		return
	}

	cmd := normalizeToken(os.Args[1])
	args := os.Args[2:]

	switch cmd {
	case "a", "add":
		runAddCommand(args)
	case "l", "list":
		runListCommand()
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

// یکسان‌سازی نام فرمان با حذف خط تیره‌های ابتدای آن
// نرمال‌سازی ورودی‌ها برای مقایسه یکنواخت
func normalizeToken(value string) string {
	normalized := strings.TrimLeft(strings.TrimSpace(value), "-")
	return strings.ToLower(normalized)
}

// افزودن سرویس جدید به فایل ذخیره‌سازی
func runAddCommand(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: pf add <name> <command>")
		fmt.Println("Example: pf add db \"kubectl port-forward service/postgres 5432:5432\"")
		os.Exit(1)
	}

	name := args[0]
	command := strings.Join(args[1:], " ")

	storage := NewStorage()
	if err := storage.AddService(name, command); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Service '%s' added\n", name)
}

// نمایش لیست سرویس‌های ذخیره‌شده
func runListCommand() {
	storage := NewStorage()
	services, err := storage.LoadServices()
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

// اجرای سرویس‌ها با رابط متنی
func runStartCommand(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: pf run <name1,name2,...>")
		fmt.Println("       pf run all")
		fmt.Println("       pf run <group-name>")
		os.Exit(1)
	}

	storage := NewStorage()
	serviceNames, err := resolveRunTargets(storage, args[0])
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// ساخت مدیر سرویس‌ها
	manager := NewServiceManager(storage)

	// مدیریت سیگنال‌ها برای خروج تمیز
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// بررسی وجود همه سرویس‌ها
	for _, name := range serviceNames {
		if _, err := storage.GetService(name); err != nil {
			fmt.Printf("Error: Service '%s' not found\n", name)
			os.Exit(1)
		}
	}

	// بررسی تداخل پورت‌ها
	conflicts, err := storage.FindPortConflicts(serviceNames)
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

	// اجرای سرویس‌ها
	for _, name := range serviceNames {
		if err := manager.StartService(ctx, name); err != nil {
			fmt.Printf("Error starting '%s': %v\n", name, err)
			os.Exit(1)
		}
	}

	// اجرای رابط متنی
	ui := NewUI(manager, ctx)
	program := tea.NewProgram(ui, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	manager.StopAllServices()
}

// حذف یک سرویس از فایل ذخیره‌سازی
func runDeleteCommand(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: pf delete <name>")
		os.Exit(1)
	}

	name := args[0]
	storage := NewStorage()
	if err := storage.DeleteService(name); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Service '%s' deleted\n", name)
}

// پاک‌سازی تمام پردازش‌های kubectl و ssh
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

// مدیریت فرمان‌های گروه‌ها
func runGroupCommand(args []string) {
	if len(args) < 1 {
		showGroupUsage()
		os.Exit(1)
	}

	subCmd := normalizeToken(args[0])
	storage := NewStorage()

	switch subCmd {
	case "add", "a":
		runGroupAddCommand(storage, args[1:])
	case "list", "ls", "l":
		runGroupListCommand(storage)
	case "delete", "rm", "d":
		runGroupDeleteCommand(storage, args[1:])
	default:
		fmt.Printf("Unknown group command: %s\n", subCmd)
		showGroupUsage()
		os.Exit(1)
	}
}

// افزودن گروه جدید از لیست سرویس‌ها
func runGroupAddCommand(storage *Storage, args []string) {
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

	if err := storage.AddGroup(groupName, serviceNames); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Group '%s' created with %d services\n", groupName, len(serviceNames))
}

// نمایش لیست گروه‌ها
func runGroupListCommand(storage *Storage) {
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

// حذف یک گروه از فایل ذخیره‌سازی
func runGroupDeleteCommand(storage *Storage, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: pf group delete <group-name>")
		os.Exit(1)
	}

	groupName := args[0]
	if err := storage.DeleteGroup(groupName); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Group '%s' deleted\n", groupName)
}

// مدیریت فرمان‌های گواهی‌نامه
func runCertCommand(args []string) {
	if len(args) < 1 {
		showCertUsage()
		os.Exit(1)
	}

	subCmd := normalizeToken(args[0])
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

// افزودن گواهی P12 برای استفاده در kubectl
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

// نمایش گواهی ثبت‌شده
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

// حذف گواهی ثبت‌شده
func runCertRemoveCommand(certMgr *cert.Manager) {
	if err := certMgr.RemoveCertificate(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Certificate removed successfully")
}

// نمایش راهنمای گواهی‌نامه
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

// نمایش راهنمای مدیریت گروه‌ها
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

// نمایش راهنمای کلی برنامه
func showUsage() {
	help := `
Usage:
  pf <command> [arguments]

Commands:
  a, add <name> "<command>"    Add new service
  l, list                      List all services
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

func runVersionCommand() {
	fmt.Printf("pf %s\n", Version)
	fmt.Printf("commit: %s\n", Commit)
	fmt.Printf("built: %s\n", BuildDate)
}

// تعیین سرویس‌هایی که باید اجرا شوند (تک، چندتایی، گروه یا همه)
func resolveRunTargets(storage *Storage, input string) ([]string, error) {
	if input == "all" {
		names, err := storage.ListServiceNames()
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
		hasConflict, err := storage.HasNameConflict(name)
		if err != nil {
			return nil, err
		}
		if hasConflict {
			return nil, fmt.Errorf("name '%s' exists as both service and group", name)
		}

		if _, err := storage.GetService(name); err == nil {
			return serviceNames, nil
		}

		groupServices, err := storage.GetGroupServices(name)
		if err != nil {
			return nil, fmt.Errorf("service or group '%s' not found", name)
		}

		fmt.Printf("Running group '%s' (%d services)...\n", name, len(groupServices))
		return groupServices, nil
	}

	return serviceNames, nil
}
