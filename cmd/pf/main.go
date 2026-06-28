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

	// Register any user-defined palettes, then load the saved color theme and
	// apply it process-wide before any rendering. Registration must come first so
	// a saved custom theme name resolves just like a built-in.
	st := storage.NewStorage()
	_ = st.RegisterCustomThemes()
	if name, err := st.ThemeName(); err == nil {
		theme.Set(name)
		applyCLITheme()
		ui.ApplyTheme()
	}

	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
