package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/alinemone/go-port-forward/internal/manager"
	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/stringutil"

	"charm.land/lipgloss/v2"
)

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

func showGroupUsage() {
	helpSection("Group Management", "pf group <sub>")
	helpRow("add <name> <svcs>", "Create a group from comma-separated services")
	helpRow("add-service <name> <svcs>", "Add services to an existing group")
	helpRow("remove-service <name> <svcs>", "Remove services from a group")
	helpRow("list", "List all groups and their members")
	helpRow("delete <name>", "Delete a group (member services are kept)")
	helpRow("rename <old> <new>", "Rename a group")

	helpSection("Examples", "")
	helpExample("group add database auth,core,crm", "")
	helpExample("group add-service database wallet-pg,redis", "")
	helpExample("group remove-service database redis", "")
	helpExample("group rename database db-group", "")
	helpExample("run database", "run every service in the group")
	helpExample("run database,cache", "run multiple groups")
	helpExample("run database,db", "mix a group and a service")

	helpSection("Note", "")
	helpNote("Group names must not conflict with service names.")
	lipgloss.Println()
}
