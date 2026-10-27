package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/ui"
)

func newDiscoverCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "discover [namespace]",
		Aliases: []string{"scan"},
		Short:   "Find a namespace's databases & caches and save them as services",
		Args:    cobra.MaximumNArgs(1),
		Run: func(_ *cobra.Command, args []string) {
			namespace := ""
			if len(args) == 1 {
				namespace = strings.TrimSpace(args[0])
			}
			runDiscoverCommand(namespace)
		},
	}
}

func runDiscoverCommand(namespace string) {
	added, err := ui.RunDiscover(storage.NewStorage(), namespace)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	if len(added) == 0 {
		fmt.Println("Nothing added.")
		return
	}
	fmt.Printf("✓ Added %d service(s): %s\n", len(added), strings.Join(added, ", "))
	fmt.Printf("  Run them with: pf run %s\n", strings.Join(added, ","))
}
