package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/icons"
)

func (u *UI) renderManageGroupRow(name string, cursorOn bool, maxNameLen int, running map[string]bool) string {
	highlight := "  "
	if cursorOn {
		highlight = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("► ")
	}
	box := u.renderSelectCheckbox(u.manageSelGroups[name])

	nameColor := colorText
	if cursorOn {
		nameColor = colorAccent
	}
	styledName := lipgloss.NewStyle().Foreground(nameColor).Bold(true).
		Render(padRightDisplayWidth(truncateRunes(name, maxNameLen), maxNameLen))

	members := u.manageGroups[name]
	run := 0
	for _, svc := range members {
		if running[svc] {
			run++
		}
	}
	info := lipgloss.NewStyle().Foreground(colorMuted).
		Render(fmt.Sprintf("%s  %d/%d running", summarizeMembers(members), run, len(members)))

	icon := u.overlayIconCell(u.manageIcons.set.ForGroup())

	return highlight + box + " " + icon + styledName + "  " + info
}

func (u *UI) renderManageServiceRow(name string, cursorOn bool, maxNameLen int, running map[string]bool) string {
	highlight := "  "
	if cursorOn {
		highlight = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("► ")
	}

	nameColor := colorText
	if cursorOn {
		nameColor = colorAccent
	}
	styledName := lipgloss.NewStyle().Foreground(nameColor).Bold(true).
		Render(padRightDisplayWidth(truncateRunes(name, maxNameLen), maxNameLen))

	var box, indicator string
	if running[name] {
		box = lipgloss.NewStyle().Foreground(colorMuted).Render("   ")
		indicator = lipgloss.NewStyle().Foreground(colorAccentAlt).Render("● running")
	} else {
		box = u.renderSelectCheckbox(u.manageSelSvcs[name])
		indicator = lipgloss.NewStyle().Foreground(colorMuted).Render("○ stopped")
	}

	icon := u.overlayIconCell(u.manageIcons.set.ForPort(u.manageIcons.ports[name]))

	return highlight + box + " " + icon + styledName + "  " + indicator
}

// renderSelectCheckbox draws a multi-select checkbox, brightening to the accent
// color when ticked so selected rows read at a glance.
func (u *UI) renderSelectCheckbox(selected bool) string {
	if selected {
		return lipgloss.NewStyle().Foreground(colorAccentAlt).Bold(true).Render("[✓]")
	}
	return lipgloss.NewStyle().Foreground(colorMuted).Render("[ ]")
}

// overlayIconCell renders the leading icon cell for an overlay row from an
// already-resolved icon, or an empty string when icons are disabled.
func (u *UI) overlayIconCell(icon icons.Icon) string {
	if !u.manageIcons.enabled {
		return ""
	}
	return renderIconCell(icon.Glyph, icon.Color)
}

func (u *UI) renderManageOverlay() string {
	width := u.width
	if width <= 0 {
		width = 120
	}
	if width < 60 {
		width = 60
	}

	running := u.runningNameSet()

	maxNameLen := 7
	for _, n := range u.manageGroupNames {
		if len(n) > maxNameLen {
			maxNameLen = len(n)
		}
	}
	for _, n := range u.manageServices {
		if len(n) > maxNameLen {
			maxNameLen = len(n)
		}
	}
	if maxNameLen > 30 {
		maxNameLen = 30
	}

	u.ensureManageVisible()
	visible := u.manageVisibleRows()
	start := u.manageOffset
	end := start + visible
	if end > len(u.manageRows) {
		end = len(u.manageRows)
	}
	if start > end {
		start = end
	}

	rows := make([]string, 0, end-start+3)
	rows = append(rows, lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("ADD / EDIT")+
		lipgloss.NewStyle().Foreground(colorMuted).Render("  — groups & services"))
	rows = append(rows, u.renderManageSearchLine())
	for i := start; i < end; i++ {
		row := u.manageRows[i]
		cursorOn := i == u.manageCursor
		switch row.kind {
		case rowHeaderGroups:
			rows = append(rows, lipgloss.NewStyle().Foreground(colorHeading).Bold(true).Render("GROUPS"))
		case rowHeaderServices:
			rows = append(rows, lipgloss.NewStyle().Foreground(colorHeading).Bold(true).Render("SERVICES"))
		case rowEmptyGroups:
			text := "  (no groups — ^n to create)"
			if u.manageSearch != "" {
				text = "  (no matching groups)"
			}
			rows = append(rows, lipgloss.NewStyle().Foreground(colorMuted).Italic(true).Render(text))
		case rowEmptyServices:
			text := "  (no services — ^n to create)"
			if u.manageSearch != "" {
				text = "  (no matching services)"
			}
			rows = append(rows, lipgloss.NewStyle().Foreground(colorMuted).Italic(true).Render(text))
		case rowGroup:
			rows = append(rows, u.renderManageGroupRow(row.name, cursorOn, maxNameLen, running))
		case rowService:
			rows = append(rows, u.renderManageServiceRow(row.name, cursorOn, maxNameLen, running))
		}
	}

	if len(u.manageRows) > visible {
		rows = append(rows, lipgloss.NewStyle().Foreground(colorMuted).
			Render(fmt.Sprintf("(%d–%d of %d)", start+1, end, len(u.manageRows))))
	}

	table := lipgloss.JoinVertical(lipgloss.Left, rows...)
	overlayBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(width - 2).
		Render(table)

	sections := []string{overlayBox}

	switch {
	case u.manageNewPrompt:
		promptText := lipgloss.NewStyle().Foreground(colorAccentAlt).Bold(true).Render("Create new —")
		promptKeys := renderActionChips([][2]string{{"g", "group"}, {"s", "service"}, {"esc", "cancel"}})
		promptBody := lipgloss.JoinVertical(lipgloss.Left, promptText, "", promptKeys)
		sections = append(sections, lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1).
			Width(width-2).
			Render(promptBody))
	case u.manageConfirmDelete != "":
		msg := fmt.Sprintf("Delete service '%s'? This cannot be undone.", u.manageConfirmDelete)
		if u.manageConfirmKind == "group" {
			msg = fmt.Sprintf("Delete group '%s'? Member services are kept.", u.manageConfirmDelete)
		}
		confirmText := lipgloss.NewStyle().Foreground(colorWarn).Bold(true).Render(msg)
		confirmKeys := renderActionChips([][2]string{{"y", "confirm"}, {"n", "cancel"}})
		confirmBody := lipgloss.JoinVertical(lipgloss.Left, confirmText, "", confirmKeys)
		sections = append(sections, lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1).
			Width(width-2).
			Render(confirmBody))
	case u.manageErr != "":
		sections = append(sections, lipgloss.NewStyle().Foreground(colorError).Render("✗ "+u.manageErr))
	case u.manageInfo != "":
		sections = append(sections, lipgloss.NewStyle().Foreground(colorAccentAlt).Bold(true).Render(u.manageInfo))
	}

	sections = append(sections, renderActionChips([][2]string{
		{"type", "search"},
		{"↑↓", "navigate"},
		{"Space", "select"},
		{"Enter", "run"},
		{"^n", "new"},
		{"^e", "edit"},
		{"^d", "delete"},
		{"^c", "config"},
		{"Esc", "clear/close"},
	}))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderManageSearchLine renders the always-focused search input shown at the top
// of the manage overlay. Typing anywhere in the overlay feeds this query.
func (u *UI) renderManageSearchLine() string {
	label := lipgloss.NewStyle().Foreground(colorMuted).Render("Search: ")
	cursor := lipgloss.NewStyle().Foreground(colorAccent).Render("▏")
	if u.manageSearch == "" {
		placeholder := lipgloss.NewStyle().Foreground(colorMuted).Italic(true).Render("type to filter…")
		return label + cursor + placeholder
	}
	query := lipgloss.NewStyle().Foreground(colorAccentAlt).Bold(true).Render(u.manageSearch)
	return label + query + cursor
}

func summarizeMembers(members []string) string {
	if len(members) == 0 {
		return "(empty)"
	}
	joined := strings.Join(members, ", ")
	if len(joined) > 48 {
		return fmt.Sprintf("%d services", len(members))
	}
	return joined
}
