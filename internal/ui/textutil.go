package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/icons"
	"github.com/alinemone/go-port-forward/internal/model"
)

func wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}
	if len(text) <= maxWidth {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		for i := 0; i < len(text); i += maxWidth {
			end := i + maxWidth
			if end > len(text) {
				end = len(text)
			}
			lines = append(lines, text[i:end])
		}
		return lines
	}

	var currentLine strings.Builder
	for _, word := range words {
		if len(word) > maxWidth {
			if currentLine.Len() > 0 {
				lines = append(lines, currentLine.String())
				currentLine.Reset()
			}
			for i := 0; i < len(word); i += maxWidth {
				end := i + maxWidth
				if end > len(word) {
					end = len(word)
				}
				lines = append(lines, word[i:end])
			}
			continue
		}

		testLine := currentLine.String()
		if len(testLine) > 0 {
			testLine += " " + word
		} else {
			testLine = word
		}

		if len(testLine) > maxWidth {
			if currentLine.Len() > 0 {
				lines = append(lines, currentLine.String())
				currentLine.Reset()
			}
			currentLine.WriteString(word)
		} else {
			if currentLine.Len() > 0 {
				currentLine.WriteString(" ")
			}
			currentLine.WriteString(word)
		}
	}

	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return lines
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
