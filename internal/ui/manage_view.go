package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/icons"
)

// The Hangar: every saved group and service, searchable, with multi-select to
// launch. Rendering only — keys and state live in manage.go.

func (u *UI) cursorBar(on bool) string {
	if on {
		return fg(colorAccent).Bold(true).Render("▎ ")
	}
	return "  "
}

func (u *UI) renderManageGroupRow(name string, cursorOn bool, maxNameLen int, running map[string]bool) string {
	nameStyle := lipgloss.NewStyle().Foreground(colorHeading).Bold(true)
	if cursorOn {
		nameStyle = nameStyle.Foreground(colorAccent)
	}

	members := u.manage.groups[name]
	run := 0
	for _, svc := range members {
		if running[svc] {
			run++
		}
	}
	count := fg(colorMuted).Render(fmt.Sprintf("%d/%d running", run, len(members)))
	if run > 0 {
		count = fg(statusHealthyColor).Render(fmt.Sprintf("%d", run)) + fg(colorMuted).Render(fmt.Sprintf("/%d running", len(members)))
	}

	return u.cursorBar(cursorOn) +
		u.renderSelectCheckbox(u.manage.selectedGroups[name]) + " " +
		u.overlayIconCell(u.manage.icons.set.ForGroup()) +
		nameStyle.Render(fitWidth(truncateRunes(name, maxNameLen), maxNameLen)) + "   " +
		count + fg(colorBorder).Render("   ") + fg(colorMuted).Render(summarizeMembers(members))
}

func (u *UI) renderManageServiceRow(name string, cursorOn bool, maxNameLen int, running map[string]bool) string {
	nameStyle := lipgloss.NewStyle().Foreground(colorText)
	if cursorOn {
		nameStyle = nameStyle.Foreground(colorAccent).Bold(true)
	}

	mark, state := u.renderSelectCheckbox(u.manage.selectedServices[name]), fg(colorMuted).Render("○ stopped")
	if running[name] {
		// Already running: nothing to launch, so no checkbox.
		mark, state = fg(colorBorder).Render("·"), fg(statusHealthyColor).Render("● running")
	}

	port := ""
	if p := u.manage.icons.localPorts[name]; p != "" {
		port = ":" + p
	}

	return u.cursorBar(cursorOn) + mark + " " +
		u.overlayIconCell(u.manage.icons.set.ForPort(u.manage.icons.ports[name])) +
		nameStyle.Render(fitWidth(truncateRunes(name, maxNameLen), maxNameLen)) + "   " +
		fg(colorText).Render(fitWidth(port, 6)) + "   " + state
}

// renderSelectCheckbox is a row's launch mark: ◉ when selected, ○ otherwise.
func (u *UI) renderSelectCheckbox(selected bool) string {
	if selected {
		return fg(colorAccentAlt).Bold(true).Render("◉")
	}
	return fg(colorMuted).Render("○")
}

// overlayIconCell renders the leading icon cell for an overlay row from an
// already-resolved icon, or an empty string when icons are disabled.
func (u *UI) overlayIconCell(icon icons.Icon) string {
	if !u.manage.icons.enabled {
		return ""
	}
	return renderIconCell(icon.Glyph, icon.Color)
}

func (u *UI) manageSelectionCount() int {
	n := 0
	for _, on := range u.manage.selectedGroups {
		if on {
			n++
		}
	}
	for _, on := range u.manage.selectedServices {
		if on {
			n++
		}
	}
	return n
}

func (u *UI) renderManageOverlay() string {
	width := max(24, u.width)
	height := u.height
	if height <= 0 {
		height = 30
	}
	inner := max(1, width-2*len(margin))
	running := u.runningNameSet()

	headerRight := ""
	if n := u.manageSelectionCount(); n > 0 {
		headerRight = fg(colorAccentAlt).Bold(true).Render(fmt.Sprintf("◉ %d selected", n)) +
			fg(colorMuted).Render(" · enter to launch")
	}

	keys := keyBar(manageHelpItems(), manageCompactHelpItems(), inner, u.manageStatus())
	context := u.renderManageContext(width)
	frameH := max(5, height-2-len(context)-len(keys))

	// Frame body: search line, spacer, list (+ a range line when it scrolls).
	listRows := max(1, frameH-2-2)
	if len(u.manage.rows) > listRows && listRows > 1 {
		listRows--
	}
	u.ensureManageVisible(listRows)
	start := u.manage.offset
	end := min(len(u.manage.rows), start+listRows)

	maxNameLen := 8
	for _, n := range append(append([]string{}, u.manage.groupNames...), u.manage.serviceNames...) {
		maxNameLen = max(maxNameLen, lipgloss.Width(n))
	}
	maxNameLen = min(maxNameLen, 28)

	body := []string{u.renderManageSearchLine(), ""}
	frameInner := width - 4
	for i := start; i < end; i++ {
		row := u.manage.rows[i]
		cursorOn := i == u.manage.cursor
		switch row.kind {
		case rowHeaderGroups:
			body = append(body, hudSection("GROUPS", frameInner))
		case rowHeaderServices:
			body = append(body, hudSection("SERVICES", frameInner))
		case rowEmptyGroups:
			body = append(body, u.emptyRowHint("no groups yet — Ctrl+N to create one", "no matching groups"))
		case rowEmptyServices:
			body = append(body, u.emptyRowHint("no services yet — Ctrl+N to create one", "no matching services"))
		case rowGroup:
			body = append(body, u.renderManageGroupRow(row.name, cursorOn, maxNameLen, running))
		case rowService:
			body = append(body, u.renderManageServiceRow(row.name, cursorOn, maxNameLen, running))
		}
	}
	if len(u.manage.rows) > listRows {
		body = append(body, fg(colorMuted).Render(fmt.Sprintf("%d–%d of %d", start+1, end, len(u.manage.rows))))
	}

	badge := fg(colorMuted).Render(fmt.Sprintf("%d groups · %d services", len(u.manage.groupNames), len(u.manage.serviceNames)))
	lines := []string{u.hudHeader("HANGAR", headerRight), ""}
	lines = append(lines, hudFrame(hudTitle("INVENTORY", ""), badge, body, width, frameH, toneNormal)...)
	lines = append(lines, context...)
	lines = append(lines, keys...)
	return screen(lines, width, height)
}

func (u *UI) emptyRowHint(empty, noMatch string) string {
	text := empty
	if u.manage.searchQuery != "" {
		text = noMatch
	}
	return "  " + fg(colorMuted).Italic(true).Render(text)
}

// manageStatus is the transient message shown at the end of the key bar.
func (u *UI) manageStatus() string {
	switch {
	case u.manage.errorMsg != "":
		return "✗ " + u.manage.errorMsg
	case u.manage.infoMsg != "":
		return u.manage.infoMsg
	}
	return ""
}

func manageHelpItems() [][2]string {
	return [][2]string{
		{"type", "search"},
		{"↑↓", "choose"},
		{"Space", "select"},
		{"Enter", "launch selected"},
		{"Ctrl+N", "create"},
		{"Ctrl+E", "edit"},
		{"Ctrl+D", "delete"},
		{"Ctrl+C", "edit config file"},
		{"Esc", "clear search / back"},
		{"?", "all shortcuts"},
	}
}

func manageCompactHelpItems() [][2]string {
	return [][2]string{
		{"↑↓", "move"},
		{"Space", "select"},
		{"Enter", "launch"},
		{"^N", "new"},
		{"^E", "edit"},
		{"^D", "delete"},
		{"^C", "config"},
		{"Esc", "back"},
		{"?", "help"},
	}
}

// renderManageContext is the prompt or confirmation shown between the
// inventory and the key bar; nil when there is none.
func (u *UI) renderManageContext(width int) []string {
	switch {
	case u.manage.showNewPrompt:
		body := []string{
			fg(colorText).Render("What do you want to create?") + "   " +
				actionKeys([][2]string{{"g", "group"}, {"s", "service"}, {"esc", "cancel"}}),
		}
		return hudFrame(hudTitle("CREATE", ""), "", body, width, 0, toneNormal)
	case u.manage.confirmDeleteName != "":
		msg := fmt.Sprintf("Delete service '%s'? This cannot be undone.", u.manage.confirmDeleteName)
		if u.manage.confirmDeleteKind == "group" {
			msg = fmt.Sprintf("Delete group '%s'? Its services are kept.", u.manage.confirmDeleteName)
		}
		body := []string{
			fg(colorWarn).Bold(true).Render(msg) + "   " +
				actionKeys([][2]string{{"y", "delete"}, {"n", "cancel"}}),
		}
		return hudFrame(lipgloss.NewStyle().Foreground(colorWarn).Bold(true).Render("CONFIRM"), "", body, width, 0, toneAlert)
	}
	return nil
}

// actionKeys renders inline key choices: "g group   s service".
func actionKeys(pairs [][2]string) string {
	key := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	chips := make([]string, 0, len(pairs))
	for _, p := range pairs {
		chips = append(chips, key.Render(p[0])+" "+fg(colorMuted).Render(p[1]))
	}
	return strings.Join(chips, "   ")
}

// renderManageSearchLine is the always-focused scanner input at the top of the
// inventory. Typing anywhere in the Hangar feeds this query.
func (u *UI) renderManageSearchLine() string {
	label := fg(colorAccent).Render("⌕ ") + lipgloss.NewStyle().Foreground(colorHeading).Bold(true).Render("SCAN") + fg(colorBorder).Render("  ▸ ")
	cursor := fg(colorAccent).Render("▏")
	if u.manage.searchQuery == "" {
		return label + cursor + fg(colorMuted).Italic(true).Render("type to filter groups and services…")
	}
	return label + fg(colorAccentAlt).Bold(true).Render(u.manage.searchQuery) + cursor
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
