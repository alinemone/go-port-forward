package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alinemone/go-port-forward/internal/hostsfile"
	"github.com/alinemone/go-port-forward/internal/storage"
)

func runAliasCommand(args []string) {
	st := storage.NewStorage()

	action := ""
	if len(args) > 0 {
		action = strings.ToLower(strings.TrimSpace(args[0]))
	}

	switch action {
	case "", "status":
		enabled, err := st.HostAliasEnabled()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		printAliasStatus(enabled)
	case "on", "enable", "true":
		setAliasEnabled(st, true)
	case "off", "disable", "false":
		setAliasEnabled(st, false)
	case "clear":
		clearHostAliases()
	default:
		fmt.Printf("Unknown option: %s\n", action)
		fmt.Println("Usage: pf alias [on|off|status|clear]")
		os.Exit(1)
	}
}

func setAliasEnabled(st *storage.Storage, enabled bool) {
	if err := st.SetHostAliasEnabled(enabled); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	printAliasStatus(enabled)
	if enabled {
		fmt.Println("Note: editing the system hosts file needs Administrator/root. pf prompts for")
		fmt.Println("elevation automatically when you run a service; the port-forward works either way.")
	}
}

func printAliasStatus(enabled bool) {
	if enabled {
		fmt.Println("✓ Cluster-host aliases: ON")
	} else {
		fmt.Println("○ Cluster-host aliases: OFF")
	}
}

// clearHostAliases strips the pf-managed block from the hosts file, self-elevating
// (hidden, one UAC prompt) if the current process can't write it.
func clearHostAliases() {
	if err := hostsfile.Clear(); err == nil {
		fmt.Println("✓ Removed pf-managed aliases from the hosts file.")
		return
	}
	if elevateHostsClear() {
		fmt.Println("✓ Removed pf-managed aliases from the hosts file.")
		return
	}
	fmt.Println("⚠ Couldn't edit the hosts file (needs Administrator) or elevation was declined.")
	os.Exit(1)
}

func completeAliasArgs(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{"on", "off", "status", "clear"}, cobra.ShellCompDirectiveNoFileComp
}
