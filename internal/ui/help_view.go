package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// The Flight Manual: every shortcut of the current page, by category.

type shortcutSection struct {
	title string
	items []helpItem
}

func (u *UI) renderShortcutOverlay(manage bool) string {
	width := max(24, u.width)
	height := max(6, u.height)
	inner := max(1, width-2*len(margin))
	frameInner := width - 4

	sections := u.dashboardShortcutSections()
	page := "mission control"
	if manage {
		sections = manageShortcutSections()
		page = "hangar"
	}

	keys := keyBar([][2]string{{"?", "close"}, {"Esc", "close"}}, nil, inner, "")
	frameH := max(3, height-2-len(keys))

	body := append([]string{""}, strings.Split(renderShortcutSections(sections, frameInner), "\n")...)
	if len(body) > frameH-2 {
		// Too short for the categorized layout: the same keys as a dense grid.
		body = helpItemLinesWithSpacing(frameInner+4, flattenSections(sections), false)
		if len(body) > frameH-2 && frameH > 2 {
			body = body[:frameH-2]
			body[len(body)-1] = fg(colorWarn).Render(truncateRunes("… enlarge the terminal for every shortcut", frameInner))
		}
	}

	lines := []string{u.hudHeader("FLIGHT MANUAL", ""), ""}
	lines = append(lines, hudFrame(hudTitle("SHORTCUTS", page), "", body, width, frameH, toneNormal)...)
	lines = append(lines, keys...)
	return screen(lines, width, height)
}

func flattenSections(sections []shortcutSection) []helpItem {
	var items []helpItem
	for _, s := range sections {
		items = append(items, s.items...)
	}
	return items
}

func (u *UI) dashboardShortcutSections() []shortcutSection {
	serviceItems := []helpItem{
		{"R", "restart selected service"},
		{"Ctrl+R", "restart all services"},
		{"S", "stop selected service"},
		{"A", "add or edit services & groups"},
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
			{"Q / Ctrl+C", "quit & stop all"},
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

// renderShortcutSection is one category: a divider heading, then aligned
// key → description rows.
func renderShortcutSection(section shortcutSection, width int) string {
	keyWidth := 0
	for _, item := range section.items {
		keyWidth = max(keyWidth, lipgloss.Width(item.key))
	}
	keyStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	lines := []string{hudSection(section.title, width)}
	for _, item := range section.items {
		description := truncateRunes(item.description, max(1, width-keyWidth-3))
		lines = append(lines, keyStyle.Render(fitWidth(item.key, keyWidth))+"   "+fg(colorMuted).Render(description))
	}
	return strings.Join(lines, "\n")
}
