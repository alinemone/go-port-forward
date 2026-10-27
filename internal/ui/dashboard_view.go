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

// renderDashboard draws Mission Control: header, the forwards frame (one line
// per forward with its live chart), the logs frame, and the key bar. The result
// is exactly the terminal size.
func (u *UI) renderDashboard() string {
	l := u.layout()
	now := time.Now()
	inner := max(1, u.width-2*len(margin))

	badge := fg(colorMuted).Render(fmt.Sprintf("%02d LINKS", len(u.services)))
	lines := []string{u.hudHeader("MISSION CONTROL", u.linkSummary()), ""}
	lines = append(lines, hudFrame(hudTitle("FORWARDS", ""), badge, u.renderRows(l, inner, now), u.width, 0, toneNormal)...)
	lines = append(lines, hudFrame(u.logsTitle(), "", u.logsBody(l, inner), u.width, 0, toneNormal)...)
	lines = append(lines, u.footerLines(inner)...)
	return screen(lines, u.width, u.height)
}

// linkSummary counts forwards by status for the header.
func (u *UI) linkSummary() string {
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
	if len(parts) == 0 {
		return fg(colorMuted).Render("standing by")
	}
	return strings.Join(parts, "   ")
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
		msg := fg(colorMuted).Render("no active links — press ") +
			lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("a") +
			fg(colorMuted).Render(" to launch one, or ") +
			lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("d") +
			fg(colorMuted).Render(" to discover a namespace")
		return starfield(inner, l.rows, 3, u.spinnerFrame, msg)
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
	title := lipgloss.NewStyle().Foreground(colorHeading).Bold(true).Render("LOGS") + fg(colorMuted).Render(" · ")
	if scope := u.logScope(); scope != "" {
		return title + fg(serviceHue(scope)).Render(scope)
	}
	return title + fg(colorMuted).Render("all")
}

// logsBody is the log viewport, or — while there is nothing to read yet — a
// quiet starfield, so an empty pane reads as "waiting", not "broken".
func (u *UI) logsBody(l dashLayout, inner int) []string {
	empty := true
	for i := range u.services {
		if scope := u.logScope(); (scope == "" || u.services[i].Name == scope) && len(u.services[i].Logs) > 0 {
			empty = false
			break
		}
	}
	if empty {
		return starfield(inner, l.logs, 7, u.spinnerFrame, fg(colorMuted).Italic(true).Render("awaiting telemetry…"))
	}
	return strings.Split(u.viewport.View(), "\n")
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
		[2]string{"d", label("discover namespace", "discover")},
		[2]string{"c", label("edit config", "config")},
		[2]string{label("r/Ctrl+R", "r/^R"), label("restart selected / all", "restart")},
		[2]string{"s", label("stop selected", "stop")},
		[2]string{"q", label("quit & stop all", "quit")},
	)
}

// footerLines is the dashboard key bar.
func (u *UI) footerLines(inner int) []string {
	return keyBar(u.footerItems(false), u.footerItems(true), inner, u.editStatus)
}

// ── shutdown ────────────────────────────────────────────────────────────────

func (u *UI) renderShutdownScreen() string {
	msg := fg(colorAccent).Render(u.spinner()+"  ") +
		lipgloss.NewStyle().Foreground(colorHeading).Bold(true).Render("DISENGAGING") +
		fg(colorMuted).Render(fmt.Sprintf("  ·  closing %d link%s and releasing ports", len(u.services), plural(len(u.services))))
	if u.width <= 0 || u.height <= 0 {
		return msg
	}
	return screen(starfield(u.width, u.height, 5, u.spinnerFrame, msg), u.width, u.height)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
