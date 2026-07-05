package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/alinemone/go-port-forward/internal/hostsfile"
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

	localPorts := make([]string, 0, len(serviceNames))
	for _, name := range serviceNames {
		command, err := st.GetService(name)
		if err != nil {
			fmt.Printf("Error: Service '%s' not found\n", name)
			os.Exit(1)
		}
		if localPort, _ := storage.ParsePortsFromCommand(command); localPort != "" {
			localPorts = append(localPorts, localPort)
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

	u := ui.NewUI(ctx, mgr, st)
	program := tea.NewProgram(u)

	// StartService only registers the service and spawns its own run loop, so
	// this loop is fast and finishes before the TUI takes over the terminal.
	// Runtime connect/error states are shown live inside the UI; printing to
	// stdout after program.Run() would corrupt the screen.
	for _, name := range serviceNames {
		if err := mgr.StartService(ctx, name); err != nil {
			fmt.Printf("Error starting '%s': %v\n", name, err)
			os.Exit(1)
		}
	}

	// Keep the system hosts file in sync with the running services so each is
	// reachable by its in-cluster FQDN on its local port — including services
	// added later from the manage overlay. Best-effort: any failure here never
	// blocks the forwards. Started before the TUI takes the screen so an
	// elevation prompt (when needed) is visible.
	clearAliases := startAliasReconciler(ctx, st, mgr)

	// Always tear down every service before returning — even when the UI exits
	// with an error or is killed by a signal — so no forwarder is left holding a
	// port. StopAllServices is idempotent, so the in-UI quit path calling it too
	// is harmless.
	_, runErr := program.Run()
	mgr.StopAllServices()

	// Remove any hosts-file aliases we added directly. When an elevated helper
	// applied them instead, clearAliases is a no-op — that helper removes them
	// itself once this process exits.
	if clearAliases != nil {
		clearAliases()
	}

	// Final guarantee that quitting really released every forwarded port: any
	// port still held here (e.g. by a forwarder that escaped the tree kill) is
	// force-freed, and whatever survives even that is reported loudly.
	if busy := manager.VerifyPortsReleased(localPorts); len(busy) > 0 {
		fmt.Printf("Warning: port(s) still in use after shutdown: %s\n", strings.Join(busy, ", "))
		fmt.Println("Run 'pf cleanup' to force-free them.")
	}

	if runErr != nil {
		fmt.Printf("Error: %v\n", runErr)
		os.Exit(1)
	}
}

// startAliasReconciler keeps the system hosts file in sync with the set of
// currently-running services while the TUI is up. For each running service it
// derives the in-cluster FQDN (plus any custom aliases pinned by local port) and
// adds a `127.0.0.1 <fqdn>` entry — so services started at launch AND those
// added later from the manage overlay ('a') are all reachable by their cluster
// hostname on their local port.
//
// It reconciles on a short timer, re-applying only when the running set changes.
// When the hosts file isn't directly writable it launches one hidden, elevated
// watcher (a single UAC prompt) that applies a control file this process keeps
// updated, so dynamic adds/removes never trigger another prompt.
//
// Everything is best-effort: if aliasing is off, elevation is declined, or a
// command yields no FQDN, the forwards run normally regardless. The returned
// cleanup removes the aliases on shutdown (nil when aliasing is inactive).
func startAliasReconciler(ctx context.Context, st *storage.Storage, mgr *manager.ServiceManager) func() {
	if on, _ := st.HostAliasEnabled(); !on {
		return nil
	}
	custom, _ := st.CustomHostAliases()

	// Write directly when we already hold write access (elevated / root);
	// otherwise drive a hidden elevated watcher through a control file.
	direct := hostsfile.Writable()
	var controlFile string
	if !direct {
		f, err := os.CreateTemp("", "pf-hosts-*.txt")
		if err != nil {
			fmt.Printf("⚠ Cluster aliases skipped: %v\n", err)
			return nil
		}
		controlFile = f.Name()
		f.Close()
		if !elevateHostsWatch(controlFile) {
			os.Remove(controlFile)
			fmt.Println("⚠ Cluster aliases skipped (no elevation) — services still work on localhost:<port>.")
			fmt.Println("  Disable this with 'pf alias off'.")
			return nil
		}
	}

	// writeControl atomically publishes the desired host list to the watcher's
	// control file (temp + rename) so it never observes a half-written line.
	writeControl := func(data string) {
		tmp := controlFile + ".tmp"
		if os.WriteFile(tmp, []byte(data), 0o644) == nil {
			_ = os.Rename(tmp, controlFile)
		}
	}

	stop := make(chan struct{})
	noted := make(map[string]bool)
	lastKey := ""

	reconcile := func() {
		type aliasEntry struct {
			name, port string
			hosts      []string
		}
		var entries []aliasEntry
		var all []string
		seen := make(map[string]bool)
		live := make(map[string]bool)

		for _, s := range mgr.ListServiceStates() {
			localPort, _ := storage.ParsePortsFromCommand(s.Command)
			var hosts []string
			if fqdn, ok := storage.ClusterHostFromCommand(s.Command); ok {
				hosts = append(hosts, fqdn)
			}
			if localPort != "" {
				for _, h := range custom[localPort] {
					if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
						hosts = append(hosts, h)
					}
				}
			}
			if len(hosts) == 0 {
				continue
			}
			live[s.Name] = true
			entries = append(entries, aliasEntry{name: s.Name, port: localPort, hosts: hosts})
			for _, h := range hosts {
				if !seen[h] {
					seen[h] = true
					all = append(all, h)
				}
			}
		}

		// Forget services that stopped, so a later re-add re-announces its alias.
		for name := range noted {
			if !live[name] {
				delete(noted, name)
			}
		}

		sort.Strings(all)
		if key := strings.Join(all, "\n"); key != lastKey {
			if direct {
				_ = hostsfile.Apply(all)
			} else {
				writeControl(key)
			}
			lastKey = key
		}

		// Announce each newly-aliased service once, in its log pane.
		for _, e := range entries {
			if !noted[e.name] {
				noted[e.name] = true
				mgr.NoteAlias(e.name, e.hosts, e.port)
			}
		}
	}

	go func() {
		ticker := time.NewTicker(700 * time.Millisecond)
		defer ticker.Stop()
		reconcile()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				reconcile()
			}
		}
	}()

	return func() {
		close(stop)
		if direct {
			_ = hostsfile.Clear()
		} else {
			// Blank the control file so the watcher strips the block promptly; it
			// also clears and exits on its own once this process exits.
			writeControl("")
		}
	}
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
