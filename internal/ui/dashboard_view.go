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
	if width < 20 {
		width = 20
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
	narrow := width < 60
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
	if narrow {
		statusWidth = 3
	}
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

	available := width - 4
	if available < 16 {
		available = 16
	}
	if compact {
		minName := 5
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

	// ALIAS column (non-compact only): shows each service's in-cluster hostname
	// in full so it can be read and copied. The column is shown only when the
	// longest alias fits entirely in the room left inside the box (width-4
	// usable) — on a narrower terminal it is dropped rather than truncated.
	maxAliasLen := 0
	for i := range services {
		if w := lipgloss.Width(services[i].Alias); w > maxAliasLen {
			maxAliasLen = w
		}
	}
	usedBase := 2 + nameCellWidth + 2 + statusWidth + 2 + uptimeWidth + 2 + portWidth + 2 + restartWidth
	roomForAlias := (width - 4) - usedBase - 2
	aliasWidth := maxAliasLen
	showAlias := !compact && maxAliasLen > 0 && roomForAlias >= maxAliasLen

	statusHeader := "STATUS"
	if narrow {
		statusHeader = "ST"
	}
	headerLine := headerPrefix + padRightDisplayWidth(truncateRunes("SERVICE", nameCellWidth), nameCellWidth) + fmt.Sprintf(
		"  %-*s",
		statusWidth, statusHeader,
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
		if showAlias {
			headerLine += fmt.Sprintf("  %-*s", aliasWidth, "ALIAS")
		}
	}
	header := lipgloss.NewStyle().
		Foreground(colorHeading).
		Bold(true).
		Render(headerLine)
	rows = append(rows, header)

	sepWidth := width - 6
	if sepWidth < 10 {
		sepWidth = 10
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
		if narrow {
			status = fmt.Sprintf("%-*s", statusWidth, statusIcon)
		}
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
			if showAlias {
				styledAlias := lipgloss.NewStyle().
					Foreground(nameColor).
					Render(svc.Alias)
				row += "  " + styledAlias
			}
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
		Width(width)

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
		if nameWidth < 4 {
			nameWidth = 4
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
		if availableWidth < 4 {
			availableWidth = 4
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

type helpItem struct{ key, description string }

// helpLines lays shortcuts out as a responsive row-major grid. It tries the
// largest possible column count first, so a wide terminal gets one row while a
// narrow terminal gets aligned columns and additional rows.
func helpLines(width int, logScope string, copyEnabled bool) []string {
	if width < 8 {
		width = 8
	}

	var items []helpItem
	if width < 50 {
		logScope = truncateRunes(logScope, 8)
		items = []helpItem{
			{"↑↓", "choose service"},
			{"L/Ctrl+L", "view / clear logs"},
			{"a", "add/edit services"},
			{"c", "edit config"},
			{"r", "restart selected"},
			{"Ctrl+R", "restart all"},
			{"s", "stop selected"},
			{"q", "quit & stop all"},
		}
	} else if width < 90 {
		items = []helpItem{
			{"↑↓", "choose service"},
			{"L / Ctrl+L", "switch view / clear logs (now " + logScope + ")"},
			{"a", "add/edit services"},
			{"c", "edit config file"},
			{"r", "restart selected"},
			{"Ctrl+R", "restart all"},
			{"s", "stop selected"},
			{"q", "quit & stop all"},
		}
	} else {
		items = []helpItem{
			{"↑↓/j/k", "choose service"},
			{"L / Ctrl+L", "switch view / clear logs (now " + logScope + ")"},
			{"a", "add/edit services"},
			{"c", "edit config file"},
			{"r", "restart selected"},
			{"Ctrl+R", "restart all"},
			{"s", "stop selected"},
			{"q", "quit & stop all"},
		}
	}

	// The copy (y) hint appears only when cluster-host aliasing is on — no alias,
	// no copy. Inserted right after the combined log-controls cell.
	if copyEnabled {
		withCopy := make([]helpItem, 0, len(items)+1)
		withCopy = append(withCopy, items[:2]...)
		withCopy = append(withCopy, helpItem{"y", "copy alias"})
		withCopy = append(withCopy, items[2:]...)
		items = withCopy
	}

	return helpItemLines(width, items)
}

func helpItemLines(width int, items []helpItem) []string {
	if width < 8 {
		width = 8
	}
	keyStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(colorMuted)

	n := len(items)
	styled := make([]string, n)
	widths := make([]int, n)
	// The box spans the terminal width; reserve cells only for its border and
	// horizontal padding when calculating the responsive grid.
	inner := width - 4
	if inner < 4 {
		inner = 4
	}
	for i, item := range items {
		keyWidth := lipgloss.Width(item.key)
		maxDescWidth := inner - keyWidth - 2 // ": " between key and description
		description := truncateRunes(item.description, maxDescWidth)
		styled[i] = keyStyle.Render(item.key) + descStyle.Render(":")
		widths[i] = keyWidth + 1
		if description != "" {
			styled[i] += descStyle.Render(" " + description)
			widths[i] += 1 + lipgloss.Width(description)
		}
	}

	grid := helpGridLines(styled, widths, inner)
	if len(grid) < 2 {
		return grid
	}

	// A blank line between grid rows keeps neighboring actions visually
	// distinct without adding internal borders or hardcoded decoration.
	spaced := make([]string, 0, len(grid)*2-1)
	for i, line := range grid {
		if i > 0 {
			spaced = append(spaced, "")
		}
		spaced = append(spaced, line)
	}
	return spaced
}

func renderHelp(width int, logScope string, copyEnabled bool) string {
	return renderHelpBox(width, helpLines(width, logScope, copyEnabled))
}

func renderHelpBox(width int, lines []string) string {
	boxWidth := width
	if boxWidth < 12 {
		boxWidth = 12
	}
	help := strings.Join(lines, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(boxWidth).
		Render(help)
}

func helpGridLines(styled []string, widths []int, inner int) []string {
	n := len(styled)
	if n == 0 {
		return nil
	}
	const gap = 2

	for columns := n; columns >= 1; columns-- {
		columnWidths := make([]int, columns)
		for i, itemWidth := range widths {
			column := i % columns
			if itemWidth > columnWidths[column] {
				columnWidths[column] = itemWidth
			}
		}
		totalWidth := gap * (columns - 1)
		for _, columnWidth := range columnWidths {
			totalWidth += columnWidth
		}
		if totalWidth > inner {
			continue
		}

		rows := (n + columns - 1) / columns
		lines := make([]string, 0, rows)
		for row := 0; row < rows; row++ {
			var line strings.Builder
			for column := 0; column < columns; column++ {
				index := row*columns + column
				if index >= n {
					break
				}
				line.WriteString(styled[index])
				if index+1 < n && column+1 < columns {
					line.WriteString(strings.Repeat(" ", columnWidths[column]-widths[index]+gap))
				}
			}
			lines = append(lines, line.String())
		}
		return lines
	}

	return styled
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
