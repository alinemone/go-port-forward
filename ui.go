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
	manager              *Manager
	services             []Service
	selectedIndex        int
	quitting             bool
	width                int
	height               int
	viewport             viewport.Model
	ready                bool
	ctx                  context.Context
	addingService        bool
	availableServices    []string
	selectedServiceIndex int
	selectedServices     map[string]bool // For multi-select

	// Performance optimizations
	lastRenderHash string        // Cache for rendered content
	tickInterval   time.Duration // Dynamic tick rate
	lastActivity   time.Time     // Last activity timestamp
}

// NewUI creates a new UI
func NewUI(manager *Manager, ctx context.Context) *UI {
	return &UI{
		manager:       manager,
		services:      []Service{},
		selectedIndex: 0,
		ctx:           ctx,
		tickInterval:  500 * time.Millisecond, // Default tick rate
		lastActivity:  time.Now(),
	}
}

// Init initializes the UI
func (u *UI) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(u.tickInterval),
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
		// Handle service addition mode
		if u.addingService {
			switch msg.String() {
			case "esc":
				u.addingService = false
				u.availableServices = nil
				u.selectedServiceIndex = 0
				u.selectedServices = nil
			case "up", "k":
				if u.selectedServiceIndex > 0 {
					u.selectedServiceIndex--
				}
			case "down", "j":
				if u.selectedServiceIndex < len(u.availableServices)-1 {
					u.selectedServiceIndex++
				}
			case " ":
				// Toggle selection for current item
				if u.selectedServiceIndex < len(u.availableServices) {
					serviceName := u.availableServices[u.selectedServiceIndex]
					if serviceName != "(All services are already running)" {
						if u.selectedServices == nil {
							u.selectedServices = make(map[string]bool)
						}
						u.selectedServices[serviceName] = !u.selectedServices[serviceName]
					}
				}
			case "enter":
				// Add all selected services
				if u.selectedServices != nil {
					for serviceName, selected := range u.selectedServices {
						if selected {
							_ = u.manager.AddService(u.ctx, serviceName)
						}
					}
				}
				u.addingService = false
				u.availableServices = nil
				u.selectedServiceIndex = 0
				u.selectedServices = nil
			}
			return u, cmd
		}

		// Normal mode handling
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

		case "a":
			// Add new service
			u.enterAddServiceMode()

		default:
			// Pass other keys to viewport
			u.viewport, cmd = u.viewport.Update(msg)
		}

	case tickMsg:
		// Update services
		newServices := u.manager.GetStates()

		// Check if there are any changes that require UI update
		hasChanges := len(newServices) != len(u.services)
		if !hasChanges {
			for i, svc := range newServices {
				if i >= len(u.services) || svc.Status != u.services[i].Status ||
					svc.Error != u.services[i].Error || svc.ReconnectCount != u.services[i].ReconnectCount {
					hasChanges = true
					break
				}
			}
		}

		u.services = newServices

		// Update viewport content only if there are changes
		if u.ready && hasChanges {
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

			// Only update content if it actually changed
			newContent := renderCombinedLogsContent(u.services)
			if newContent != u.lastRenderHash {
				u.viewport.SetContent(newContent)
				u.lastRenderHash = newContent
				u.viewport.GotoBottom()
			}

			// Update activity timestamp
			u.lastActivity = time.Now()
		}

		// Adjust selected index if needed
		if u.selectedIndex >= len(u.services) && len(u.services) > 0 {
			u.selectedIndex = len(u.services) - 1
		}

		// Dynamic tick rate based on activity
		newInterval := u.tickInterval
		timeSinceActivity := time.Since(u.lastActivity)

		// Slow down tick rate if no recent activity
		if timeSinceActivity > 30*time.Second {
			newInterval = 2000 * time.Millisecond // 2 seconds
		} else if timeSinceActivity > 10*time.Second {
			newInterval = 1000 * time.Millisecond // 1 second
		} else if timeSinceActivity > 5*time.Second {
			newInterval = 750 * time.Millisecond // 750ms
		} else {
			newInterval = 500 * time.Millisecond // 500ms default
		}

		// Update tick interval if changed
		if newInterval != u.tickInterval {
			u.tickInterval = newInterval
		}

		return u, tickCmd(u.tickInterval)
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

	// If in add service mode, show service selection overlay
	if u.addingService {
		return u.renderAddServiceOverlay()
	}

	// Ensure viewport has correct dimensions before rendering
	if u.width > 0 && u.height > 0 {
		tableLines := 4 + len(u.services)
		if len(u.services) == 0 {
			tableLines = 1
		}
		overhead := tableLines + 2 + 3
		viewportHeight := u.height - overhead
		if viewportHeight < 3 {
			viewportHeight = 3
		}
		if u.viewport.Height != viewportHeight {
			u.viewport.Height = viewportHeight
		}
		if u.viewport.Width != u.width {
			u.viewport.Width = u.width
		}
	}

	var sections []string

	// Services Table (compact)
	if len(u.services) == 0 {
		sections = append(sections, renderEmpty())
	} else {
		sections = append(sections, renderCompactServicesTable(u.services, u.selectedIndex, u.width))
	}

	// Scrollable logs with border (full width)
	logBoxWidth := u.width - 2
	if logBoxWidth < 58 {
		logBoxWidth = 58 // Minimum width
	}
	logBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6FFFB0")).
		Width(logBoxWidth).
		Render(u.viewport.View())
	sections = append(sections, logBox)

	// Help always at bottom (full width)
	sections = append(sections, renderCompactHelp(u.width))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderEmpty renders empty state
func renderEmpty() string {
	emptyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#808080")).
		Italic(true)
	return emptyStyle.Render("⚬ No services running...")
}

// renderCompactServicesTable renders a compact services table
func renderCompactServicesTable(services []Service, selectedIndex int, width int) string {
	// Ensure minimum width
	if width < 60 {
		width = 60
	}

	// Calculate maximum service name length
	maxNameLen := 7 // minimum for "SERVICE" header
	for i := range services {
		nameLen := len(services[i].Name)
		if nameLen > maxNameLen {
			maxNameLen = nameLen
		}
	}
	// Cap at reasonable maximum to prevent table from being too wide
	if maxNameLen > 30 {
		maxNameLen = 30
	}

	var rows []string

	// Compact header with dynamic width
	headerName := fmt.Sprintf("%-*s", maxNameLen, "SERVICE")
	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Render(headerName + "  STATUS       UPTIME   RESTARTS")

	rows = append(rows, header)

	// Separator matches available width (accounting for border and padding)
	sepWidth := width - 6 // subtract border (2) and padding (4)
	if sepWidth < 50 {
		sepWidth = 50
	}
	if sepWidth > 200 {
		sepWidth = 200 // Maximum separator width
	}
	rows = append(rows, strings.Repeat("─", sepWidth))

	// Service rows
	for i := range services {
		svc := &services[i]
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

		// Build row parts - truncate long service names with ellipsis if needed
		displayName := svc.Name
		if len(displayName) > maxNameLen {
			displayName = displayName[:maxNameLen-3] + "..."
		}
		name := fmt.Sprintf("%-*s", maxNameLen, displayName)
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
		row := highlight + styledName + "  " + styledStatus + " " + styledUptime + " " + styledRestarts

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
	for i := range services {
		svc := &services[i]
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

			// Service name (max 12 chars with ellipsis)
			serviceName := log.ServiceName
			if len(serviceName) > 12 {
				serviceName = serviceName[:9] + "..."
			}

			// Truncate very long messages to prevent layout breaking
			message := log.Entry.Message
			if len(message) > 200 {
				message = message[:197] + "..."
			}

			// Message style based on error or info
			var msgColor lipgloss.Color
			if log.Entry.IsError {
				msgColor = lipgloss.Color("#FF6B6B")
			} else if strings.Contains(message, "━━━━") {
				msgColor = lipgloss.Color("#FFE66D")
			} else {
				msgColor = lipgloss.Color("#E0E0E0")
			}

			// Format: [service time] message
			nameStyled := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#4DD4FF")).
				Bold(true).
				Render(fmt.Sprintf("%-12s", serviceName))

			timeStyled := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#808080")).
				Render(timestamp)

			msgStyled := lipgloss.NewStyle().
				Foreground(msgColor).
				Render(message)

			logLine := fmt.Sprintf("[%s %s] %s", nameStyled, timeStyled, msgStyled)

			content.WriteString(logLine)
			content.WriteString("\n")
		}
	}

	return content.String()
}

// renderAddServiceOverlay renders the service selection overlay
func (u *UI) renderAddServiceOverlay() string {
	// Simple overlay with multi-select indicators
	var content []string

	// Header
	content = append(content, "╭─ SELECT SERVICES TO ADD ─────────────────────╮")
	content = append(content, "│                                              │")

	// Service list with multi-select indicators
	for i, serviceName := range u.availableServices {
		var prefix string
		if i == u.selectedServiceIndex {
			prefix = "❯ "
		} else {
			prefix = "  "
		}

		// Check if this service is selected
		var checkbox string
		if u.selectedServices != nil && u.selectedServices[serviceName] {
			checkbox = "✓"
		} else {
			checkbox = " "
		}

		line := fmt.Sprintf("│ %s[%s] %s%s │", prefix, checkbox, serviceName, strings.Repeat(" ", 37-len(serviceName)))
		content = append(content, line)
	}

	content = append(content, "│                                              │")

	// Instructions
	content = append(content, "│ ↑↓:navigate • Space:select • Enter:add • Esc:cancel │")
	content = append(content, "╰──────────────────────────────────────────────╯")

	return strings.Join(content, "\n")
}

// renderCompactHelp renders compact help at bottom
func renderCompactHelp(width int) string {
	// Ensure minimum width
	if width < 60 {
		width = 60
	}

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#808080")).
		Render("↑↓:navigate/scroll • r:restart • s:stop • a:add • q:quit")

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
// enterAddServiceMode enters the service addition mode
func (u *UI) enterAddServiceMode() {
	// Get all available services from storage
	storage := NewStorage()
	allServices, err := storage.GetAllServiceNames()
	if err != nil {
		return // Silently fail if we can't load services
	}

	// Get currently running services
	runningServices := u.manager.GetStates()
	runningMap := make(map[string]bool)
	for _, svc := range runningServices {
		runningMap[svc.Name] = true
	}

	// Filter out already running services
	available := make([]string, 0)
	for _, serviceName := range allServices {
		if !runningMap[serviceName] {
			available = append(available, serviceName)
		}
	}

	// Always enter add mode if we have services at all
	if len(allServices) > 0 {
		u.addingService = true
		if len(available) == 0 {
			// Show a message if all services are running
			u.availableServices = []string{"(All services are already running)"}
		} else {
			u.availableServices = available
		}
		u.selectedServiceIndex = 0
		u.selectedServices = make(map[string]bool) // Initialize for multi-select
	}
}

func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
