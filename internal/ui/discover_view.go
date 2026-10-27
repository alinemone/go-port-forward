package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/kube"
)

// renderDiscover draws the active wizard step under a stepper header.
func (u *UI) renderDiscover() string {
	width := max(40, u.width)
	height := u.height
	if height <= 0 {
		height = 24
	}
	inner := max(1, width-2*len(margin))
	frameInner := width - 4
	d := &u.discover

	var title, badge string
	var body []string
	var keys [][2]string
	status := ""

	switch d.step {
	case stepTarget:
		title, body = hudTitle("TARGET", "where to scan"), u.renderDiscoverTarget(frameInner)
		keys = [][2]string{{"type", "namespace"}, {"↑↓", "suggestions"}, {"Tab", "complete"}, {"Ctrl+T", "switch context"}, {"Enter", "scan"}, {"Esc", "cancel"}}
	case stepSelect:
		title = hudTitle("SELECT", d.namespace)
		badge = fg(colorMuted).Render(fmt.Sprintf("%d services", len(d.candidates)))
		body = u.renderDiscoverSelect(frameInner, height)
		keys = [][2]string{{"↑↓", "move"}, {"Space", "select"}, {"Enter", "review"}, {"Esc", "back"}}
	case stepReview:
		title = hudTitle("REVIEW", fmt.Sprintf("%d service%s", len(d.items), plural(len(d.items))))
		body = u.renderDiscoverReview(frameInner, height)
		keys = [][2]string{{"Tab/↑↓", "next field"}, {"Enter", "add all"}, {"Esc", "back"}}
		if d.errorMsg != "" {
			status = "✗ " + d.errorMsg
		}
	}

	keyLines := keyBar(keys, nil, inner, status)
	frameH := max(4, height-2-len(keyLines))
	if room := frameH - 2 - len(body); room > 1 {
		body = append(append(body, ""), starfield(frameInner, room-1, 13, u.spinnerFrame, "")...)
	}

	lines := []string{u.hudHeader("DISCOVER", u.discoverStepper()), ""}
	lines = append(lines, hudFrame(title, badge, body, width, frameH, toneNormal)...)
	lines = append(lines, keyLines...)
	return screen(lines, width, height)
}

// discoverStepper shows where the wizard is: ✓ done, ● current, ○ next.
func (u *UI) discoverStepper() string {
	steps := []string{"TARGET", "SELECT", "REVIEW"}
	parts := make([]string, len(steps))
	for i, s := range steps {
		switch {
		case discoverStep(i) < u.discover.step:
			parts[i] = fg(statusHealthyColor).Render("✓ " + s)
		case discoverStep(i) == u.discover.step:
			parts[i] = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("● " + s)
		default:
			parts[i] = fg(colorMuted).Render("○ " + s)
		}
	}
	return strings.Join(parts, fg(colorBorder).Render(" ── "))
}

func (u *UI) renderDiscoverTarget(width int) []string {
	d := &u.discover
	ctx := d.currentContext
	if c := u.kubeContext(); c != "" {
		ctx = c
	}
	if ctx == "" {
		ctx = "(current)"
	}
	ctxLine := fg(colorMuted).Render("  CONTEXT     ") + fg(colorAccent).Render("◂ ") +
		lipgloss.NewStyle().Foreground(colorText).Bold(true).Render(ctx) + fg(colorAccent).Render(" ▸")
	if len(d.contexts) > 1 {
		ctxLine += fg(colorBorder).Render("   Ctrl+T to switch")
	}

	lines := []string{
		"",
		ctxLine,
		"",
		formLabel("NAMESPACE", true, "the namespace whose services to scan"),
		formInput(d.nsInput.View(), true),
	}

	suggestions := u.nsSuggestions(maxNSSuggestions)
	switch {
	case d.nsError != "" && !d.nsFromSaved:
		lines = append(lines, "    "+fg(colorMuted).Italic(true).Render("no namespace suggestions ("+truncateRunes(d.nsError, width-30)+") — type it in full"))
	case d.namespaces == nil:
		lines = append(lines, "    "+fg(colorMuted).Render(u.spinner()+" loading namespaces…"))
	case len(suggestions) == 0:
		lines = append(lines, "    "+fg(colorMuted).Italic(true).Render("no namespace matches"))
	default:
		if d.nsFromSaved {
			lines = append(lines, "    "+fg(colorBorder).Render("from your saved services — the cluster doesn't let you list namespaces"))
		}
		for i, ns := range suggestions {
			if i == d.suggestion {
				lines = append(lines, "  "+u.cursorBar(true)+lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(ns))
			} else {
				lines = append(lines, "    "+fg(colorMuted).Render(ns))
			}
		}
	}
	return lines
}

func (u *UI) renderDiscoverSelect(width, height int) []string {
	d := &u.discover
	switch {
	case d.loading:
		return append([]string{""}, starfield(width, 5, 17, u.spinnerFrame,
			fg(colorAccent).Render(u.spinner()+" ")+fg(colorText).Render("scanning "+d.namespace+"…"))...)
	case d.scanError != "":
		return []string{"", fg(colorError).Bold(true).Render("✗ " + d.scanError), "", fg(colorMuted).Render("Esc to go back and pick another namespace")}
	case len(d.candidates) == 0:
		return []string{"", fg(colorMuted).Italic(true).Render("no forwardable services in " + d.namespace)}
	}

	// Section headers interleave with candidates; build them, then window
	// the result around the cursor.
	type line struct {
		text   string
		cursor bool
	}
	nameWidth := 0
	for _, c := range d.candidates {
		nameWidth = max(nameWidth, lipgloss.Width(c.Service))
	}
	nameWidth = min(nameWidth, max(16, width-40))

	var all []line
	tier := kube.Tier(-1)
	for i, c := range d.candidates {
		if c.Tier != tier {
			tier = c.Tier
			all = append(all, line{text: hudSection(tierTitle(tier), width)})
		}
		all = append(all, line{text: u.renderCandidate(i, c, nameWidth), cursor: i == d.cursor})
	}

	rows := max(3, height-8)
	cursorAt := 0
	for i, l := range all {
		if l.cursor {
			cursorAt = i
		}
	}
	start := 0
	if cursorAt >= rows {
		start = cursorAt - rows + 1
	}
	end := min(len(all), start+rows)

	out := []string{fg(colorText).Render(fmt.Sprintf("%d selected", u.discoverSelectionCount())) +
		fg(colorMuted).Render("  ·  suggested data stores are pre-selected")}
	for _, l := range all[start:end] {
		out = append(out, l.text)
	}
	if len(all) > rows {
		out = append(out, fg(colorMuted).Render(fmt.Sprintf("%d–%d of %d", start+1, end, len(all))))
	}
	return out
}

func tierTitle(t kube.Tier) string {
	switch t {
	case kube.TierSuggested:
		return "SUGGESTED"
	case kube.TierMore:
		return "MORE DATA STORES"
	}
	return "OTHER SERVICES"
}

func (u *UI) renderCandidate(i int, c kube.Candidate, nameWidth int) string {
	d := &u.discover
	cursor := i == d.cursor
	savedAs := d.saved[d.namespace+"/"+c.Service]

	mark := u.renderSelectCheckbox(d.selected[i])
	if savedAs != "" {
		mark = fg(colorBorder).Render("·")
	}

	kind := c.Kind
	if kind == "" {
		kind = "service"
	}
	kindStyle := fg(serviceHue(kind))
	if c.Kind == "" {
		kindStyle = fg(colorMuted)
	}

	nameStyle := fg(colorText)
	if cursor {
		nameStyle = nameStyle.Foreground(colorAccent).Bold(true)
	}

	info := ""
	switch {
	case savedAs != "":
		info = fg(statusHealthyColor).Render("✓ saved as " + savedAs)
	case c.Note != "":
		info = fg(colorMuted).Render(c.Note)
	}

	return u.cursorBar(cursor) + mark + " " + kindStyle.Render(fitWidth(kind, 13)) +
		nameStyle.Render(fitWidth(truncateRunes(c.Service, nameWidth), nameWidth)) + "   " +
		fg(colorText).Render(fitWidth(fmt.Sprintf(":%d", c.Port), 7)) + "  " + info
}

func (u *UI) renderDiscoverReview(width, height int) []string {
	d := &u.discover
	saved, _ := u.store.LoadServices()

	blocks := make([][]string, len(d.items))
	for i := range d.items {
		item := &d.items[i]
		focusName, focusPort, focusCmd := d.focus == i*3, d.focus == i*3+1, d.focus == i*3+2

		head := fg(serviceHue(item.cand.Kind)).Render("◆ "+fitWidth(nonEmpty(item.cand.Kind, "service"), 12)) +
			fg(colorMuted).Render(item.cand.Service)

		field := func(label string, focused bool) string {
			if focused {
				return fg(colorAccent).Bold(true).Render(label)
			}
			return fg(colorMuted).Render(label)
		}
		rail := func(focused bool) string {
			if focused {
				return fg(colorAccent).Render("┃ ")
			}
			return fg(colorBorder).Render("┃ ")
		}

		check := fg(statusHealthyColor).Render("  ✓ ready")
		if p := u.reviewProblem(i, saved); p != "" {
			check = fg(colorWarn).Render("  ! " + p)
		} else if w := u.reviewWarning(i, saved); w != "" {
			check = fg(statusHealthyColor).Render("  ✓ ready") + fg(colorMuted).Render("   (note: "+w+")")
		}

		blocks[i] = []string{
			head,
			"  " + field("NAME ", focusName) + rail(focusName) + fitWidth(item.name.View(), 28) +
				field("  LOCAL PORT ", focusPort) + rail(focusPort) + item.port.View(),
			"  " + field("CMD  ", focusCmd) + rail(focusCmd) + item.command.View(),
			check,
		}
	}

	// Window whole items around the focused one.
	perItem := 5
	visible := max(1, (height-7)/perItem)
	focused := d.focus / 3
	start := 0
	if focused >= visible {
		start = focused - visible + 1
	}
	end := min(len(blocks), start+visible)

	out := []string{""}
	for i := start; i < end; i++ {
		out = append(out, blocks[i]...)
		out = append(out, "")
	}
	if len(blocks) > visible {
		out = append(out, fg(colorMuted).Render(fmt.Sprintf("%d–%d of %d services", start+1, end, len(blocks))))
	}
	return out
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
