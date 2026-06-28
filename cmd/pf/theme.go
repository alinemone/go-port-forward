package main

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/theme"
	"github.com/alinemone/go-port-forward/internal/ui"
)

// runThemeCommand handles `pf theme [name|list]`: with no arg (or "list") it
// shows the available themes and the active one; with a name it persists and
// applies the choice.
func runThemeCommand(args []string) {
	st := storage.NewStorage()

	action := ""
	if len(args) > 0 {
		action = strings.ToLower(strings.TrimSpace(args[0]))
	}
	if action == "" || action == "list" || action == "status" {
		showThemes(st)
		return
	}

	if !theme.Exists(action) {
		lipgloss.Println(cliMuted.Render("Unknown theme: " + action))
		showThemes(st)
		os.Exit(1)
	}

	if err := st.SetTheme(action); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	// Apply immediately so the confirmation prints in the newly chosen theme.
	theme.Set(action)
	applyCLITheme()
	ui.ApplyTheme()

	lipgloss.Println(cliName.Render("✓ Theme: ") + cliTitle.Render(action) + "  " + swatch(action))
}

// showThemes lists every theme with a color swatch, marking the active one.
func showThemes(st *storage.Storage) {
	current, _ := st.ThemeName()
	if current == "" {
		current = "default"
	}

	lipgloss.Println()
	lipgloss.Println("  " + cliArrow.Render("▸") + " " + cliHeading.Render("THEMES"))
	for _, name := range theme.Names() {
		marker := "  "
		if name == current {
			marker = cliArrow.Render("►") + " "
		}
		lipgloss.Println("    " + marker + cliName.Render(fmt.Sprintf("%-9s", name)) + "  " + swatch(name))
	}
	lipgloss.Println()
	lipgloss.Println("  " + cliMuted.Render("Switch with: pf theme <name>"))
	lipgloss.Println()
}

// swatch renders small colored blocks previewing a palette's key colors.
func swatch(name string) string {
	p, ok := theme.Get(name)
	if !ok {
		return ""
	}
	const block = "███"
	cells := []string{p.Accent, p.AccentAlt, p.Warn, p.Error, p.Heading}
	parts := make([]string, 0, len(cells))
	for _, hex := range cells {
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render(block))
	}
	return strings.Join(parts, " ")
}
