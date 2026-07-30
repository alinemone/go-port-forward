package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

type shortcutSection struct {
	title string
	items []helpItem
}

func (u *UI) renderShortcutOverlay(manage bool) string {
	width, height := u.width, u.height
	if width < 12 {
		width = 12
	}
	if height < 5 {
		height = 5
	}

	sections := u.dashboardShortcutSections()
	if manage {
		sections = manageShortcutSections()
	}

	title := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("SHORTCUTS")
	closeHint := lipgloss.NewStyle().Foreground(colorMuted).Render("? / Esc: close")
	innerWidth := width - 6 // border + horizontal padding
	if innerWidth < 6 {
		innerWidth = 6
	}

	categorized := lipgloss.JoinVertical(lipgloss.Left, title, closeHint, "", renderShortcutSections(sections, innerWidth))
	var content string
	// Choose the dense overlay from its measured height, not a fixed terminal
	// breakpoint; two-column layouts often fit categories at the same height
	// where a narrow single-column layout does not.
	tiny := lipgloss.Height(categorized)+4 > height // vertical padding + border
	if tiny {
		// Extremely short splits prioritize density. The same complete key set is
		// shown as an unspaced grid, without category headings consuming rows.
		items := compactOverlayItems(manage, u.aliasEnabled)
		grid := helpItemLinesWithSpacing(innerWidth+4, items, false)
		spacedGrid := helpItemLinesWithSpacing(innerWidth+4, items, true)
		// Prefer breathing room even in the compact overlay whenever it still
		// fits; only genuinely short splits fall back to dense rows.
		if len(spacedGrid)+3 <= height {
			grid = spacedGrid
		}
		tinyHeader := title + lipgloss.NewStyle().Foreground(colorMuted).Render("  ?: close")
		if lipgloss.Width(tinyHeader) > innerWidth {
			tinyHeader = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("HELP")
		}
		content = lipgloss.JoinVertical(lipgloss.Left, tinyHeader, strings.Join(grid, "\n"))
	} else {
		content = categorized
	}

	verticalPadding := 1
	if tiny {
		verticalPadding = 0
		content = fitHelpOverlayHeight(content, height-2, innerWidth)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(verticalPadding, 2).
		Width(width).
		Height(height).
		Render(content)
}

func compactOverlayItems(manage, aliasEnabled bool) []helpItem {
	if manage {
		return manageHelpItems()
	}
	items := []helpItem{
		{"↑↓ / J K", "choose service"},
		{"PgUp / PgDn", "scroll logs"},
		{"L / Ctrl+L", "switch / clear logs"},
		{"R", "restart selected"},
		{"Ctrl+R", "restart all"},
		{"S", "stop selected"},
		{"A", "add/edit services"},
		{"C", "edit config"},
		{"Q", "quit & stop all"},
	}
	if aliasEnabled {
		items = append(items, helpItem{"Y", "copy alias"})
	}
	return items
}

func fitHelpOverlayHeight(content string, maxLines, width int) string {
	if maxLines < 1 {
		maxLines = 1
	}
	lines := strings.Split(content, "\n")
	if len(lines) <= maxLines {
		return content
	}
	lines = lines[:maxLines]
	message := truncateRunes("… enlarge terminal for more shortcuts", width)
	lines[maxLines-1] = lipgloss.NewStyle().Foreground(colorWarn).Render(message)
	return strings.Join(lines, "\n")
}

func (u *UI) dashboardShortcutSections() []shortcutSection {
	serviceItems := []helpItem{
		{"R", "restart selected service"},
		{"Ctrl+R", "restart all services"},
		{"S", "stop selected service"},
		{"A", "add or edit services"},
	}
	if u.aliasEnabled {
		serviceItems = append(serviceItems, helpItem{"Y", "copy selected alias"})
	}
	return []shortcutSection{
		{title: "NAVIGATION", items: []helpItem{
			{"↑↓ / J K", "choose service"},
			{"PgUp / PgDn", "scroll logs"},
			{"Home / End", "first / latest log"},
		}},
		{title: "LOGS", items: []helpItem{
			{"L", "switch all / selected logs"},
			{"Ctrl+L", "clear visible logs"},
		}},
		{title: "SERVICES", items: serviceItems},
		{title: "APPLICATION", items: []helpItem{
			{"C", "edit configuration file"},
			{"Q / Esc", "quit & stop all"},
		}},
	}
}

func manageShortcutSections() []shortcutSection {
	return []shortcutSection{
		{title: "NAVIGATION", items: []helpItem{
			{"type", "search the list"},
			{"↑↓", "choose item"},
			{"Esc", "clear search / go back"},
		}},
		{title: "SELECTION", items: []helpItem{
			{"Space", "select or unselect item"},
			{"Enter", "start selected services"},
		}},
		{title: "MANAGEMENT", items: []helpItem{
			{"Ctrl+N", "create item"},
			{"Ctrl+E", "edit current item"},
			{"Ctrl+D", "delete current item"},
		}},
		{title: "CONFIGURATION", items: []helpItem{
			{"Ctrl+C", "edit config file"},
		}},
	}
}

func renderShortcutSections(sections []shortcutSection, width int) string {
	if width >= 64 {
		gap := 5
		columnWidth := (width - gap) / 2
		rows := make([]string, 0, (len(sections)+1)/2)
		for i := 0; i < len(sections); i += 2 {
			left := lipgloss.NewStyle().Width(columnWidth).Render(renderShortcutSection(sections[i], columnWidth))
			right := ""
			if i+1 < len(sections) {
				right = renderShortcutSection(sections[i+1], columnWidth)
			}
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), right))
		}
		return strings.Join(rows, "\n\n")
	}

	blocks := make([]string, 0, len(sections))
	for _, section := range sections {
		blocks = append(blocks, renderShortcutSection(section, width))
	}
	return strings.Join(blocks, "\n\n")
}

func renderShortcutSection(section shortcutSection, width int) string {
	heading := lipgloss.NewStyle().Foreground(colorHeading).Bold(true).Render(section.title)
	keyStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(colorMuted)
	lines := []string{heading, ""}
	for _, item := range section.items {
		maxDescription := width - lipgloss.Width(item.key) - 2
		description := truncateRunes(item.description, maxDescription)
		lines = append(lines, keyStyle.Render(item.key)+descStyle.Render(": "+description))
	}
	return strings.Join(lines, "\n")
}
