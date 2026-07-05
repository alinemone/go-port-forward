package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/icons"
	"github.com/alinemone/go-port-forward/internal/model"
)

// wrapText wraps text into lines no wider than maxWidth display columns. It
// breaks on spaces and hard-splits any single word wider than a full line.
// Widths are measured in terminal cells (not bytes), so wide runes such as
// CJK characters wrap correctly.
func wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 || lipgloss.Width(text) <= maxWidth {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return splitByDisplayWidth(text, maxWidth)
	}

	var lines []string
	var line string
	lineWidth := 0
	for _, word := range words {
		wordWidth := lipgloss.Width(word)
		switch {
		case wordWidth > maxWidth:
			if line != "" {
				lines = append(lines, line)
				line, lineWidth = "", 0
			}
			lines = append(lines, splitByDisplayWidth(word, maxWidth)...)
		case line == "":
			line, lineWidth = word, wordWidth
		case lineWidth+1+wordWidth > maxWidth:
			lines = append(lines, line)
			line, lineWidth = word, wordWidth
		default:
			line += " " + word
			lineWidth += 1 + wordWidth
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// splitByDisplayWidth hard-splits s into chunks of at most maxWidth display
// columns, never cutting through the middle of a rune.
func splitByDisplayWidth(s string, maxWidth int) []string {
	var chunks []string
	var chunk strings.Builder
	width := 0
	for _, r := range s {
		runeWidth := lipgloss.Width(string(r))
		if width+runeWidth > maxWidth && chunk.Len() > 0 {
			chunks = append(chunks, chunk.String())
			chunk.Reset()
			width = 0
		}
		chunk.WriteRune(r)
		width += runeWidth
	}
	if chunk.Len() > 0 {
		chunks = append(chunks, chunk.String())
	}
	return chunks
}

func truncateRunes(text string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func padRightRunes(text string, width int) string {
	runes := []rune(text)
	if len(runes) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-len(runes))
}

func padRightDisplayWidth(text string, width int) string {
	textWidth := lipgloss.Width(text)
	if textWidth >= width {
		return text
	}
	return text + strings.Repeat(" ", width-textWidth)
}

// serviceIcon returns the icon for a running service. It prefers the glyph the
// manager already resolved (which honors the user's config overrides) and falls
// back to the built-in port mapping when none was carried — e.g. in tests that
// construct a Service from just a port.
func serviceIcon(svc *model.Service) icons.Icon {
	if svc.IconGlyph != "" {
		return icons.Icon{Glyph: svc.IconGlyph, Color: svc.IconColor}
	}
	return icons.ForPort(svc.MainPort)
}

// renderIconCell renders a fixed two-column icon cell from an already-resolved
// glyph/color. An empty glyph yields two blank columns so name columns stay
// aligned whether or not a row carries an icon.
func renderIconCell(glyph, colorHex string) string {
	if glyph == "" {
		return "  "
	}
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color(colorHex)).Render(glyph)
	return padRightDisplayWidth(styled+" ", 2)
}

func renderActionChips(pairs [][2]string) string {
	keyStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(colorMuted)
	sep := descStyle.Render("  •  ")
	chips := make([]string, 0, len(pairs))
	for _, p := range pairs {
		chips = append(chips, keyStyle.Render(p[0])+descStyle.Render(" "+p[1]))
	}
	return strings.Join(chips, sep)
}
