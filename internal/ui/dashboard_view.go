package ui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/alinemone/go-port-forward/internal/model"
)

const margin = "  "

// renderDashboard draws the monitoring screen: header, one line per forward
// with its live chart, the logs, and the key hints. Every line is exactly the
// terminal width.
func (u *UI) renderDashboard() string {
	l := u.layout()
	now := time.Now()
	inner := max(1, u.width-2*len(margin))

	forwardsTitle := fg(colorHeading).Bold(true).Render("forwards") +
		fg(colorMuted).Render(fmt.Sprintf(" · %d", len(u.services)))

	lines := []string{u.renderHeader(inner)}
	lines = append(lines, box(forwardsTitle, u.renderRows(l, inner, now), u.width)...)
	lines = append(lines, box(u.logsTitle(), strings.Split(u.viewport.View(), "\n"), u.width)...)
	lines = append(lines, u.footerLines(inner)...)

	for i := range lines {
		lines[i] = fitWidth(lines[i], u.width)
	}
	return strings.Join(lines, "\n")
}

// box frames body lines in a rounded border of the given width with the title
// set into the top edge ("╭─ forwards ───╮"). Body lines are clipped or padded
// to the inner width (width-4).
func box(title string, body []string, width int) []string {
	border := fg(colorBorder)
	inner := max(1, width-4)

	top := border.Render("╭─") + " " + ansi.Truncate(title, max(1, width-6), "…") + " "
	if fill := width - 1 - lipgloss.Width(top); fill > 0 {
		top += border.Render(strings.Repeat("─", fill))
	}
	top += border.Render("╮")

	out := make([]string, 0, len(body)+2)
	out = append(out, top)
	side := border.Render("│")
	for _, line := range body {
		out = append(out, side+" "+fitWidth(line, inner)+" "+side)
	}
	return append(out, border.Render("╰"+strings.Repeat("─", max(0, width-2))+"╯"))
}

// fitWidth pads or clips a styled line to exactly width cells.
func fitWidth(line string, width int) string {
	if width <= 0 {
		return ""
	}
	line = ansi.Truncate(line, width, "…")
	if pad := width - lipgloss.Width(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return line
}

func fg(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

// ── header ──────────────────────────────────────────────────────────────────

func (u *UI) renderHeader(inner int) string {
	logo := fg(colorAccent).Render("◆ ") +
		lipgloss.NewStyle().Foreground(colorText).Bold(true).Render("pf") +
		fg(colorMuted).Render("  port forward")

	var live, connecting, failed int
	for i := range u.services {
		switch u.services[i].Status {
		case model.StatusHealthy:
			live++
		case model.StatusError:
			failed++
		default:
			connecting++
		}
	}

	var parts []string
	if live > 0 {
		parts = append(parts, fg(statusHealthyColor).Render(fmt.Sprintf("● %d live", live)))
	}
	if connecting > 0 {
		parts = append(parts, fg(statusConnectingColor).Render(fmt.Sprintf("%s %d connecting", u.spinner(), connecting)))
	}
	if failed > 0 {
		parts = append(parts, fg(statusErrorColor).Render(fmt.Sprintf("✕ %d error", failed)))
	}
	summary := strings.Join(parts, "   ")
	if summary == "" {
		summary = fg(colorMuted).Render("idle")
	}

	gap := inner - lipgloss.Width(logo) - lipgloss.Width(summary)
	if gap < 2 {
		return margin + logo
	}
	return margin + logo + strings.Repeat(" ", gap) + summary
}

func (u *UI) spinner() string {
	return spinnerFrames[u.spinnerFrame%len(spinnerFrames)]
}

// ── service rows ────────────────────────────────────────────────────────────

// rowColumns are the widths of one row's fixed columns; the chart takes the
// rest. Narrow terminals drop the chart, then the status word.
type rowColumns struct {
	name, status, chart int
	icons               bool // a port icon cell before the name (`pf icon on`)
}

const (
	statusWidth = 10 // "connecting"
	portWidth   = 6  // ":15432"
	uptimeWidth = 7  // "23h 59m"
	rateWidth   = 5  // "12.4M"
	minChart    = 8
)

func computeRowColumns(services []model.Service, inner int) rowColumns {
	name := 8
	for i := range services {
		name = max(name, lipgloss.Width(services[i].Name))
	}
	name = min(name, 24)

	c := rowColumns{name: name, status: statusWidth}
	for i := range services {
		if services[i].IconEnabled {
			c.icons = true
			break
		}
	}
	// bar+space, status glyph+space, [port icon], name, 3, status, 2, port, 2, uptime
	fixed := func() int {
		w := 4 + c.name + 3 + c.status + 2 + portWidth + 2 + uptimeWidth
		if c.icons {
			w += 2
		}
		return w
	}

	if chart := inner - fixed() - 3 - 1 - rateWidth; chart >= minChart {
		c.chart = min(chart, historyLen/2)
		return c
	}
	if inner-fixed() < 0 {
		c.status = 0
		c.name = max(4, min(c.name, inner-fixed()+c.name))
	}
	return c
}

func (u *UI) renderRows(l dashLayout, inner int, now time.Time) []string {
	if len(u.services) == 0 {
		return []string{
			fg(colorMuted).Render("No forwards running — press ") +
				lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("a") +
				fg(colorMuted).Render(" to start one."),
		}
	}

	u.ensureCursorVisible(l.rows)
	end := min(len(u.services), u.tableOffset+l.rows)
	cols := computeRowColumns(u.services, inner)

	lines := make([]string, 0, l.rows+1)
	for i := u.tableOffset; i < end; i++ {
		lines = append(lines, u.renderRow(&u.services[i], cols, i == u.cursorIndex, now))
	}
	if l.scrolled {
		var hint []string
		if u.tableOffset > 0 {
			hint = append(hint, fmt.Sprintf("↑ %d above", u.tableOffset))
		}
		if below := len(u.services) - end; below > 0 {
			hint = append(hint, fmt.Sprintf("↓ %d below", below))
		}
		lines = append(lines, fg(colorMuted).Render(strings.Join(hint, "   ")))
	}
	return lines
}

func (u *UI) renderRow(svc *model.Service, c rowColumns, selected bool, now time.Time) string {
	hue := serviceHue(svc.Name)
	var b strings.Builder

	// Selection: a bar and the name in the service's own color.
	if selected {
		b.WriteString(fg(hue).Render("▎") + " ")
	} else {
		b.WriteString("  ")
	}

	icon, word, statusColor := u.statusMark(svc)
	b.WriteString(fg(u.iconColor(svc, statusColor)).Render(icon) + " ")

	if c.icons {
		cell := "  "
		if svc.IconEnabled {
			portIcon := serviceIcon(svc)
			cell = renderIconCell(portIcon.Glyph, portIcon.Color)
		}
		b.WriteString(cell)
	}

	name := lipgloss.NewStyle().Foreground(colorText)
	if selected {
		name = name.Foreground(hue).Bold(true)
	}
	b.WriteString(name.Render(fitWidth(truncateRunes(svc.Name, c.name), c.name)))
	b.WriteString("   ")

	if c.status > 0 {
		b.WriteString(fg(statusColor).Render(fitWidth(word, c.status)) + "  ")
	}
	b.WriteString(fg(colorText).Render(fitWidth(":"+svc.LocalPort, portWidth)) + "  ")

	uptime := "—"
	if svc.Status == model.StatusHealthy && !svc.ConnectedSince.IsZero() {
		uptime = formatDuration(now.Sub(svc.ConnectedSince))
	}
	b.WriteString(fg(colorMuted).Render(fitWidth(uptime, uptimeWidth)))

	if c.chart > 0 {
		b.WriteString("   ")
		switch svc.Status {
		case model.StatusHealthy:
			t := u.traffic[svc.Name]
			b.WriteString(streamChart(t.history(), c.chart, hue))
			rate := formatRate(t.current())
			b.WriteString(" " + fg(hue).Render(fmt.Sprintf("%*s", rateWidth, rate)))
		case model.StatusError:
			msg := svc.LastError
			if msg == "" {
				msg = "forward failed"
			}
			b.WriteString(fg(lipgloss.Darken(statusErrorColor, 0.15)).Render(truncateRunes(msg, c.chart+1+rateWidth)))
		default:
			b.WriteString(scanChart(c.chart, u.spinnerFrame, statusConnectingColor))
		}
	}
	return b.String()
}

// statusMark is a forward's status as a glyph, a word and a color — never color
// alone.
func (u *UI) statusMark(svc *model.Service) (icon, word string, c color.Color) {
	switch svc.Status {
	case model.StatusHealthy:
		return "●", "live", statusHealthyColor
	case model.StatusError:
		return "✕", "error", statusErrorColor
	default:
		return u.spinner(), "connecting", statusConnectingColor
	}
}

// iconColor is the status glyph's color: a live forward's dot pulses while data
// is flowing through it, so traffic is visible even before reading the chart.
func (u *UI) iconColor(svc *model.Service, c color.Color) color.Color {
	if svc.Status != model.StatusHealthy || u.traffic[svc.Name].current() <= 0 {
		return c
	}
	// triangle wave over the animation frame: bright → dim → bright
	phase := u.spinnerFrame % 6
	if phase > 3 {
		phase = 6 - phase
	}
	return lipgloss.Darken(c, float64(phase)*0.18)
}

// ── logs ────────────────────────────────────────────────────────────────────

func (u *UI) logsTitle() string {
	title := fg(colorHeading).Bold(true).Render("logs") + fg(colorMuted).Render(" · ")
	if scope := u.logScope(); scope != "" {
		return title + fg(serviceHue(scope)).Render(scope)
	}
	return title + fg(colorMuted).Render("all")
}

// renderLogsContent merges the services' logs in time order. With showNames
// each line carries its service name in that service's color.
func renderLogsContent(services []model.Service, maxWidth int, showNames bool) string {
	type entry struct {
		service string
		log     model.LogEntry
	}
	var all []entry
	for i := range services {
		for _, log := range services[i].Logs {
			all = append(all, entry{services[i].Name, log})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].log.Time.Before(all[j].log.Time) })

	if len(all) == 0 {
		return fg(colorMuted).Italic(true).Render("quiet so far…")
	}

	nameWidth := 0
	if showNames {
		for _, e := range all {
			nameWidth = max(nameWidth, lipgloss.Width(e.service))
		}
		nameWidth = min(nameWidth, max(4, maxWidth/5))
	}
	prefixWidth := 10 // "15:04:05  "
	if showNames {
		prefixWidth += nameWidth + 2
	}
	msgWidth := max(4, maxWidth-prefixWidth)
	timeStyle := fg(colorBorder)

	var out strings.Builder
	for _, e := range all {
		msgColor := colorText
		switch {
		case e.log.IsError:
			msgColor = colorError
		case strings.Contains(e.log.Message, "━━━━"):
			msgColor = colorWarn
		}
		msgStyle := fg(msgColor)

		prefix := timeStyle.Render(e.log.Time.Format("15:04:05")) + "  "
		if showNames {
			prefix += fg(serviceHue(e.service)).Render(fitWidth(truncateRunes(e.service, nameWidth), nameWidth)) + "  "
		}
		for i, line := range wrapText(e.log.Message, msgWidth) {
			if i > 0 {
				prefix = strings.Repeat(" ", prefixWidth)
			}
			out.WriteString(prefix + msgStyle.Render(line) + "\n")
		}
	}
	return strings.TrimSuffix(out.String(), "\n")
}

// ── footer ──────────────────────────────────────────────────────────────────

// footerItems are the dashboard key hints exactly as the previous version
// showed them; the compact labels are used only when the full ones don't fit
// in two lines.
func (u *UI) footerItems(compact bool) [][2]string {
	label := func(full, short string) string {
		if compact {
			return short
		}
		return full
	}
	items := [][2]string{
		{label("L/Ctrl+L", "L/^L"), label("view / clear logs", "logs")},
	}
	if u.aliasEnabled {
		items = append(items, [2]string{"y", label("copy alias", "copy")})
	}
	return append(items,
		[2]string{"a", label("add/edit services", "add/edit")},
		[2]string{"c", label("edit config", "config")},
		[2]string{label("r/Ctrl+R", "r/^R"), label("restart selected / all", "restart")},
		[2]string{"s", label("stop selected", "stop")},
		[2]string{"q", label("quit & stop all", "quit")},
	)
}

const maxFooterLines = 2

// packHints lays the hints out left to right, wrapping onto as many lines as
// needed.
func packHints(items [][2]string, inner int, gap string) []string {
	key := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	desc := fg(colorMuted)

	lines := []string{""}
	for _, item := range items {
		hint := key.Render(item[0]) + " " + desc.Render(item[1])
		last := &lines[len(lines)-1]
		switch {
		case *last == "":
			*last = hint
		case lipgloss.Width(*last)+len(gap)+lipgloss.Width(hint) <= inner:
			*last += gap + hint
		default:
			lines = append(lines, hint)
		}
	}
	return lines
}

// footerLines shows every key hint — never silently dropping one on a narrow
// terminal: full labels when they fit in two lines, else compact labels (on
// more lines only if the terminal is tiny). A transient status message takes
// the right end of the last line, or the line itself when there is no room.
func (u *UI) footerLines(inner int) []string {
	lines := packHints(u.footerItems(false), inner, "   ")
	if len(lines) > maxFooterLines {
		lines = packHints(u.footerItems(true), inner, "  ")
	}

	if u.editStatus != "" {
		statusColor := colorAccentAlt
		if strings.HasPrefix(u.editStatus, "✗") {
			statusColor = colorError
		}
		status := fg(statusColor).Render(u.editStatus)
		last := &lines[len(lines)-1]
		if gap := inner - lipgloss.Width(*last) - lipgloss.Width(status); gap >= 3 {
			*last += strings.Repeat(" ", gap) + status
		} else {
			*last = status
		}
	}

	for i := range lines {
		lines[i] = margin + lines[i]
	}
	return lines
}

// ── shutdown ────────────────────────────────────────────────────────────────

func (u *UI) renderShutdownScreen() string {
	msg := fg(colorAccentAlt).Bold(true).Render(u.spinner() + "  stopping forwards…")
	if u.width <= 0 || u.height <= 0 {
		return msg
	}
	return lipgloss.Place(u.width, u.height, lipgloss.Center, lipgloss.Center, msg)
}
