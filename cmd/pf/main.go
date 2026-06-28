package main

import (
	"os"
	"strings"

	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/stringutil"
	"github.com/alinemone/go-port-forward/internal/theme"
	"github.com/alinemone/go-port-forward/internal/ui"
	"github.com/alinemone/go-port-forward/internal/updater"

	"charm.land/lipgloss/v2"
)

func main() {
	updater.CleanupStaleArtifacts()

	storage.NewStorage().EnsureExists()

	// Load the saved color theme and apply it process-wide before any rendering.
	if name, err := storage.NewStorage().ThemeName(); err == nil {
		theme.Set(name)
		applyCLITheme()
		ui.ApplyTheme()
	}

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
	case "theme", "themes":
		runThemeCommand(args)
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
