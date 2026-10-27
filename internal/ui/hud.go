package ui

// The HUD design language shared by every screen: a header bar with the page
// name and mission clock, frames with bright corner brackets on faint edges, a
// key bar, and a sparse twinkling starfield for empty space. Every helper
// returns lines of an exact width so screens compose on a fixed grid.

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const margin = "  "

func fg(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

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

// screen fits lines to an exact width×height: clipped or padded to width, then
// cut or filled (with blank lines) to height.
func screen(lines []string, width, height int) string {
	if height > 0 {
		if len(lines) > height {
			lines = lines[:height]
		}
		for len(lines) < height {
			lines = append(lines, "")
		}
	}
	for i := range lines {
		lines[i] = fitWidth(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

// ── header ──────────────────────────────────────────────────────────────────

// hudHeader is the top bar of every screen: "◢◤ PF // PAGE" on the left, an
// optional status on the right, and the mission clock (time since pf started).
func (u *UI) hudHeader(page, right string) string {
	left := fg(colorAccent).Render("◢◤") + " " +
		lipgloss.NewStyle().Foreground(colorText).Bold(true).Render("PF") +
		fg(colorBorder).Render(" // ") +
		lipgloss.NewStyle().Foreground(colorHeading).Bold(true).Render(page)

	clock := fg(colorMuted).Render("T+") + fg(colorText).Render(missionClock(time.Since(u.startedAt)))
	inner := max(1, u.width-2*len(margin))

	for _, r := range []string{joinNonEmpty(right, clock), right, clock, ""} {
		if gap := inner - lipgloss.Width(left) - lipgloss.Width(r); gap >= 2 || r == "" {
			return margin + left + strings.Repeat(" ", max(0, gap)) + r
		}
	}
	return margin + left
}

func joinNonEmpty(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "     " + b
}

// missionClock renders an elapsed time as HH:MM:SS (days roll into hours).
func missionClock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Seconds())
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, s/60%60, s%60)
}

// ── frames ──────────────────────────────────────────────────────────────────

// frameTone colors a frame's corner brackets: the accent normally, the warning
// color for destructive confirmations.
type frameTone int

const (
	toneNormal frameTone = iota
	toneAlert
)

// hudFrame draws body inside a HUD frame of exactly width cells and, when
// height > 0, exactly height lines (body is padded or cut):
//
//	┌─ ▸ TITLE ─────────────────── badge ─┐
//	│ body                                 │
//	└──────────────────────────────────────┘
//
// Corners and their first edge cells are bright; the rest of the edge is faint.
func hudFrame(title, badge string, body []string, width, height int, tone frameTone) []string {
	width = max(8, width)
	inner := width - 4
	corner := fg(colorAccent)
	if tone == toneAlert {
		corner = fg(colorWarn)
	}
	edge := fg(colorBorder)

	head := corner.Render("┌─") + " " + corner.Render("▸ ") + title + " "
	tail := ""
	if badge != "" {
		tail = " " + badge + " "
	}
	tail += corner.Render("─┐")
	if fill := width - lipgloss.Width(head) - lipgloss.Width(tail); fill >= 0 {
		head += edge.Render(strings.Repeat("─", fill)) + tail
	} else {
		head = fitWidth(head, width-2) + corner.Render("─┐")
	}

	if height > 0 {
		rows := max(0, height-2)
		if len(body) > rows {
			body = body[:rows]
		}
		for len(body) < rows {
			body = append(body, "")
		}
	}

	out := make([]string, 0, len(body)+2)
	out = append(out, head)
	side := edge.Render("│")
	for _, line := range body {
		out = append(out, side+" "+fitWidth(line, inner)+" "+side)
	}
	out = append(out, corner.Render("└─")+edge.Render(strings.Repeat("─", width-4))+corner.Render("─┘"))
	return out
}

// hudTitle is a frame title: an uppercase label with an optional dim detail.
func hudTitle(label, detail string) string {
	out := lipgloss.NewStyle().Foreground(colorHeading).Bold(true).Render(label)
	if detail != "" {
		out += fg(colorMuted).Render(" · " + detail)
	}
	return out
}

// hudSection is an in-frame divider: "── GROUPS ─────".
func hudSection(label string, width int) string {
	head := fg(colorBorder).Render("── ") + lipgloss.NewStyle().Foreground(colorHeading).Bold(true).Render(label) + " "
	if fill := width - lipgloss.Width(head); fill > 0 {
		head += fg(colorBorder).Render(strings.Repeat("─", fill))
	}
	return head
}

// ── key bar ─────────────────────────────────────────────────────────────────

const maxKeyBarLines = 2

// keyBar lays key hints out left to right. Full labels are used when they fit
// in two lines, else the compact ones — wrapping further only on a tiny
// terminal, so no key is ever silently dropped. A transient status message
// takes the right end of the last line, or the line itself when there's no room.
func keyBar(full, compact [][2]string, inner int, status string) []string {
	lines := packHints(full, inner, "   ")
	if len(lines) > maxKeyBarLines && compact != nil {
		lines = packHints(compact, inner, "  ")
	}

	if status != "" {
		statusColor := colorAccentAlt
		if strings.HasPrefix(status, "✗") {
			statusColor = colorError
		}
		styled := fg(statusColor).Render(status)
		last := &lines[len(lines)-1]
		if gap := inner - lipgloss.Width(*last) - lipgloss.Width(styled); gap >= 3 {
			*last += strings.Repeat(" ", gap) + styled
		} else {
			*last = styled
		}
	}

	for i := range lines {
		lines[i] = margin + lines[i]
	}
	return lines
}

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

// ── starfield ───────────────────────────────────────────────────────────────

// starAt decides, deterministically per cell, whether a star sits at (x, y) of
// a field identified by seed, and which glyph it is. About 2% of cells hold a
// star; a few are brighter "✦" stars.
func starAt(x, y, seed int) (rune, uint32, bool) {
	h := fnv.New32a()
	fmt.Fprintf(h, "%d:%d:%d", seed, x, y)
	v := h.Sum32()
	switch {
	case v%97 == 0:
		return '✦', v, true
	case v%53 < 1:
		return '∙', v, true
	case v%37 < 1:
		return '·', v, true
	}
	return 0, 0, false
}

// starRun renders cells [x0, x0+width) of starfield row y. Stars twinkle: each
// one brightens briefly on its own beat of the animation frame.
func starRun(x0, width, y, seed, frame int) string {
	if width <= 0 {
		return ""
	}
	dim := fg(colorBorder)
	mid := fg(colorMuted)
	bright := fg(colorText)

	var b strings.Builder
	for x := x0; x < x0+width; x++ {
		r, v, ok := starAt(x, y, seed)
		if !ok {
			b.WriteByte(' ')
			continue
		}
		style := dim
		if r == '✦' {
			style = mid
		}
		if (int(v>>8)+frame/2)%23 == 0 {
			style = bright
		}
		b.WriteString(style.Render(string(r)))
	}
	return b.String()
}

// starfield renders width×height of sky. A non-empty message is centered on
// the middle row with clear space around it.
func starfield(width, height, seed, frame int, message string) []string {
	lines := make([]string, height)
	msgRow := height / 2
	msgW := lipgloss.Width(message)
	for y := range lines {
		if message != "" && y == msgRow && msgW+4 <= width {
			left := (width - msgW) / 2
			lines[y] = starRun(0, left-2, y, seed, frame) + "  " + message + "  " +
				starRun(left+msgW+2, width-left-msgW-2, y, seed, frame)
			continue
		}
		lines[y] = starRun(0, width, y, seed, frame)
	}
	if message != "" && msgW+4 > width && height > 0 {
		lines[msgRow] = fitWidth(message, width)
	}
	return lines
}
