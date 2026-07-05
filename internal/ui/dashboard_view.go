package ui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/model"
)

func renderEmptyState() string {
	emptyStyle := lipgloss.NewStyle().
		Foreground(colorMuted).
		Italic(true)
	return emptyStyle.Render("⚬ No services running...")
}

func renderServiceTable(services []model.Service, selectedIndex, offset, maxVisible, width int) string {
	if width < 60 {
		width = 60
	}

	if maxVisible <= 0 {
		maxVisible = len(services)
	}
	start := offset
	if start < 0 {
		start = 0
	}
	end := start + maxVisible
	if end > len(services) {
		end = len(services)
	}

	compact := width < 90
	showIcons := false
	for i := start; i < end; i++ {
		if services[i].IconEnabled {
			showIcons = true
			break
		}
	}
	iconWidth := 0
	if showIcons {
		iconWidth = 2
	}
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

	available := width - 2
	if available < 60 {
		available = 60
	}
	if compact {
		minName := 8
		fixed := statusWidth + portWidth + iconWidth + 6
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
		fixed := statusWidth + uptimeWidth + portWidth + restartWidth + iconWidth + 10
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
	nameCellWidth := maxNameLen + iconWidth
	headerLine := headerPrefix + padRightDisplayWidth("SERVICE", nameCellWidth) + fmt.Sprintf(
		"  %-*s",
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
		Foreground(colorHeading).
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
	rows = append(rows, lipgloss.NewStyle().Foreground(colorBorder).Render(strings.Repeat("─", sepWidth)))

	for i := start; i < end; i++ {
		svc := &services[i]
		var statusIcon, statusText string
		var statusColor color.Color

		selected := i == selectedIndex
		highlight := "  "
		if selected {
			highlight = "► "
		}

		switch svc.Status {
		case model.StatusHealthy:
			statusColor = statusHealthyColor
			statusIcon = "●"
			statusText = "HEALTHY"
		case model.StatusConnecting:
			statusColor = statusConnectingColor
			statusIcon = "◐"
			statusText = "CONNECTING"
		case model.StatusError:
			statusColor = statusErrorColor
			statusIcon = "✗"
			statusText = "ERROR"
		}

		uptime := formatUptime(svc.StartTime)

		status := fmt.Sprintf("%s %-*s", statusIcon, statusWidth-2, statusText)
		uptimeStr := fmt.Sprintf("%-*s", uptimeWidth, uptime)
		portStr := fmt.Sprintf("%-*s", portWidth, svc.LocalPort)
		restarts := fmt.Sprintf("%-*d", restartWidth, svc.RestartCount)

		nameColor := colorText
		if selected {
			nameColor = colorAccent
		}
		displayName := truncateRunes(svc.Name, maxNameLen)
		nameText := padRightDisplayWidth(displayName, maxNameLen)
		styledName := lipgloss.NewStyle().
			Foreground(nameColor).
			Bold(true).
			Render(nameText)
		if showIcons {
			cell := "  "
			if svc.IconEnabled {
				icon := serviceIcon(svc)
				cell = renderIconCell(icon.Glyph, icon.Color)
			}
			styledName = padRightDisplayWidth(cell+styledName, nameCellWidth)
		}

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

		marker := highlight
		if selected {
			marker = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(highlight)
		}

		row := marker + styledName + "  " + styledStatus
		if compact {
			row += "  " + styledPort
		} else {
			row += "  " + styledUptime + "  " + styledPort + "  " + styledRestarts
		}
		rows = append(rows, row)
	}

	if len(services) > maxVisible {
		above := start
		below := len(services) - end
		var parts []string
		if above > 0 {
			parts = append(parts, fmt.Sprintf("↑ %d more above", above))
		}
		if below > 0 {
			parts = append(parts, fmt.Sprintf("↓ %d more below", below))
		}
		indicator := fmt.Sprintf("%s   (%d–%d of %d • ↑↓ to scroll)",
			strings.Join(parts, "   "), start+1, end, len(services))
		rows = append(rows, lipgloss.NewStyle().
			Foreground(colorWarn).
			Bold(true).
			Render(indicator))
	}

	table := lipgloss.JoinVertical(lipgloss.Left, rows...)
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(width - 2)

	return style.Render(table)
}

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

func renderLogsContent(services []model.Service, maxWidth int) string {
	var content strings.Builder

	type logWithService struct {
		ServiceName string
		Entry       model.LogEntry
	}

	allLogs := make([]logWithService, 0)
	for i := range services {
		svc := &services[i]
		for _, log := range svc.Logs {
			allLogs = append(allLogs, logWithService{
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

// helpLines builds the wrapped, balanced content rows for the help bar (without
// the surrounding border). The height layout depends on len(helpLines(...)), so
// renderHelp must render exactly these lines.
func helpLines(width int, logScope string) []string {
	if width < 60 {
		width = 60
	}

	keyStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(colorMuted)
	const sepText = "  •  "
	sepStyled := descStyle.Render(sepText)
	sepW := lipgloss.Width(sepText)

	type chip struct{ k, d string }
	var chips []chip
	if width < 90 {
		chips = []chip{
			{"↑↓", "move"},
			{"l", "logs=" + logScope},
			{"a", "add/edit"},
			{"c", "config"},
			{"r", "restart"},
			{"s", "stop"},
			{"q", "quit"},
		}
	} else {
		chips = []chip{
			{"↑↓/j/k", "move"},
			{"l", "logs=" + logScope},
			{"a", "add/edit"},
			{"c", "config"},
			{"r", "restart"},
			{"^r", "restart all"},
			{"s", "stop"},
			{"q", "quit"},
		}
	}

	n := len(chips)
	styled := make([]string, n)
	widths := make([]int, n)
	for i, c := range chips {
		styled[i] = keyStyle.Render(c.k) + descStyle.Render(" "+c.d)
		widths[i] = lipgloss.Width(c.k + " " + c.d)
	}

	inner := width - 4 // 2 border + 2 padding
	if inner < 10 {
		inner = 10
	}

	// Minimum number of lines a greedy fit needs at this width.
	minLines := 1
	lineW := 0
	for i, w := range widths {
		if i == 0 {
			lineW = w
			continue
		}
		if lineW+sepW+w > inner {
			minLines++
			lineW = w
		} else {
			lineW += sepW + w
		}
	}

	// Split into minLines rows of (almost) equal chip count so the rows look
	// balanced (e.g. 4+4 instead of 7+1). Fall back to greedy if an even
	// split would overflow.
	return balancedHelpLines(styled, widths, sepStyled, sepW, inner, minLines)
}

func renderHelp(width int, logScope string) string {
	boxWidth := width
	if boxWidth < 60 {
		boxWidth = 60
	}

	help := strings.Join(helpLines(width, logScope), "\n")

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(boxWidth - 2)

	return style.Render(help)
}

// balancedHelpLines splits chips into L contiguous rows of as-equal-as-possible
// count. If any such row would exceed inner width, it falls back to greedy
// packing (which fits by construction).
func balancedHelpLines(styled []string, widths []int, sepStyled string, sepW, inner, L int) []string {
	n := len(styled)
	if L < 1 {
		L = 1
	}

	base, rem := n/L, n%L
	groups := make([][]string, 0, L)
	idx := 0
	for g := 0; g < L; g++ {
		cnt := base
		if g < rem {
			cnt++
		}
		w := 0
		for j := idx; j < idx+cnt; j++ {
			w += widths[j]
			if j > idx {
				w += sepW
			}
		}
		if w > inner {
			return greedyHelpLines(styled, widths, sepStyled, sepW, inner)
		}
		groups = append(groups, styled[idx:idx+cnt])
		idx += cnt
	}

	out := make([]string, 0, len(groups))
	for _, g := range groups {
		out = append(out, strings.Join(g, sepStyled))
	}
	return out
}

func greedyHelpLines(styled []string, widths []int, sepStyled string, sepW, inner int) []string {
	var lines []string
	var line string
	lineW := 0
	for i, s := range styled {
		switch {
		case line == "":
			line, lineW = s, widths[i]
		case lineW+sepW+widths[i] > inner:
			lines = append(lines, line)
			line, lineW = s, widths[i]
		default:
			line += sepStyled + s
			lineW += sepW + widths[i]
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func (u *UI) renderShutdownScreen() string {
	frame := spinnerFrames[u.spinnerFrame%len(spinnerFrames)]

	shutdownStyle := lipgloss.NewStyle().
		Foreground(colorAccentAlt).
		Bold(true)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(1, 4).
		Align(lipgloss.Center).
		Render(shutdownStyle.Render(fmt.Sprintf("%s  Stopping services, please wait...", frame)))

	if u.width <= 0 || u.height <= 0 {
		return box
	}
	return lipgloss.Place(u.width, u.height, lipgloss.Center, lipgloss.Center, box)
}
