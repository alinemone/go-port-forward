package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tickMsg time.Time

// UI model
type UI struct {
	manager       *Manager
	services      []Service
	selectedIndex int
	quitting      bool
	width         int
	height        int
	viewport      viewport.Model
	ready         bool
	ctx           context.Context
}

// NewUI creates a new UI
func NewUI(manager *Manager, ctx context.Context) *UI {
	return &UI{
		manager:       manager,
		services:      []Service{},
		selectedIndex: 0,
		ctx:           ctx,
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
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		u.width = msg.Width
		u.height = msg.Height

		// Calculate dynamic viewport height
		// Table: 4 (border + header + separator + bottom border) + number of services
		// Log box border: 2 (top + bottom)
		// Help: 3 (border + text)
		// Total overhead: 9 + number of services
		tableLines := 4 + len(u.services)
		if len(u.services) == 0 {
			tableLines = 1 // Just "No services running..."
		}
		overhead := tableLines + 2 + 3 // table + log border + help
		viewportHeight := msg.Height - overhead
		if viewportHeight < 3 {
			viewportHeight = 3 // Minimum height
		}

		if !u.ready {
			u.viewport = viewport.New(msg.Width, viewportHeight)
			u.viewport.YPosition = 0
			u.ready = true
		} else {
			u.viewport.Width = msg.Width
			u.viewport.Height = viewportHeight
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			u.quitting = true
			u.manager.StopAll()
			return u, tea.Quit

		case "up", "k":
			if u.selectedIndex > 0 {
				u.selectedIndex--
			} else {
				// Pass to viewport for scrolling
				u.viewport, cmd = u.viewport.Update(msg)
			}

		case "down", "j":
			if u.selectedIndex < len(u.services)-1 {
				u.selectedIndex++
			} else {
				// Pass to viewport for scrolling
				u.viewport, cmd = u.viewport.Update(msg)
			}

		case "r":
			// Restart selected service
			if u.selectedIndex < len(u.services) && len(u.services) > 0 {
				serviceName := u.services[u.selectedIndex].Name
				u.manager.Restart(u.ctx, serviceName)
			}

		case "s":
			// Stop selected service
			if u.selectedIndex < len(u.services) && len(u.services) > 0 {
				u.manager.Stop(u.services[u.selectedIndex].Name)
			}

		default:
			// Pass other keys to viewport
			u.viewport, cmd = u.viewport.Update(msg)
		}

	case tickMsg:
		u.services = u.manager.GetStates()
		// Update viewport content
		if u.ready {
			// Recalculate viewport height based on current number of services
			tableLines := 4 + len(u.services)
			if len(u.services) == 0 {
				tableLines = 1
			}
			overhead := tableLines + 2 + 3 // table + log border + help
			viewportHeight := u.height - overhead
			if viewportHeight < 3 {
				viewportHeight = 3
			}
			u.viewport.Height = viewportHeight

			u.viewport.SetContent(renderCombinedLogsContent(u.services))
			u.viewport.GotoBottom()
		}
		// Adjust selected index if needed
		if u.selectedIndex >= len(u.services) && len(u.services) > 0 {
			u.selectedIndex = len(u.services) - 1
		}
		return u, tickCmd()
	}

	return u, cmd
}

// View renders the UI
func (u *UI) View() string {
	if u.quitting {
		return renderShutdown()
	}

	if !u.ready {
		return "Initializing..."
	}

	var sections []string

	// Services Table (compact)
	if len(u.services) == 0 {
		sections = append(sections, renderEmpty())
	} else {
		sections = append(sections, renderCompactServicesTable(u.services, u.selectedIndex, u.width))
	}

	// Scrollable logs with border (full width)
	logBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6FFFB0")).
		Width(u.width - 2).
		Render(u.viewport.View())
	sections = append(sections, logBox)

	// Help always at bottom (full width)
	sections = append(sections, renderCompactHelp(u.width))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// Styles
var (
	compactTable = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#4A90E2")).
			Padding(0, 1)

	helpBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#4DD4FF")).
			Padding(0, 1)
)

// renderEmpty renders empty state
func renderEmpty() string {
	emptyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#808080")).
		Italic(true)
	return emptyStyle.Render("⚬ No services running...")
}

// renderCompactServicesTable renders a compact services table
func renderCompactServicesTable(services []Service, selectedIndex int, width int) string {
	var rows []string

	// Compact header
	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Render("SERVICE       STATUS       UPTIME   RESTARTS")

	rows = append(rows, header)

	// Separator matches terminal width
	sepWidth := width - 6 // subtract border and padding
	if sepWidth < 50 {
		sepWidth = 50
	}
	rows = append(rows, strings.Repeat("─", sepWidth))

	// Service rows
	for i, svc := range services {
		var statusIcon, statusText string
		var statusColor lipgloss.Color

		highlight := "  "
		if i == selectedIndex {
			highlight = "► "
		}

		switch svc.Status {
		case StatusHealthy:
			statusColor = lipgloss.Color("#6FFFB0")
			statusIcon = "●"
			statusText = "HEALTHY"
		case StatusConnecting:
			statusColor = lipgloss.Color("#FFE66D")
			statusIcon = "◐"
			statusText = "CONNECTING"
		case StatusError:
			statusColor = lipgloss.Color("#FF6B6B")
			statusIcon = "✗"
			statusText = "ERROR"
		}

		uptime := formatUptime(svc.StartTime)

		// Build row parts
		name := fmt.Sprintf("%-12s", svc.Name)
		status := fmt.Sprintf("%s %-10s", statusIcon, statusText)
		uptimeStr := fmt.Sprintf("%-8s", uptime)
		restarts := fmt.Sprintf("%d", svc.ReconnectCount)

		// Style each part
		styledName := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E0E0E0")).
			Bold(true).
			Render(name)

		styledStatus := lipgloss.NewStyle().
			Foreground(statusColor).
			Render(status)

		styledUptime := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#C0C0C0")).
			Render(uptimeStr)

		styledRestarts := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#C0C0C0")).
			Render(restarts)

		// Combine
		row := highlight + styledName + " " + styledStatus + " " + styledUptime + " " + styledRestarts

		rows = append(rows, row)
	}

	table := lipgloss.JoinVertical(lipgloss.Left, rows...)

	// Apply border with matching width
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#4A90E2")).
		Padding(0, 1).
		Width(width - 2)

	return style.Render(table)
}

// formatUptime formats duration since start time
func formatUptime(startTime time.Time) string {
	if startTime.IsZero() {
		return "-"
	}

	duration := time.Since(startTime)

	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// renderCombinedLogsContent renders logs content for viewport
func renderCombinedLogsContent(services []Service) string {
	var content strings.Builder

	// Collect all logs with service name
	type LogWithService struct {
		ServiceName string
		Entry       LogEntry
	}

	var allLogs []LogWithService
	for _, svc := range services {
		for _, log := range svc.LogHistory {
			allLogs = append(allLogs, LogWithService{
				ServiceName: svc.Name,
				Entry:       log,
			})
		}
	}

	// Sort by time
	sort.Slice(allLogs, func(i, j int) bool {
		return allLogs[i].Entry.Time.Before(allLogs[j].Entry.Time)
	})

	if len(allLogs) == 0 {
		content.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("#808080")).
			Italic(true).
			Render("No logs yet..."))
	} else {
		for i := 0; i < len(allLogs); i++ {
			log := allLogs[i]
			timestamp := log.Entry.Time.Format("15:04:05")

			// Service name (max 8 chars)
			serviceName := log.ServiceName
			if len(serviceName) > 8 {
				serviceName = serviceName[:8]
			}

			// Message style based on error or info
			var msgColor lipgloss.Color
			if log.Entry.IsError {
				msgColor = lipgloss.Color("#FF6B6B")
			} else if strings.Contains(log.Entry.Message, "━━━━") {
				msgColor = lipgloss.Color("#FFE66D")
			} else {
				msgColor = lipgloss.Color("#E0E0E0")
			}

			// Format: [service time] message
			nameStyled := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#4DD4FF")).
				Bold(true).
				Render(fmt.Sprintf("%-8s", serviceName))

			timeStyled := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#808080")).
				Render(timestamp)

			msgStyled := lipgloss.NewStyle().
				Foreground(msgColor).
				Render(log.Entry.Message)

			logLine := fmt.Sprintf("[%s %s] %s", nameStyled, timeStyled, msgStyled)

			content.WriteString(logLine)
			content.WriteString("\n")
		}
	}

	return content.String()
}

// renderCompactHelp renders compact help at bottom
func renderCompactHelp(width int) string {
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#808080")).
		Render("↑↓:navigate/scroll • r:restart • s:stop • q:quit")

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#4DD4FF")).
		Padding(0, 1).
		Width(width - 2)

	return style.Render(help)
}

// renderShutdown renders shutdown message
func renderShutdown() string {
	shutdownStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6FFFB0")).
		Bold(true)

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
