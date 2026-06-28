package main

import (
	"os"

	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/theme"
	"github.com/alinemone/go-port-forward/internal/ui"
	"github.com/alinemone/go-port-forward/internal/updater"
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

	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
