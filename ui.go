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

		// Calculate dynamic viewport height based on rendered sections
		servicesHeight, helpHeight := u.computeSectionHeights()
		overhead := servicesHeight + helpHeight + 2 // log box border
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

		// Reset render cache on resize to force recalculation
		u.lastRenderHash = ""

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
			for i := range newServices {
				svc := &newServices[i]
				if i >= len(u.services) || svc.Status != u.services[i].Status ||
					svc.Error != u.services[i].Error || svc.ReconnectCount != u.services[i].ReconnectCount ||
					serviceLogsChanged(svc, &u.services[i]) {
					hasChanges = true
					break
				}
			}
		}

		u.services = newServices

		// Update viewport content only if there are changes
		if u.ready && hasChanges {
			// Recalculate viewport height based on rendered sections
			servicesHeight, helpHeight := u.computeSectionHeights()
			overhead := servicesHeight + helpHeight + 2 // log box border
			viewportHeight := u.height - overhead
			if viewportHeight < 3 {
				viewportHeight = 3
			}
			u.viewport.Height = viewportHeight

			// Only update content if it actually changed
			// Pass viewport width for proper wrapping (subtract border width)
			contentWidth := u.viewport.Width - 4 // Account for border and padding
			if contentWidth < 10 {
				contentWidth = 10 // Minimum width
			}
			newContent := renderCombinedLogsContent(u.services, contentWidth)
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

	// If in add service mode, show overlay from top
	if u.addingService {
		overlayContent := u.renderAddServiceOverlay()

		// Render overlay at top without centering
		return overlayContent
	}

	// Ensure viewport has correct dimensions before rendering
	if u.width > 0 && u.height > 0 {
		servicesHeight, helpHeight := u.computeSectionHeights()
		overhead := servicesHeight + helpHeight + 2
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

	// Services Table (responsive)
	if len(u.services) == 0 {
		sections = append(sections, renderEmpty())
	} else {
		sections = append(sections, renderServicesSection(u.services, u.selectedIndex, u.width))
	}

	// Scrollable logs with border (full width)
	logBoxWidth := u.width - 2
	if logBoxWidth < 0 {
		logBoxWidth = 0
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

// renderServicesSection renders a responsive services section
func renderServicesSection(services []Service, selectedIndex int, width int) string {
	if width <= 0 {
		width = 80
	}

	return renderCompactServicesTable(services, selectedIndex, width)
}

// renderCompactServicesTable renders a compact services table
func renderCompactServicesTable(services []Service, selectedIndex int, width int) string {
	if width <= 0 {
		width = 80
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

	// Decide which columns to show based on width
	showUptime := width >= 72
	showRestarts := width >= 88

	// Column widths
	statusWidth := 12
	statusLabel := "STATUS"
	shortStatus := width < 40
	if shortStatus {
		statusWidth = 3
		statusLabel = "ST"
	}
	uptimeWidth := 8
	restartsWidth := 8
	baseWidth := 2 + 2 + statusWidth // highlight + spacing + status
	if showUptime {
		baseWidth += 2 + uptimeWidth
	}
	if showRestarts {
		baseWidth += 2 + restartsWidth
	}

	// Adjust name width to fit available space
	maxNameAvailable := width - baseWidth - 4
	if maxNameAvailable < 3 {
		maxNameAvailable = 3
	}
	if maxNameLen > maxNameAvailable {
		maxNameLen = maxNameAvailable
	}

	// Compact header with dynamic width
	headerTitle := "SERVICE"
	if maxNameLen < len(headerTitle) {
		headerTitle = "SVC"
	}
	headerName := fmt.Sprintf("%-*s", maxNameLen, headerTitle)
	headerParts := []string{headerName, statusLabel}
	if showUptime {
		headerParts = append(headerParts, "UPTIME")
	}
	if showRestarts {
		headerParts = append(headerParts, "RESTARTS")
	}
	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Render(strings.Join(headerParts, "  "))

	rows = append(rows, header)

	// Separator matches available width (accounting for border and padding)
	sepWidth := width - 6 // subtract border (2) and padding (4)
	if sepWidth < 1 {
		sepWidth = 1
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
		if shortStatus {
			statusText = statusText[:1]
		}
		status := fmt.Sprintf("%s %-2s", statusIcon, statusText)
		if !shortStatus {
			status = fmt.Sprintf("%s %-10s", statusIcon, statusText)
		}
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

		// Combine, respecting responsive columns
		row := highlight + styledName + "  " + styledStatus
		if showUptime {
			row += " " + styledUptime
		}
		if showRestarts {
			row += " " + styledRestarts
		}

		rows = append(rows, row)
	}

	table := lipgloss.JoinVertical(lipgloss.Left, rows...)

	// Apply border with matching width
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#4A90E2")).
		Padding(0, 1).
		Width(safeStyleWidth(width))

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

// renderCombinedLogsContent renders logs content for viewport with proper width handling
func renderCombinedLogsContent(services []Service, maxWidth int) string {
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

			// Message style based on error or info
			var msgColor lipgloss.Color
			message := log.Entry.Message
			if log.Entry.IsError {
				msgColor = lipgloss.Color("#FF6B6B")
			} else if strings.Contains(message, "━━━━") {
				msgColor = lipgloss.Color("#FFE66D")
			} else {
				msgColor = lipgloss.Color("#E0E0E0")
			}

			// Calculate prefix width: [serviceName timestamp]
			// serviceName is always 12 chars, timestamp is 8 chars, brackets and spaces = 5
			// Total prefix = 1 + 12 + 1 + 8 + 2 = 24 chars
			prefixWidth := 24

			// Calculate available width for message
			availableWidth := maxWidth - prefixWidth
			if availableWidth < 10 {
				availableWidth = 10 // Minimum message width
			}

			// Wrap message to fit available width
			wrappedLines := wrapText(message, availableWidth)

			// Format: [service time] message
			nameStyled := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#4DD4FF")).
				Bold(true).
				Render(fmt.Sprintf("%-12s", serviceName))

			timeStyled := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#808080")).
				Render(timestamp)

			// Render first line with prefix
			if len(wrappedLines) > 0 {
				msgStyled := lipgloss.NewStyle().
					Foreground(msgColor).
					Render(wrappedLines[0])
				logLine := fmt.Sprintf("[%s %s] %s", nameStyled, timeStyled, msgStyled)
				content.WriteString(logLine)
				content.WriteString("\n")

				// Render continuation lines with proper indentation
				if len(wrappedLines) > 1 {
					indent := strings.Repeat(" ", prefixWidth)
					for j := 1; j < len(wrappedLines); j++ {
						msgStyled := lipgloss.NewStyle().
							Foreground(msgColor).
							Render(wrappedLines[j])
						content.WriteString(indent + msgStyled + "\n")
					}
				}
			}
		}
	}

	return content.String()
}

// wrapText wraps text to fit within maxWidth, breaking at word boundaries
func wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}

	// If text fits, return as-is
	if len(text) <= maxWidth {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		// Handle text with no spaces (long single word)
		for i := 0; i < len(text); i += maxWidth {
			end := i + maxWidth
			if end > len(text) {
				end = len(text)
			}
			lines = append(lines, text[i:end])
		}
		return lines
	}

	var currentLine strings.Builder
	for _, word := range words {
		// If word itself is longer than maxWidth, split it
		if len(word) > maxWidth {
			// Flush current line if not empty
			if currentLine.Len() > 0 {
				lines = append(lines, currentLine.String())
				currentLine.Reset()
			}
			// Split the long word
			for i := 0; i < len(word); i += maxWidth {
				end := i + maxWidth
				if end > len(word) {
					end = len(word)
				}
				lines = append(lines, word[i:end])
			}
			continue
		}

		// Check if adding this word would exceed maxWidth
		testLine := currentLine.String()
		if len(testLine) > 0 {
			testLine += " " + word
		} else {
			testLine = word
		}

		if len(testLine) > maxWidth {
			// Flush current line and start new one
			if currentLine.Len() > 0 {
				lines = append(lines, currentLine.String())
				currentLine.Reset()
			}
			currentLine.WriteString(word)
		} else {
			if currentLine.Len() > 0 {
				currentLine.WriteString(" ")
			}
			currentLine.WriteString(word)
		}
	}

	// Add the last line if not empty
	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return lines
}

// renderAddServiceOverlay renders the service selection overlay
func (u *UI) renderAddServiceOverlay() string {
	// Use exact same width as main UI
	width := u.width
	if width <= 0 {
		width = 120 // fallback
	}

	// Calculate maximum service name length (exact same as main UI)
	maxNameLen := 7 // minimum for "SERVICE" header
	for _, serviceName := range u.availableServices {
		nameLen := len(serviceName)
		if nameLen > maxNameLen {
			maxNameLen = nameLen
		}
	}
	// Cap at reasonable maximum to prevent table from being too wide (same as main UI)
	if maxNameLen > 30 {
		maxNameLen = 30
	}

	var rows []string

	// Compact header with dynamic width (exact copy from main UI)
	headerName := fmt.Sprintf("%-*s", maxNameLen, "SERVICE")
	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Render(headerName + "  SELECT")

	rows = append(rows, header)

	// Separator matches available width (exact copy from main UI)
	sepWidth := width - 6 // subtract border (2) and padding (4)
	if sepWidth < 1 {
		sepWidth = 1
	}
	if sepWidth > 200 {
		sepWidth = 200 // Maximum separator width
	}
	rows = append(rows, strings.Repeat("─", sepWidth))

	// Service rows with exact same styling as main UI
	for i, serviceName := range u.availableServices {
		// Highlight logic (same as main UI)
		highlight := "  "
		if i == u.selectedServiceIndex {
			highlight = "► "
		}

		// Checkbox status
		isSelected := false
		if u.selectedServices != nil && u.selectedServices[serviceName] {
			isSelected = true
		}

		// Checkbox display
		var checkbox string
		if isSelected {
			checkbox = "[✓]"
		} else {
			checkbox = "[ ]"
		}

		// Build row parts (exact same as main UI)
		displayName := serviceName
		if len(displayName) > maxNameLen {
			displayName = displayName[:maxNameLen-3] + "..."
		}
		name := fmt.Sprintf("%-*s", maxNameLen, displayName)
		selectStr := fmt.Sprintf("%-7s", checkbox) // Fixed width for select column

		// Style each part (exact same as main UI)
		styledName := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E0E0E0")).
			Bold(true).
			Render(name)

		styledSelect := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#C0C0C0")).
			Render(selectStr)

		// Combine (exact same as main UI)
		row := highlight + styledName + "  " + styledSelect

		rows = append(rows, row)
	}

	table := lipgloss.JoinVertical(lipgloss.Left, rows...)

	// Apply border with matching width (exact same as main UI)
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#4A90E2")).
		Padding(0, 1).
		Width(safeStyleWidth(width))

	overlayBox := style.Render(table)

	// Add instructions below the main box
	instructions := "↑↓:navigate • Space:toggle selection • Enter:add selected • Esc:cancel"
	instructionStyled := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#808080")).
		Render(instructions)

	// Combine box and instructions with spacing
	return lipgloss.JoinVertical(lipgloss.Left, overlayBox, instructionStyled)
}

// renderCompactHelp renders compact help at bottom
func renderCompactHelp(width int) string {
	if width <= 0 {
		width = 80
	}

	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#808080")).
		Render("↑↓:navigate/scroll • r:restart • s:stop • a:add • q:quit")

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#4DD4FF")).
		Padding(0, 1).
		Width(safeStyleWidth(width))

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
	for i := range runningServices {
		runningMap[runningServices[i].Name] = true
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

func safeStyleWidth(width int) int {
	if width <= 2 {
		if width < 0 {
			return 0
		}
		return width
	}
	return width - 2
}

func serviceLogsChanged(newSvc *Service, oldSvc *Service) bool {
	if len(newSvc.LogHistory) != len(oldSvc.LogHistory) {
		return true
	}
	if len(newSvc.LogHistory) == 0 {
		return false
	}
	newLast := newSvc.LogHistory[len(newSvc.LogHistory)-1]
	oldLast := oldSvc.LogHistory[len(oldSvc.LogHistory)-1]
	return !newLast.Time.Equal(oldLast.Time) || newLast.IsError != oldLast.IsError || newLast.Message != oldLast.Message
}

func (u *UI) computeSectionHeights() (int, int) {
	if u.width <= 0 {
		return 0, 0
	}
	services := renderEmpty()
	if len(u.services) > 0 {
		services = renderServicesSection(u.services, u.selectedIndex, u.width)
	}
	help := renderCompactHelp(u.width)
	return lipgloss.Height(services), lipgloss.Height(help)
}
