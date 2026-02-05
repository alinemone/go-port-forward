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

// تعریف پالت رنگی برای UI
var (
	colorText      = lipgloss.Color("#E6E6E6")
	colorMuted     = lipgloss.Color("#8A94A6")
	colorBorder    = lipgloss.Color("#2E3A4A")
	colorAccent    = lipgloss.Color("#4DD4FF")
	colorAccentAlt = lipgloss.Color("#6FFFB0")
	colorWarn      = lipgloss.Color("#FFE66D")
	colorError     = lipgloss.Color("#FF6B6B")
)

// مدل اصلی UI
type UI struct {
	manager       *ServiceManager
	services      []Service
	cursorIndex   int
	quitting      bool
	width         int
	height        int
	viewport      viewport.Model
	ready         bool
	ctx           context.Context
	addMode       bool
	addCandidates []string
	addCursor     int
	addSelected   map[string]bool
}

const uiTickInterval = 500 * time.Millisecond

// ساخت مدل UI جدید
func NewUI(manager *ServiceManager, ctx context.Context) *UI {
	return &UI{
		manager:  manager,
		services: []Service{},
		ctx:      ctx,
	}
}

// مقداردهی اولیه مدل Bubble Tea
func (u *UI) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(uiTickInterval),
		tea.EnterAltScreen,
	)
}

// مدیریت رویدادها و به‌روزرسانی وضعیت UI
func (u *UI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		u.width = msg.Width
		u.height = msg.Height

		viewportHeight := calculateViewportHeight(len(u.services), u.height)
		if !u.ready {
			u.viewport = viewport.New(msg.Width, viewportHeight)
			u.viewport.YPosition = 0
			u.ready = true
		} else {
			u.viewport.Width = msg.Width
			u.viewport.Height = viewportHeight
		}

	case tea.KeyMsg:
		keyRaw := msg.String()
		key := keyRaw
		if keyRaw != " " {
			key = normalizeToken(keyRaw)
		}
		if u.addMode {
			return u.updateAddMode(msg)
		}

		switch key {
		case "q", "ctrl+c", "esc":
			u.quitting = true
			u.manager.StopAllServices()
			return u, tea.Quit

		case "up", "k":
			if u.cursorIndex > 0 {
				u.cursorIndex--
			} else {
				u.viewport, cmd = u.viewport.Update(msg)
			}

		case "down", "j":
			if u.cursorIndex < len(u.services)-1 {
				u.cursorIndex++
			} else {
				u.viewport, cmd = u.viewport.Update(msg)
			}

		case "r":
			if u.cursorIndex < len(u.services) && len(u.services) > 0 {
				serviceName := u.services[u.cursorIndex].Name
				u.manager.RestartService(u.ctx, serviceName)
			}

		case "s":
			if u.cursorIndex < len(u.services) && len(u.services) > 0 {
				u.manager.StopService(u.services[u.cursorIndex].Name)
			}

		case "a":
			u.enterAddMode()

		default:
			u.viewport, cmd = u.viewport.Update(msg)
		}

	case tickMsg:
		u.services = u.manager.ListServiceStates()
		u.ensureCursorInRange()
		u.refreshViewportContent()
		return u, tickCmd(uiTickInterval)
	}

	return u, cmd
}

// رندر رابط کاربری
func (u *UI) View() string {
	if u.quitting {
		return renderShutdownScreen()
	}

	if !u.ready {
		return "Initializing..."
	}

	if u.addMode {
		return u.renderAddServiceOverlay()
	}

	u.ensureViewportSize()

	sections := make([]string, 0, 3)
	if len(u.services) == 0 {
		sections = append(sections, renderEmptyState())
	} else {
		sections = append(sections, renderServiceTable(u.services, u.cursorIndex, u.width))
	}

	logBoxWidth := u.width - 2
	if logBoxWidth < 58 {
		logBoxWidth = 58
	}
	logBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentAlt).
		Width(logBoxWidth).
		Render(u.viewport.View())
	sections = append(sections, logBox)

	sections = append(sections, renderHelp(u.width))
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// ورود به حالت افزودن سرویس
func (u *UI) enterAddMode() {
	storage := NewStorage()
	allServices, err := storage.ListServiceNames()
	if err != nil {
		return
	}

	runningServices := u.manager.ListServiceStates()
	runningMap := make(map[string]bool)
	for i := range runningServices {
		runningMap[runningServices[i].Name] = true
	}

	available := make([]string, 0)
	for _, serviceName := range allServices {
		if !runningMap[serviceName] {
			available = append(available, serviceName)
		}
	}

	if len(allServices) > 0 {
		u.addMode = true
		if len(available) == 0 {
			u.addCandidates = []string{"(All services are already running)"}
		} else {
			u.addCandidates = available
		}
		u.addCursor = 0
		u.addSelected = make(map[string]bool)
	}
}

// پردازش کلیدها در حالت افزودن سرویس
func (u *UI) updateAddMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyRaw := msg.String()
	key := keyRaw
	if keyRaw != " " {
		key = normalizeToken(keyRaw)
	}
	switch key {
	case "esc":
		u.addMode = false
		u.addCandidates = nil
		u.addCursor = 0
		u.addSelected = nil
	case "up", "k":
		if u.addCursor > 0 {
			u.addCursor--
		}
	case "down", "j":
		if u.addCursor < len(u.addCandidates)-1 {
			u.addCursor++
		}
	case " ":
		if u.addCursor < len(u.addCandidates) {
			serviceName := u.addCandidates[u.addCursor]
			if serviceName != "(All services are already running)" {
				u.addSelected[serviceName] = !u.addSelected[serviceName]
			}
		}
	case "enter":
		for serviceName, selected := range u.addSelected {
			if selected {
				_ = u.manager.StartStoredService(u.ctx, serviceName)
			}
		}
		u.addMode = false
		u.addCandidates = nil
		u.addCursor = 0
		u.addSelected = nil
	}
	return u, nil
}

// اطمینان از معتبر بودن موقعیت انتخاب
func (u *UI) ensureCursorInRange() {
	if u.cursorIndex >= len(u.services) && len(u.services) > 0 {
		u.cursorIndex = len(u.services) - 1
	}
	if len(u.services) == 0 {
		u.cursorIndex = 0
	}
}

// به‌روزرسانی محتوای viewport لاگ‌ها
func (u *UI) refreshViewportContent() {
	if !u.ready {
		return
	}

	u.ensureViewportSize()
	contentWidth := u.viewport.Width - 4
	if contentWidth < 40 {
		contentWidth = 40
	}

	newContent := renderLogsContent(u.services, contentWidth)
	u.viewport.SetContent(newContent)
	u.viewport.GotoBottom()
}

// تنظیم اندازه viewport بر اساس ارتفاع پنجره
func (u *UI) ensureViewportSize() {
	if u.width == 0 || u.height == 0 {
		return
	}

	viewportHeight := calculateViewportHeight(len(u.services), u.height)
	if u.viewport.Height != viewportHeight {
		u.viewport.Height = viewportHeight
	}
	if u.viewport.Width != u.width {
		u.viewport.Width = u.width
	}
}

// محاسبه ارتفاع مناسب برای viewport
func calculateViewportHeight(serviceCount, totalHeight int) int {
	tableLines := 4 + serviceCount
	if serviceCount == 0 {
		tableLines = 1
	}
	overhead := tableLines + 2 + 3
	viewportHeight := totalHeight - overhead
	if viewportHeight < 3 {
		viewportHeight = 3
	}
	return viewportHeight
}

// نمایش حالت بدون سرویس
func renderEmptyState() string {
	emptyStyle := lipgloss.NewStyle().
		Foreground(colorMuted).
		Italic(true)
	return emptyStyle.Render("⚬ No services running...")
}

// رندر جدول سرویس‌ها
func renderServiceTable(services []Service, selectedIndex int, width int) string {
	if width < 60 {
		width = 60
	}

	compact := width < 90
	statusWidth := 12
	uptimeWidth := 8
	portWidth := 6
	restartWidth := 8
	maxNameLen := 7
	for i := range services {
		nameLen := len(services[i].Name)
		if nameLen > maxNameLen {
			maxNameLen = nameLen
		}
	}
	if maxNameLen > 30 {
		maxNameLen = 30
	}

	// تنظیم عرض‌ها بر اساس فضای موجود برای هم‌ترازی بهتر
	available := width - 2
	if available < 60 {
		available = 60
	}
	if compact {
		minName := 8
		fixed := statusWidth + portWidth + 6
		nameWidth := available - fixed
		if nameWidth < minName {
			nameWidth = minName
		}
		if nameWidth > maxNameLen {
			nameWidth = maxNameLen
		}
		maxNameLen = nameWidth
	} else {
		minName := 10
		fixed := statusWidth + uptimeWidth + portWidth + restartWidth + 10
		nameWidth := available - fixed
		if nameWidth < minName {
			nameWidth = minName
		}
		if nameWidth > maxNameLen {
			nameWidth = maxNameLen
		}
		maxNameLen = nameWidth
	}

	rows := make([]string, 0, len(services)+2)
	headerPrefix := "  "
	headerLine := headerPrefix + fmt.Sprintf(
		"%-*s  %-*s",
		maxNameLen, "SERVICE",
		statusWidth, "STATUS",
	)
	if compact {
		headerLine += fmt.Sprintf("  %-*s", portWidth, "PORT")
	} else {
		headerLine += fmt.Sprintf(
			"  %-*s  %-*s  %-*s",
			uptimeWidth, "UPTIME",
			portWidth, "PORT",
			restartWidth, "RESTARTS",
		)
	}
	header := lipgloss.NewStyle().
		Foreground(colorText).
		Bold(true).
		Render(headerLine)
	rows = append(rows, header)

	sepWidth := width - 6
	if sepWidth < 50 {
		sepWidth = 50
	}
	if sepWidth > 200 {
		sepWidth = 200
	}
	rows = append(rows, strings.Repeat("─", sepWidth))

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
			statusColor = colorAccentAlt
			statusIcon = "●"
			statusText = "HEALTHY"
		case StatusConnecting:
			statusColor = colorWarn
			statusIcon = "◐"
			statusText = "CONNECTING"
		case StatusError:
			statusColor = colorError
			statusIcon = "✗"
			statusText = "ERROR"
		}

		uptime := formatUptime(svc.StartTime)

		displayName := svc.Name
		if len(displayName) > maxNameLen {
			displayName = displayName[:maxNameLen-3] + "..."
		}
		name := fmt.Sprintf("%-*s", maxNameLen, displayName)
		status := fmt.Sprintf("%s %-*s", statusIcon, statusWidth-2, statusText)
		uptimeStr := fmt.Sprintf("%-*s", uptimeWidth, uptime)
		portStr := fmt.Sprintf("%-*s", portWidth, svc.LocalPort)
		restarts := fmt.Sprintf("%-*d", restartWidth, svc.RestartCount)

		styledName := lipgloss.NewStyle().
			Foreground(colorText).
			Bold(true).
			Render(name)

		styledStatus := lipgloss.NewStyle().
			Foreground(statusColor).
			Render(status)

		styledUptime := lipgloss.NewStyle().
			Foreground(colorMuted).
			Render(uptimeStr)

		styledRestarts := lipgloss.NewStyle().
			Foreground(colorMuted).
			Render(restarts)

		styledPort := lipgloss.NewStyle().
			Foreground(colorText).
			Render(portStr)

		row := highlight + styledName + "  " + styledStatus
		if compact {
			row += "  " + styledPort
		} else {
			row += "  " + styledUptime + "  " + styledPort + "  " + styledRestarts
		}
		rows = append(rows, row)
	}

	table := lipgloss.JoinVertical(lipgloss.Left, rows...)
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(width - 2)

	return style.Render(table)
}

// قالب‌بندی زمان روشن بودن سرویس
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

// رندر لاگ‌های ترکیبی برای viewport
func renderLogsContent(services []Service, maxWidth int) string {
	var content strings.Builder

	type LogWithService struct {
		ServiceName string
		Entry       LogEntry
	}

	allLogs := make([]LogWithService, 0)
	for i := range services {
		svc := &services[i]
		for _, log := range svc.Logs {
			allLogs = append(allLogs, LogWithService{
				ServiceName: svc.Name,
				Entry:       log,
			})
		}
	}

	sort.Slice(allLogs, func(i, j int) bool {
		return allLogs[i].Entry.Time.Before(allLogs[j].Entry.Time)
	})

	if len(allLogs) == 0 {
		content.WriteString(lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true).
			Render("No logs yet..."))
		return content.String()
	}

	for i := 0; i < len(allLogs); i++ {
		log := allLogs[i]
		timestamp := log.Entry.Time.Format("15:04:05")

		nameWidth := maxWidth / 4
		if nameWidth < 8 {
			nameWidth = 8
		}
		if nameWidth > 24 {
			nameWidth = 24
		}
		serviceName := truncateRunes(log.ServiceName, nameWidth)
		namePlain := padRightRunes(serviceName, nameWidth)

		message := log.Entry.Message
		msgColor := colorText
		if log.Entry.IsError {
			msgColor = colorError
		} else if strings.Contains(message, "━━━━") {
			msgColor = colorWarn
		}

		prefixWidth := nameWidth + 12
		availableWidth := maxWidth - prefixWidth
		if availableWidth < 20 {
			availableWidth = 20
		}

		wrappedLines := wrapText(message, availableWidth)

		nameStyled := lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			Render(namePlain)

		timeStyled := lipgloss.NewStyle().
			Foreground(colorMuted).
			Render(timestamp)

		if len(wrappedLines) > 0 {
			msgStyled := lipgloss.NewStyle().
				Foreground(msgColor).
				Render(wrappedLines[0])
			logLine := fmt.Sprintf("[%s %s] %s", nameStyled, timeStyled, msgStyled)
			content.WriteString(logLine)
			content.WriteString("\n")

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

	return content.String()
}

// شکست متن برای رعایت عرض نمایش
func wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}
	if len(text) <= maxWidth {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
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
		if len(word) > maxWidth {
			if currentLine.Len() > 0 {
				lines = append(lines, currentLine.String())
				currentLine.Reset()
			}
			for i := 0; i < len(word); i += maxWidth {
				end := i + maxWidth
				if end > len(word) {
					end = len(word)
				}
				lines = append(lines, word[i:end])
			}
			continue
		}

		testLine := currentLine.String()
		if len(testLine) > 0 {
			testLine += " " + word
		} else {
			testLine = word
		}

		if len(testLine) > maxWidth {
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

	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return lines
}

// کوتاه‌سازی رشته با سه‌نقطه
func truncateRunes(text string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

// پر کردن فضای خالی برای هم‌ترازی
func padRightRunes(text string, width int) string {
	runes := []rune(text)
	if len(runes) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-len(runes))
}

// نمایش پنجره افزودن سرویس
func (u *UI) renderAddServiceOverlay() string {
	width := u.width
	if width <= 0 {
		width = 120
	}
	if width < 60 {
		width = 60
	}

	maxNameLen := 7
	for _, serviceName := range u.addCandidates {
		nameLen := len(serviceName)
		if nameLen > maxNameLen {
			maxNameLen = nameLen
		}
	}
	if maxNameLen > 30 {
		maxNameLen = 30
	}

	rows := make([]string, 0, len(u.addCandidates)+2)
	headerName := fmt.Sprintf("%-*s", maxNameLen, "SERVICE")
	header := lipgloss.NewStyle().
		Foreground(colorText).
		Bold(true).
		Render(headerName + "  SELECT")
	rows = append(rows, header)

	sepWidth := width - 6
	if sepWidth < 50 {
		sepWidth = 50
	}
	if sepWidth > 200 {
		sepWidth = 200
	}
	rows = append(rows, strings.Repeat("─", sepWidth))

	for i, serviceName := range u.addCandidates {
		highlight := "  "
		if i == u.addCursor {
			highlight = "► "
		}

		isSelected := u.addSelected != nil && u.addSelected[serviceName]
		checkbox := "[ ]"
		if isSelected {
			checkbox = "[✓]"
		}

		displayName := serviceName
		if len(displayName) > maxNameLen {
			displayName = displayName[:maxNameLen-3] + "..."
		}
		name := fmt.Sprintf("%-*s", maxNameLen, displayName)
		selectStr := fmt.Sprintf("%-7s", checkbox)

		styledName := lipgloss.NewStyle().
			Foreground(colorText).
			Bold(true).
			Render(name)

		styledSelect := lipgloss.NewStyle().
			Foreground(colorMuted).
			Render(selectStr)

		row := highlight + styledName + "  " + styledSelect
		rows = append(rows, row)
	}

	table := lipgloss.JoinVertical(lipgloss.Left, rows...)
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(width - 2)

	overlayBox := style.Render(table)

	instructions := "↑↓:navigate • Space:toggle selection • Enter:add selected • Esc:cancel"
	instructionStyled := lipgloss.NewStyle().
		Foreground(colorMuted).
		Render(instructions)

	return lipgloss.JoinVertical(lipgloss.Left, overlayBox, instructionStyled)
}

// نمایش راهنمای کلیدها
func renderHelp(width int) string {
	if width < 60 {
		width = 60
	}

	helpText := "↑↓/j/k: move  •  a: add  •  r: restart  •  s: stop  •  q/esc: quit"
	if width < 90 {
		helpText = "↑↓: move  •  a:add  •  r:restart  •  s:stop  •  q:quit"
	}
	help := lipgloss.NewStyle().
		Foreground(colorMuted).
		Render(helpText)

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(width - 2)

	return style.Render(help)
}

// نمایش پیام خروج امن
func renderShutdownScreen() string {
	shutdownStyle := lipgloss.NewStyle().
		Foreground(colorAccentAlt).
		Bold(true)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentAlt).
		Padding(1, 4).
		Align(lipgloss.Center)

	return box.Render(shutdownStyle.Render("✓ Shutting down gracefully..."))
}

// ساخت فرمان تیک برای بروزرسانی دوره‌ای
func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
