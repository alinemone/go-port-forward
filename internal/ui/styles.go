package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// Fixed color scheme
const (
	colorPrimary   = "#00D9FF" // Cyan
	colorSecondary = "#00A8CC"
	colorSuccess   = "#00FF88"
	colorWarning   = "#FFD700"
	colorError     = "#FF6B6B"
	colorText      = "#FFFFFF"
	colorTextDim   = "#666666"
	colorBorder    = "#444444"

	colorStatusOnline       = "#00FF88" // Green
	colorStatusConnecting   = "#FFD700" // Yellow
	colorStatusReconnecting = "#FF8C00" // Orange
	colorStatusError        = "#FF6B6B" // Red
)

// Styles holds all UI styles.
type Styles struct {
	// Component styles
	banner             lipgloss.Style
	title              lipgloss.Style
	table              lipgloss.Style
	tableHeader        lipgloss.Style
	statusOnline       lipgloss.Style
	statusConnecting   lipgloss.Style
	statusReconnecting lipgloss.Style
	statusError        lipgloss.Style
	errorMsg           lipgloss.Style
	helpText           lipgloss.Style
}

// NewStyles creates styles with fixed colors.
func NewStyles() *Styles {
	s := &Styles{}

	// Create component styles
	s.banner = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorPrimary)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorPrimary)).
		Padding(0, 2)

	s.title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorPrimary))

	s.table = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorBorder)).
		Padding(0, 1)

	s.tableHeader = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorText))

	s.statusOnline = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorStatusOnline)).
		Bold(true)

	s.statusConnecting = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorStatusConnecting)).
		Bold(true)

	s.statusReconnecting = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorStatusReconnecting)).
		Bold(true)

	s.statusError = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorStatusError)).
		Bold(true)

	s.errorMsg = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorError))

	s.helpText = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorTextDim))

	return s
}

// GetStatusStyle returns the appropriate style for a status.
func (s *Styles) GetStatusStyle(status string) lipgloss.Style {
	switch status {
	case "ONLINE":
		return s.statusOnline
	case "CONNECTING":
		return s.statusConnecting
	case "RECONNECTING":
		return s.statusReconnecting
	case "ERROR":
		return s.statusError
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim))
	}
}

// GetStatusIcon returns the icon for a status.
func GetStatusIcon(status string) string {
	switch status {
	case "ONLINE":
		return "●"
	case "CONNECTING":
		return "◐"
	case "RECONNECTING":
		return "○"
	case "ERROR":
		return "✗"
	default:
		return "•"
	}
}
