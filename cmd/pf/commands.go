package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/cert"
	"github.com/alinemone/go-port-forward/internal/storage"
)

// newRootCmd builds the full Cobra command tree. Each command is a thin wrapper
// around an existing run* function (business logic is unchanged); Cobra adds
// argument routing plus the `completion` / hidden `__complete` commands that
// power Tab-completion on bash, zsh, fish and PowerShell.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "pf",
		Short:         "Port Forward Manager",
		Args:          cobra.ArbitraryArgs,
		SilenceErrors: true, // our handlers print + exit themselves
		SilenceUsage:  true,
		// Bare `pf <service|group>` is a shortcut for `pf run <…>`. Cobra calls
		// this Run only when the first arg isn't a known subcommand.
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				showUsage()
				return
			}
			if looksLikeRunTarget(storage.NewStorage(), strings.Join(args, " ")) {
				runStartCommand(args)
				return
			}
			lipgloss.Println(cliMuted.Render("Unknown command: " + args[0]))
			lipgloss.Println(cliMuted.Render("Run 'pf help' for usage"))
			os.Exit(1)
		},
		ValidArgsFunction: completeServicesAndGroups,
	}
	// Preserve our themed help for `pf`, `pf -h`, and `pf help`.
	root.SetHelpFunc(func(*cobra.Command, []string) { showUsage() })

	// Replace Cobra's default `completion` command with ours (which adds
	// `install`); the hidden `__complete` that powers Tab stays registered.
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(
		newAddCmd(), newListCmd(), newRunCmd(), newRaCmd(), newDeleteCmd(),
		newRenameCmd(), newKubectlCmd(), newCleanupCmd(), newUpdateCmd(),
		newEditCmd(), newIconCmd(), newThemeCmd(), newVersionCmd(),
		newGroupCmd(), newCertCmd(), newCompletionCmd(),
	)
	return root
}

func newAddCmd() *cobra.Command {
	return &cobra.Command{
		Use: "add", Aliases: []string{"a"}, Short: "Save a new service",
		Args: cobra.ArbitraryArgs,
		Run:  func(_ *cobra.Command, args []string) { runAddCommand(args) },
	}
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Aliases: []string{"l"}, Short: "List saved services",
		Run: func(_ *cobra.Command, _ []string) { runListCommand() },
	}
}

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use: "run", Aliases: []string{"r"}, Short: "Run services/groups in the live TUI",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeServicesAndGroups,
		Run:               func(_ *cobra.Command, args []string) { runStartCommand(args) },
	}
}

func newRaCmd() *cobra.Command {
	return &cobra.Command{
		Use: "ra", Short: "Run every saved service",
		Run: func(_ *cobra.Command, _ []string) { runStartCommand([]string{"all"}) },
	}
}

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use: "delete", Aliases: []string{"d", "rm"}, Short: "Delete a service",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeServices,
		Run:               func(_ *cobra.Command, args []string) { runDeleteCommand(args) },
	}
}

func newRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use: "rename", Aliases: []string{"ren", "mv"}, Short: "Rename a service or group",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeServicesAndGroups,
		Run:               func(_ *cobra.Command, args []string) { runRenameCommand(args) },
	}
}

func newKubectlCmd() *cobra.Command {
	return &cobra.Command{
		Use: "kubectl", Aliases: []string{"k"}, Short: "Run kubectl with the configured certificate",
		DisableFlagParsing: true, // pass -n/--context etc. straight to kubectl
		Run:                func(_ *cobra.Command, args []string) { runKubectlCommand(args) },
	}
}

func newCleanupCmd() *cobra.Command {
	return &cobra.Command{
		Use: "cleanup", Aliases: []string{"c"}, Short: "Free configured ports",
		DisableFlagParsing: true, // the handler parses --all / -y itself
		Run:                func(_ *cobra.Command, args []string) { runCleanupCommand(args) },
	}
}

func newUpdateCmd() *cobra.Command {
	c := &cobra.Command{
		Use: "update", Aliases: []string{"u"}, Short: "Update pf to the latest release",
		DisableFlagParsing: true, // the handler parses --yes / --force itself
		Run:                func(_ *cobra.Command, args []string) { runUpdateCommand(args) },
	}
	c.SetHelpFunc(func(*cobra.Command, []string) { showUpdateUsage() })
	return c
}

func newEditCmd() *cobra.Command {
	return &cobra.Command{
		Use: "edit", Aliases: []string{"config"}, Short: "Bulk-edit services & groups in $EDITOR",
		Run: func(_ *cobra.Command, _ []string) { runEditCommand() },
	}
}

func newIconCmd() *cobra.Command {
	return &cobra.Command{
		Use: "icon", Aliases: []string{"icons"}, Short: "Toggle Nerd Font icons",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeIconArgs,
		Run:               func(_ *cobra.Command, args []string) { runIconCommand(args) },
	}
}

func newThemeCmd() *cobra.Command {
	return &cobra.Command{
		Use: "theme", Aliases: []string{"themes"}, Short: "Switch the color theme",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: completeThemes,
		Run:               func(_ *cobra.Command, args []string) { runThemeCommand(args) },
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use: "version", Aliases: []string{"v"}, Short: "Show build version details",
		Run: func(_ *cobra.Command, _ []string) { runVersionCommand() },
	}
}

// --- group -----------------------------------------------------------------

func newGroupCmd() *cobra.Command {
	g := &cobra.Command{
		Use: "group", Aliases: []string{"g"}, Short: "Manage groups of services",
		Args: cobra.ArbitraryArgs,
		Run: func(_ *cobra.Command, args []string) {
			if len(args) > 0 {
				fmt.Printf("Unknown group command: %s\n", args[0])
			}
			showGroupUsage()
			os.Exit(1)
		},
	}
	g.SetHelpFunc(func(*cobra.Command, []string) { showGroupUsage() })

	g.AddCommand(
		&cobra.Command{
			Use: "add", Aliases: []string{"a"}, Short: "Create a group",
			Args:              cobra.ArbitraryArgs,
			ValidArgsFunction: completeServiceList,
			Run:               func(_ *cobra.Command, args []string) { runGroupAddCommand(storage.NewStorage(), args) },
		},
		&cobra.Command{
			Use: "add-service", Aliases: []string{"addsvc", "as"}, Short: "Add services to a group",
			Args:              cobra.ArbitraryArgs,
			ValidArgsFunction: completeGroupThenServices,
			Run:               func(_ *cobra.Command, args []string) { runGroupAddServiceCommand(storage.NewStorage(), args) },
		},
		&cobra.Command{
			Use: "remove-service", Aliases: []string{"rmsvc", "rs"}, Short: "Remove services from a group",
			Args:              cobra.ArbitraryArgs,
			ValidArgsFunction: completeGroupThenServices,
			Run:               func(_ *cobra.Command, args []string) { runGroupRemoveServiceCommand(storage.NewStorage(), args) },
		},
		&cobra.Command{
			Use: "list", Aliases: []string{"ls", "l"}, Short: "List groups",
			Run: func(_ *cobra.Command, _ []string) { runGroupListCommand(storage.NewStorage()) },
		},
		&cobra.Command{
			Use: "delete", Aliases: []string{"rm", "d"}, Short: "Delete a group",
			Args:              cobra.ArbitraryArgs,
			ValidArgsFunction: completeGroups,
			Run:               func(_ *cobra.Command, args []string) { runGroupDeleteCommand(storage.NewStorage(), args) },
		},
		&cobra.Command{
			Use: "rename", Aliases: []string{"ren", "mv"}, Short: "Rename a group",
			Args:              cobra.ArbitraryArgs,
			ValidArgsFunction: completeGroups,
			Run:               func(_ *cobra.Command, args []string) { runGroupRenameCommand(storage.NewStorage(), args) },
		},
	)
	return g
}

// --- cert ------------------------------------------------------------------

func mustCertManager() *cert.Manager {
	m, err := cert.NewManager()
	if err != nil {
		fmt.Printf("Error: Failed to initialize certificate manager: %v\n", err)
		os.Exit(1)
	}
	return m
}

func newCertCmd() *cobra.Command {
	c := &cobra.Command{
		Use: "cert", Short: "Manage the kubectl client certificate",
		Args: cobra.ArbitraryArgs,
		Run: func(_ *cobra.Command, args []string) {
			if len(args) > 0 {
				fmt.Printf("Unknown cert command: %s\n", args[0])
			}
			showCertUsage()
			os.Exit(1)
		},
	}
	c.SetHelpFunc(func(*cobra.Command, []string) { showCertUsage() })

	c.AddCommand(
		&cobra.Command{
			Use: "add", Short: "Add a certificate for all kubectl services",
			Args: cobra.ArbitraryArgs,
			Run:  func(_ *cobra.Command, args []string) { runCertAddCommand(mustCertManager(), args) },
		},
		&cobra.Command{
			Use: "list", Aliases: []string{"ls"}, Short: "Show the configured certificate",
			Run: func(_ *cobra.Command, _ []string) { runCertListCommand(mustCertManager()) },
		},
		&cobra.Command{
			Use: "remove", Aliases: []string{"rm", "delete"}, Short: "Remove the certificate",
			Run: func(_ *cobra.Command, _ []string) { runCertRemoveCommand(mustCertManager()) },
		},
	)
	return c
}
