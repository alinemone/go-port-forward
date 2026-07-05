package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/theme"
)

// by ApplyTheme so the user's theme choice takes effect process-wide; the TUI
// builds its styles per-render from these vars, so no caching gets stale.
var (
	colorText      color.Color
	colorMuted     color.Color
	colorBorder    color.Color
	colorAccent    color.Color
	colorAccentAlt color.Color
	colorWarn      color.Color
	colorError     color.Color
	colorHeading   color.Color
	colorSelected  color.Color
)

// Service-health colors are fixed (never themed) so HEALTHY always reads green,
// CONNECTING yellow, and ERROR red regardless of the active palette.
var (
	statusHealthyColor    = lipgloss.Color(theme.StatusHealthy)
	statusConnectingColor = lipgloss.Color(theme.StatusConnecting)
	statusErrorColor      = lipgloss.Color(theme.StatusError)
)

func init() { ApplyTheme() }

// ApplyTheme refreshes the package-level chrome colors from theme.Active. Call
// it once at startup after selecting the theme (init seeds the default).
func ApplyTheme() {
	p := theme.Active
	colorText = lipgloss.Color(p.Text)
	colorMuted = lipgloss.Color(p.Muted)
	colorBorder = lipgloss.Color(p.Border)
	colorAccent = lipgloss.Color(p.Accent)
	colorAccentAlt = lipgloss.Color(p.AccentAlt)
	colorWarn = lipgloss.Color(p.Warn)
	colorError = lipgloss.Color(p.Error)
	colorHeading = lipgloss.Color(p.Heading)
	colorSelected = lipgloss.Color(p.Selected)
}
