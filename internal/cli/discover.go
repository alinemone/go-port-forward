package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alinemone/go-port-forward/internal/kube"
	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/ui"
)

func newDiscoverCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "discover [namespace]",
		Aliases: []string{"d", "scan"},
		Short:   "Find a namespace's databases & caches and save them as services",
		Args:    cobra.MaximumNArgs(1),
		// Tab completes the namespaces your saved services already use.
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return savedNamespaces(storage.NewStorage()), cobra.ShellCompDirectiveNoFileComp
		},
		Run: func(_ *cobra.Command, args []string) {
			namespace := ""
			if len(args) == 1 {
				namespace = strings.TrimSpace(args[0])
			}
			runDiscoverCommand(namespace)
		},
	}
}

// savedNamespaces lists the namespaces of the saved kubectl port-forwards.
func savedNamespaces(st *storage.Storage) []string {
	services, err := st.LoadServices()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, cmd := range services {
		if spec, err := kube.ParseCommand(cmd); err == nil && spec.Namespace != "" && !seen[spec.Namespace] {
			seen[spec.Namespace] = true
			out = append(out, spec.Namespace)
		}
	}
	sort.Strings(out)
	return out
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
