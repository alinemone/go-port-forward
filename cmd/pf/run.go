package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"unicode"

	"github.com/alinemone/go-port-forward/internal/manager"
	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/ui"

	tea "charm.land/bubbletea/v2"
)

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
