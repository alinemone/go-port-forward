package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/alinemone/go-port-forward/internal/manager"
	"github.com/alinemone/go-port-forward/internal/storage"

	"charm.land/lipgloss/v2"
)

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
