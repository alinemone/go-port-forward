package main

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/theme"
)

// CLI styling. Colors come from the active palette (internal/theme) so the
// command-line output matches the TUI exactly. The styles are rebuilt by
// applyCLITheme (init seeds the default; main refreshes after loading the saved
// theme). lipgloss drops color automatically when piped to a non-TTY / NO_COLOR.
var (
	cliHeading lipgloss.Style
	cliCount   lipgloss.Style
	cliIndex   lipgloss.Style
	cliName    lipgloss.Style
	cliArrow   lipgloss.Style
	cliDetail  lipgloss.Style
	cliMuted   lipgloss.Style
	cliTitle   lipgloss.Style
	cliBorder  lipgloss.Style
)

func init() { applyCLITheme() }

// applyCLITheme rebuilds the CLI styles from theme.Active.
func applyCLITheme() {
	p := theme.Active
	cliHeading = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.Heading))
	cliCount = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Muted)).Italic(true)
	cliIndex = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Muted))
	cliName = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.Accent))
	cliArrow = lipgloss.NewStyle().Foreground(lipgloss.Color(p.AccentAlt))
	cliDetail = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Text))
	cliMuted = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Muted))
	cliTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.Accent))
	cliBorder = lipgloss.NewStyle().Foreground(lipgloss.Color(p.Border))
}

// printList renders a titled, numbered list with a consistent two-line item
// layout: "  N. <title>" then "     → <detail>". headingMeta is an optional
// dim suffix on the heading (e.g. a count).
func printList(heading, headingMeta string, items [][2]string) {
	lipgloss.Println()
	line := cliHeading.Render(heading)
	if headingMeta != "" {
		line += cliCount.Render("  " + headingMeta)
	}
	lipgloss.Println(line)
	lipgloss.Println()
	for i, it := range items {
		lipgloss.Printf("  %s %s\n", cliIndex.Render(fmt.Sprintf("%2d.", i+1)), cliName.Render(it[0]))
		lipgloss.Printf("     %s %s\n", cliArrow.Render("→"), cliDetail.Render(it[1]))
	}
	lipgloss.Println()
}
