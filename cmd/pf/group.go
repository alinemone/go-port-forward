package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/alinemone/go-port-forward/internal/manager"
	"github.com/alinemone/go-port-forward/internal/storage"

	"charm.land/lipgloss/v2"
)

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
	uHead("GROUPS:")
	uRow(34, "group add <name> <svcs>", "Create a group from comma-separated services")
	uRow(34, "group add-service <name> <svcs>", "Add services to an existing group")
	uRow(34, "group remove-service <name> <svcs>", "Remove services from a group")
	uRow(34, "group list", "List all groups and their members")
	uRow(34, "group delete <name>", "Delete a group (member services are kept)")
	uRow(34, "group rename <old> <new>", "Rename a group")
	uExample(
		"group add database auth,core,crm",
		"group add-service database wallet-pg,redis",
		"group remove-service database redis",
		"group rename database db-group",
		"run database",
		"run database,cache",
		"run database,db",
	)

	uHead("NOTES:")
	fmt.Println("  Group names must not conflict with service names.")
	fmt.Println()
}
