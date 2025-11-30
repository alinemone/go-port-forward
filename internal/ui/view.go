package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// View renders the UI.
func (m *Model) View() string {
	if m.quitting {
		return "Shutting down...\n"
	}

	var sections []string

	// Banner
	sections = append(sections, m.renderBanner())
	sections = append(sections, "")

	// Services table
	sections = append(sections, m.renderServicesTable())
	sections = append(sections, "")

	// Errors
	if len(m.errors) > 0 {
		sections = append(sections, m.renderErrors())
		sections = append(sections, "")
	}

	// Help
	sections = append(sections, m.renderHelp())

	return strings.Join(sections, "\n")
}

func (m *Model) renderBanner() string {
	banner := m.styles.banner.Render("Port Forward Manager v2.0")
	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(banner)
}

func (m *Model) renderServicesTable() string {
	if len(m.services) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim)).Render("  No services running...")
	}

	// Table header
	header := fmt.Sprintf("%-20s %-17s %-18s %-10s",
		m.styles.tableHeader.Render("Service"),
		m.styles.tableHeader.Render("Status"),
		m.styles.tableHeader.Render("Ports"),
		m.styles.tableHeader.Render("Uptime"),
	)

	var rows []string
	rows = append(rows, header)
	rows = append(rows, strings.Repeat("─", 70))

	// Table rows
	now := time.Now()
	for _, svc := range m.services {
		icon := GetStatusIcon(string(svc.Status))
		statusStyle := m.styles.GetStatusStyle(string(svc.Status))
		statusText := statusStyle.Render(fmt.Sprintf("%s %s", icon, svc.Status))

		ports := fmt.Sprintf("%s → %s", svc.LocalPort, svc.RemotePort)

		uptime := "--:--:--"
		if svc.Status == "ONLINE" && !svc.OnlineTime.IsZero() {
			duration := now.Sub(svc.OnlineTime)
			uptime = formatDuration(duration)
		}

		// Truncate service name if too long
		name := svc.Name
		if len(name) > 20 {
			name = name[:17] + "..."
		}

		row := fmt.Sprintf("%-20s %-17s %-18s %-10s",
			name,
			statusText,
			ports,
			uptime,
		)

		rows = append(rows, row)
	}

	table := strings.Join(rows, "\n")
	return lipgloss.NewStyle().
		Padding(0, 2).
		Render(table)
}

func (m *Model) renderErrors() string {
	title := m.styles.errorMsg.Bold(true).Render("Recent Errors:")

	var errorLines []string
	errorLines = append(errorLines, title)

	now := time.Now()
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim))
	primaryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorPrimary))

	for _, err := range m.errors {
		timeSince := now.Sub(err.Timestamp)
		timeStr := err.Timestamp.Format("15:04:05")

		errLine := fmt.Sprintf("  [%s] %s: %s (%s ago)",
			dimStyle.Render(timeStr),
			primaryStyle.Render(err.Service),
			m.styles.errorMsg.Render(err.Message),
			dimStyle.Render(formatDuration(timeSince)),
		)

		errorLines = append(errorLines, errLine)
	}

	return strings.Join(errorLines, "\n")
}

func (m *Model) renderHelp() string {
	help := m.styles.helpText.Render("Press 'q' to quit | 'r' to refresh | Ctrl+C to stop")
	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(help)
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}
