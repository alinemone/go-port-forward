package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"unicode"

	"github.com/alinemone/go-port-forward/internal/cert"
	"github.com/alinemone/go-port-forward/internal/configedit"
	"github.com/alinemone/go-port-forward/internal/manager"
	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/stringutil"
	"github.com/alinemone/go-port-forward/internal/ui"
	"github.com/alinemone/go-port-forward/internal/updater"
	"github.com/alinemone/go-port-forward/internal/version"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// CLI list styling. Hex values mirror the TUI palette (see internal/ui) so the
// command-line lists read with the same brand colors as the interactive view.
// lipgloss auto-detects the terminal and drops color when piped to a non-TTY.
var (
	cliHeading = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#AEB9CC"))
	cliCount   = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C879B")).Italic(true)
	cliIndex   = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C879B"))
	cliName    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5BD4FF"))
	cliArrow   = lipgloss.NewStyle().Foreground(lipgloss.Color("#73FFB6"))
	cliDetail  = lipgloss.NewStyle().Foreground(lipgloss.Color("#EAEEF5"))
	cliMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C879B"))
	cliTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5BD4FF"))
	cliBorder  = lipgloss.NewStyle().Foreground(lipgloss.Color("#33415A"))
)

// printList renders a titled, numbered list with a consistent two-line item
// layout: "  N. <title>" then "     → <detail>". headingMeta is an optional
// dim suffix on the heading (e.g. a count).
func printList(heading, headingMeta string, items [][2]string) {
	lipgloss.Println()
	line := cliHeading.Render(heading)
	if headingMeta != "" {
		line += cliCount.Render("  " + headingMeta)
	}
	lipgloss.Println(line)
	lipgloss.Println()
	for i, it := range items {
		lipgloss.Printf("  %s %s\n", cliIndex.Render(fmt.Sprintf("%2d.", i+1)), cliName.Render(it[0]))
		lipgloss.Printf("     %s %s\n", cliArrow.Render("→"), cliDetail.Render(it[1]))
	}
	lipgloss.Println()
}

func main() {
	updater.CleanupStaleArtifacts()

	storage.NewStorage().EnsureExists()

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
	case "rename", "ren", "mv":
		runRenameCommand(args)
	case "c", "cleanup":
		runCleanupCommand(args)
	case "g", "group":
		runGroupCommand(args)
	case "cert":
		runCertCommand(args)
	case "edit", "config":
		runEditCommand()
	case "icon", "icons":
		runIconCommand(args)
	case "h", "help":
		showUsage()
	case "v", "version":
		runVersionCommand()
	case "u", "update":
		runUpdateCommand(args)
	default:
		// Bare `pf <service|group>` is a shortcut for `pf run <service|group>`,
		// but only when the first token actually names something runnable — a
		// genuine typo still falls through to the unknown-command message.
		if looksLikeRunTarget(storage.NewStorage(), strings.Join(os.Args[1:], " ")) {
			runStartCommand(os.Args[1:])
			return
		}
		lipgloss.Println(cliMuted.Render("Unknown command: " + cmd))
		lipgloss.Println(cliMuted.Render("Run 'pf help' for usage"))
		os.Exit(1)
	}
}

// looksLikeRunTarget reports whether the first whitespace/comma-separated token
// names an existing service or group, so a bare `pf <name>` can be treated as a
// run. Read-only and quiet: it never prints or mutates storage.
func looksLikeRunTarget(st runTargetStore, input string) bool {
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	if len(fields) == 0 {
		return false
	}
	first := fields[0]
	if _, err := st.GetService(first); err == nil {
		return true
	}
	if _, err := st.GetGroupServices(first); err == nil {
		return true
	}
	return false
}

func runIconCommand(args []string) {
	st := storage.NewStorage()

	action := ""
	if len(args) > 0 {
		action = strings.ToLower(strings.TrimSpace(args[0]))
	}

	switch action {
	case "", "status":
		enabled, err := st.IconEnabled()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		printIconStatus(enabled)
	case "on", "enable", "true":
		setIcons(st, true)
	case "off", "disable", "false":
		setIcons(st, false)
	default:
		fmt.Printf("Unknown option: %s\n", action)
		fmt.Println("Usage: pf icon [on|off|status]")
		os.Exit(1)
	}
}

func setIcons(st *storage.Storage, enabled bool) {
	if err := st.SetIconEnabled(enabled); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	printIconStatus(enabled)
	if enabled {
		fmt.Println("Note: icons need a Nerd Font (https://www.nerdfonts.com) — without")
		fmt.Println("one they render as blank boxes. Set your terminal to a Nerd Font.")
	}
}

func printIconStatus(enabled bool) {
	if enabled {
		fmt.Println("✓ Service icons: ON")
	} else {
		fmt.Println("○ Service icons: OFF")
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
		lipgloss.Println(cliMuted.Render("No services found"))
		return
	}

	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([][2]string, 0, len(names))
	for _, name := range names {
		items = append(items, [2]string{name, services[name]})
	}
	printList("Services", fmt.Sprintf("(%d)", len(items)), items)
}

func runStartCommand(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: pf run <name1,name2,...>")
		fmt.Println("       pf run all")
		fmt.Println("       pf run <group-name>")
		fmt.Println("       pf run <group1,group2,...>")
		fmt.Println("       pf run <group-or-service,...>")
		os.Exit(1)
	}

	st := storage.NewStorage()
	serviceNames, err := resolveRunTargets(st, strings.Join(args, " "))
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

	// Start UI immediately
	u := ui.NewUI(mgr, ctx)
	program := tea.NewProgram(u)

	// Start all services in parallel - they will appear in UI as they connect
	for _, name := range serviceNames {
		go func(serviceName string) {
			if err := mgr.StartService(ctx, serviceName); err != nil {
				fmt.Printf("Error starting '%s': %v\n", serviceName, err)
			}
		}(name)
	}

	if _, err := program.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	mgr.StopAllServices()
}

func runRenameCommand(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: pf rename <old-name> <new-name>")
		fmt.Println("Example: pf rename db database")
		os.Exit(1)
	}

	oldName := args[0]
	newName := args[1]

	if err := manager.ValidateServiceName(newName); err != nil {
		fmt.Printf("Error: invalid new name: %v\n", err)
		os.Exit(1)
	}

	st := storage.NewStorage()

	if _, err := st.GetService(oldName); err == nil {
		if err := st.RenameService(oldName, newName); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Service renamed '%s' → '%s'\n", oldName, newName)
		return
	}

	if _, err := st.GetGroupServices(oldName); err == nil {
		if err := st.RenameGroup(oldName, newName); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Group renamed '%s' → '%s'\n", oldName, newName)
		return
	}

	fmt.Printf("Error: service or group '%s' not found\n", oldName)
	os.Exit(1)
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
	case "rename", "ren", "mv":
		runGroupRenameCommand(st, args[1:])
	case "add-service", "addsvc", "as":
		runGroupAddServiceCommand(st, args[1:])
	case "remove-service", "rmsvc", "rs":
		runGroupRemoveServiceCommand(st, args[1:])
	default:
		fmt.Printf("Unknown group command: %s\n", subCmd)
		showGroupUsage()
		os.Exit(1)
	}
}

func splitNameList(args []string) []string {
	input := strings.Join(args, " ")
	return strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
}

func runGroupAddCommand(st *storage.Storage, args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: pf group add <group-name> <service1,service2,...>")
		fmt.Println("Example: pf group add database auth,core,crm")
		os.Exit(1)
	}

	groupName := args[0]
	serviceNames := splitNameList(args[1:])
	if len(serviceNames) == 0 {
		fmt.Println("Usage: pf group add <group-name> <service1,service2,...>")
		os.Exit(1)
	}

	if err := st.AddGroup(groupName, serviceNames); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Group '%s' created with %d services\n", groupName, len(serviceNames))
}

func runGroupAddServiceCommand(st *storage.Storage, args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: pf group add-service <group-name> <service1,service2,...>")
		fmt.Println("Example: pf group add-service database redis,wallet-pg")
		os.Exit(1)
	}

	groupName := args[0]
	serviceNames := splitNameList(args[1:])
	if len(serviceNames) == 0 {
		fmt.Println("Usage: pf group add-service <group-name> <service1,service2,...>")
		os.Exit(1)
	}

	if err := st.AddServicesToGroup(groupName, serviceNames); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	services, _ := st.GetGroupServices(groupName)
	fmt.Printf("✓ Added %d service(s) to group '%s' (now %d total)\n", len(serviceNames), groupName, len(services))
}

func runGroupRemoveServiceCommand(st *storage.Storage, args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: pf group remove-service <group-name> <service1,service2,...>")
		fmt.Println("Example: pf group remove-service database redis")
		os.Exit(1)
	}

	groupName := args[0]
	serviceNames := splitNameList(args[1:])
	if len(serviceNames) == 0 {
		fmt.Println("Usage: pf group remove-service <group-name> <service1,service2,...>")
		os.Exit(1)
	}

	if err := st.RemoveServicesFromGroup(groupName, serviceNames); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	services, _ := st.GetGroupServices(groupName)
	fmt.Printf("✓ Removed %d service(s) from group '%s' (now %d total)\n", len(serviceNames), groupName, len(services))
}

func runGroupRenameCommand(st *storage.Storage, args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: pf group rename <old-name> <new-name>")
		os.Exit(1)
	}

	oldName := args[0]
	newName := args[1]

	if err := manager.ValidateServiceName(newName); err != nil {
		fmt.Printf("Error: invalid new name: %v\n", err)
		os.Exit(1)
	}

	if err := st.RenameGroup(oldName, newName); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Group renamed '%s' → '%s'\n", oldName, newName)
}

func runGroupListCommand(st *storage.Storage) {
	groups, err := st.ListGroups()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if len(groups) == 0 {
		lipgloss.Println(cliMuted.Render("No groups found"))
		lipgloss.Println(cliMuted.Render("Use 'pf group add <name> <services>' to create a group"))
		return
	}

	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([][2]string, 0, len(names))
	for _, name := range names {
		services := groups[name]
		title := fmt.Sprintf("%s  (%d)", name, len(services))
		detail := strings.Join(services, ", ")
		if detail == "" {
			detail = "(empty)"
		}
		items = append(items, [2]string{title, detail})
	}
	printList("Groups", fmt.Sprintf("(%d)", len(items)), items)
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
		lipgloss.Println(cliMuted.Render("No certificate configured"))
		lipgloss.Println(cliMuted.Render("Use 'pf cert add <p12-file>' to add a certificate"))
		return
	}

	lipgloss.Println()
	lipgloss.Println(cliHeading.Render("📜 Configured Certificate"))
	lipgloss.Println()
	for _, kv := range [][2]string{
		{"P12", config.P12Path},
		{"Cert", config.CertPath},
		{"Key", config.KeyPath},
	} {
		lipgloss.Printf("  %s %s\n", cliName.Render(fmt.Sprintf("%-5s", kv[0])), cliDetail.Render(kv[1]))
	}
	lipgloss.Println()
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
  pf group add <name> <svc1,svc2,...>            Create a group
  pf group add-service <name> <svc1,svc2,...>    Add services to a group
  pf group remove-service <name> <svc1,...>      Remove services from a group
  pf group list                                  List all groups
  pf group delete <name>                         Delete a group
  pf group rename <old> <new>                    Rename a group

Examples:
  pf group add database auth,core,crm
  pf group add-service database wallet-pg,redis
  pf group remove-service database redis
  pf group list
  pf group delete database
  pf group rename database db-group
  pf run database                        Run all services in group
  pf run database,cache                  Run multiple groups
  pf run database,db                     Run mixed group and service

Note: Group names must not conflict with service names.
`
	fmt.Println(help)
}

// helpSection prints a blank line then a bold, uppercased section header marked
// with an accent "▸" so the sections separate cleanly at a glance.
func helpSection(title string) {
	lipgloss.Println()
	lipgloss.Println("  " + cliArrow.Render("▸") + " " + cliHeading.Render(strings.ToUpper(title)))
}

// helpRow prints one "name  description" command row with the name in a fixed
// column so descriptions line up down the page.
func helpRow(name, desc string) {
	lipgloss.Printf("    %s  %s\n", cliName.Render(fmt.Sprintf("%-24s", name)), cliDetail.Render(desc))
}

// helpExample prints "pf <cmd>" with an optional trailing comment.
func helpExample(cmd, note string) {
	line := "    " + cliName.Render("pf ") + cliDetail.Render(cmd)
	if note != "" {
		line += "  " + cliMuted.Render("# "+note)
	}
	lipgloss.Println(line)
}

func showUsage() {
	titleBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#33415A")).
		Padding(0, 2).
		Render(cliTitle.Render("⚡ pf") + cliMuted.Render("  ·  Port Forward Manager"))

	lipgloss.Println()
	lipgloss.Println(titleBox)
	lipgloss.Println("  " + cliMuted.Render("Manage kubectl/ssh port-forwards with a live status TUI."))

	helpSection("Usage")
	helpRow("pf <command> [args]", "Run a command (see below)")
	helpRow("pf <service|group>", "Shortcut: runs it directly, same as `pf run`")

	helpSection("Run")
	helpRow("r, run <targets>", "Run services/groups in the live TUI (comma-separated)")
	helpRow("ra, run all", "Run every configured service")
	helpRow("<service|group>", "Bare name(s) run directly — no `run` needed")

	helpSection("Services")
	helpRow(`a, add <name> "<cmd>"`, "Add a service")
	helpRow("l, list", "List all services")
	helpRow("d, delete <name>", "Delete a service")
	helpRow("rename <old> <new>", "Rename a service or group")

	helpSection("Groups")
	helpRow("g, group <subcommand>", "add · add-service · remove-service · list · delete · rename")

	helpSection("Certificate")
	helpRow("cert <subcommand>", "add · list · remove  (auto-injected into every kubectl call)")
	helpRow("k, kubectl <args>", "Run kubectl with the configured certificate")

	helpSection("Tools")
	helpRow("edit", "Bulk-edit services & groups in $EDITOR")
	helpRow("icon [on|off|status]", "Toggle Nerd Font icons (needs a Nerd Font)")
	helpRow("c, cleanup [--all]", "Free configured ports (--all kills all kubectl/ssh)")
	helpRow("u, update [--force]", "Update pf to the latest GitHub release")
	helpRow("v, version", "Show build version details")
	helpRow("h, help", "Show this help")

	helpSection("Examples")
	helpExample("db,redis", "run two services")
	helpExample("backend", "run a group")
	helpExample(`add db "kubectl port-forward service/postgres 5432:5432"`, "")
	helpExample("group add backend auth,core,crm", "")
	helpExample("k get pods -n production", "")

	lipgloss.Println()
	lipgloss.Println("  " + cliBorder.Render("https://github.com/alinemone/go-port-forward"))
	lipgloss.Println()
}

func runEditCommand() {
	st := storage.NewStorage()

	data, err := st.LoadData()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	seed, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	tmp, err := os.CreateTemp("", "pf-config-*.json")
	if err != nil {
		fmt.Printf("Error: failed to create temp file: %v\n", err)
		os.Exit(1)
	}
	tmpPath := tmp.Name()
	tmp.Write(seed)
	tmp.Close()

	for {
		cmd, err := configedit.EditorCommand(tmpPath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Remove(tmpPath)
			os.Exit(1)
		}

		if err := cmd.Run(); err != nil {
			fmt.Printf("Error: editor exited with error: %v\n", err)
			os.Remove(tmpPath)
			os.Exit(1)
		}

		edited, err := os.ReadFile(tmpPath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Remove(tmpPath)
			os.Exit(1)
		}

		validated, err := configedit.Validate(edited)
		if err == nil {
			if err := st.SaveData(validated); err != nil {
				fmt.Printf("Error: failed to save config: %v\n", err)
				os.Remove(tmpPath)
				os.Exit(1)
			}
			fmt.Printf("✓ Config saved: %d service(s), %d group(s)\n", len(validated.Services), len(validated.Groups))
			os.Remove(tmpPath)
			return
		}

		fmt.Printf("\n✗ Invalid config: %v\n", err)
		fmt.Print("Reopen to fix? [Y/n]: ")
		var answer string
		fmt.Scanln(&answer)
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer == "n" || answer == "no" {
			fmt.Printf("Aborted. Your edits are preserved at: %s\n", tmpPath)
			return
		}
	}
}

func runVersionCommand() {
	fmt.Printf("pf %s\n", version.Version)
	fmt.Printf("commit: %s\n", version.Commit)
	fmt.Printf("built: %s\n", version.BuildDate)
}

func runUpdateCommand(args []string) {
	opts := updater.Options{CurrentVersion: version.Version}
	for _, a := range args {
		switch strings.ToLower(strings.TrimSpace(a)) {
		case "-y", "--yes":
			opts.AssumeYes = true
		case "-f", "--force":
			opts.Force = true
		case "-h", "--help":
			showUpdateUsage()
			return
		default:
			fmt.Printf("Unknown flag for update: %s\n", a)
			showUpdateUsage()
			os.Exit(1)
		}
	}

	if err := updater.Run(opts); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func showUpdateUsage() {
	help := `
Update:
  pf update                Download the latest release for this OS/arch and replace this binary in-place
  pf update --yes          Skip confirmation prompt (for scripts)
  pf update --force        Re-install even if already on the latest version

Notes:
  • The binary's current path is auto-detected via os.Executable(), so this works
    no matter where you placed pf on disk (any directory on PATH is fine).
  • On Windows the running pf.exe is renamed to pf.exe.old and cleaned up on next launch.
  • If the install directory is not writable, pf will try to relaunch itself with
    sudo (Linux/macOS) or trigger a UAC prompt (Windows).
`
	fmt.Println(help)
}

type runTargetStore interface {
	ListServiceNames() ([]string, error)
	HasNameConflict(name string) (bool, error)
	GetService(name string) (string, error)
	GetGroupServices(name string) ([]string, error)
}

func resolveRunTargets(st runTargetStore, input string) ([]string, error) {
	if strings.TrimSpace(input) == "all" {
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

	targets := strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})

	if len(targets) == 0 {
		return nil, fmt.Errorf("no run targets provided")
	}

	if len(targets) == 1 {
		return resolveSingleRunTarget(st, targets[0])
	}

	resolvedServices := make([]string, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))

	for _, target := range targets {
		services, err := resolveSingleRunTarget(st, target)
		if err != nil {
			return nil, err
		}

		for _, serviceName := range services {
			if _, exists := seen[serviceName]; exists {
				continue
			}
			seen[serviceName] = struct{}{}
			resolvedServices = append(resolvedServices, serviceName)
		}
	}

	return resolvedServices, nil
}

func resolveSingleRunTarget(st runTargetStore, target string) ([]string, error) {
	if target == "" {
		return nil, fmt.Errorf("invalid run target: empty value")
	}

	hasConflict, err := st.HasNameConflict(target)
	if err != nil {
		return nil, err
	}
	if hasConflict {
		return nil, fmt.Errorf("name '%s' exists as both service and group", target)
	}

	if _, err := st.GetService(target); err == nil {
		return []string{target}, nil
	} else if !isNotFoundErr(err) {
		return nil, err
	}

	groupServices, err := st.GetGroupServices(target)
	if err == nil {
		if len(groupServices) > 0 {
			fmt.Printf("Running group '%s' (%d services)...\n", target, len(groupServices))
		}
		return groupServices, nil
	}
	if !isNotFoundErr(err) {
		return nil, err
	}

	return nil, fmt.Errorf("service or group '%s' not found", target)
}

func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not found")
}
