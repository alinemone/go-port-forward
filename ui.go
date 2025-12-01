package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tickMsg time.Time

// UI model
type UI struct {
	manager  *Manager
	services []Service
	quitting bool
	width    int
	height   int
}

// NewUI creates a new UI
func NewUI(manager *Manager) *UI {
	return &UI{
		manager:  manager,
		services: []Service{},
	}
}

// Init initializes the UI
func (u *UI) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		tea.EnterAltScreen,
	)
}

// Update handles messages
func (u *UI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		u.width = msg.Width
		u.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			u.quitting = true
			u.manager.StopAll()
			return u, tea.Quit
		}

	case tickMsg:
		u.services = u.manager.GetStates()
		return u, tickCmd()
	}

	return u, nil
}

// View renders the UI
func (u *UI) View() string {
	if u.quitting {
		return renderShutdown()
	}

	var sections []string

	// Header
	sections = append(sections, renderHeader())

	// Services Table
	if len(u.services) == 0 {
		sections = append(sections, renderEmpty())
	} else {
		sections = append(sections, renderServicesTable(u.services))
	}

	// Error section
	errorSection := renderErrors(u.services)
	if errorSection != "" {
		sections = append(sections, errorSection)
	}

	// Footer
	sections = append(sections, renderFooter())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// Glass styles
var (
	glassHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#E0F7FF")).
			Background(lipgloss.Color("#0A2540")).
			Padding(1, 4).
			MarginBottom(1).
			Width(100).
			Align(lipgloss.Center).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#4DD4FF"))

	glassTable = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#4A90E2")).
			Background(lipgloss.Color("#0D1B2A")).
			Padding(1, 2).
			MarginBottom(1).
			Width(100)

	tableHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#4DD4FF")).
				Background(lipgloss.Color("#0D1B2A")).
				Bold(true)

	rowHealthy = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6FFFB0")).
			Background(lipgloss.Color("#0D1B2A"))

	rowConnecting = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFE66D")).
			Background(lipgloss.Color("#0D1B2A"))

	rowError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B6B")).
			Background(lipgloss.Color("#0D1B2A"))

	cellName = lipgloss.NewStyle().
			Width(20).
			Background(lipgloss.Color("#0D1B2A")).
			Bold(true)

	cellStatus = lipgloss.NewStyle().
			Width(15).
			Background(lipgloss.Color("#0D1B2A")).
			Bold(true)

	cellPort = lipgloss.NewStyle().
			Width(25).
			Background(lipgloss.Color("#0D1B2A")).
			Foreground(lipgloss.Color("#8B9BAA"))

	glassErrorBox = lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(lipgloss.Color("#FF6B6B")).
			Background(lipgloss.Color("#1A0A0A")).
			Padding(1, 2).
			MarginTop(1).
			MarginBottom(1).
			Width(100)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4A90E2")).
			MarginTop(1).
			Italic(true)
)

// renderHeader renders glassmorphic header
func renderHeader() string {
	title := "✦ PORT FORWARD MANAGER ✦"
	return glassHeader.Render(title)
}

// renderEmpty renders empty state
func renderEmpty() string {
	emptyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B8E99")).
		Italic(true).
		MarginLeft(2).
		MarginTop(1).
		MarginBottom(1)
	return emptyStyle.Render("⚬ No services running...")
}

// renderServicesTable renders services as a table
func renderServicesTable(services []Service) string {
	var rows []string

	// Table header
	header := lipgloss.JoinHorizontal(
		lipgloss.Top,
		tableHeaderStyle.Render(cellStatus.Render("STATUS")),
		tableHeaderStyle.Render(cellName.Render("SERVICE")),
		tableHeaderStyle.Render(cellPort.Render("PORT")),
	)
	rows = append(rows, header)

	// Separator
	separator := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#4A90E2")).
		Background(lipgloss.Color("#0D1B2A")).
		Render(strings.Repeat("─", 60))
	rows = append(rows, separator)

	// Service rows
	for _, svc := range services {
		rows = append(rows, renderServiceRow(svc))
	}

	table := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return glassTable.Render(table)
}

// renderServiceRow renders a single service row
func renderServiceRow(svc Service) string {
	var rowStyle lipgloss.Style
	var statusIcon, statusText string

	switch svc.Status {
	case StatusHealthy:
		rowStyle = rowHealthy
		statusIcon = "●"
		statusText = "HEALTHY"
	case StatusConnecting:
		rowStyle = rowConnecting
		statusIcon = "◐"
		statusText = "CONNECTING"
	case StatusError:
		rowStyle = rowError
		statusIcon = "✗"
		statusText = "ERROR"
	}

	// Format cells
	statusCell := rowStyle.Render(cellStatus.Render(fmt.Sprintf("%s %s", statusIcon, statusText)))
	nameCell := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#4DD4FF")).
		Background(lipgloss.Color("#0D1B2A")).
		Bold(true).
		Render(cellName.Render(svc.Name))
	portCell := cellPort.Render(fmt.Sprintf("%s → %s", svc.LocalPort, svc.RemotePort))

	return lipgloss.JoinHorizontal(lipgloss.Top, statusCell, nameCell, portCell)
}

// renderErrors renders the error section
func renderErrors(services []Service) string {
	var errors []string

	for _, svc := range services {
		if svc.Error != "" {
			errorLine := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF6B6B")).
				Bold(true).
				Render(fmt.Sprintf("⚠ %s", svc.Name))

			errorMsg := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFB4B4")).
				Render(fmt.Sprintf("  └─ %s", svc.Error))

			errors = append(errors, errorLine+"\n"+errorMsg)
		}
	}

	if len(errors) == 0 {
		return ""
	}

	var content strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF6B6B")).
		Bold(true).
		Underline(true)

	content.WriteString(titleStyle.Render("ERROR LOG"))
	content.WriteString("\n\n")

	for _, err := range errors {
		content.WriteString(err)
		content.WriteString("\n")
	}

	return glassErrorBox.Render(content.String())
}

// renderFooter renders the footer
func renderFooter() string {
	return footerStyle.Render("◆ Press 'q' or Ctrl+C to quit ◆")
}

// renderShutdown renders shutdown message
func renderShutdown() string {
	shutdownStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6FFFB0")).
		Bold(true).
		MarginTop(2).
		MarginBottom(2)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6FFFB0")).
		Padding(1, 4).
		Align(lipgloss.Center)

	return box.Render(shutdownStyle.Render("✓ Shutting down gracefully..."))
}

// tickCmd returns a tick command
func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
