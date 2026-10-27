package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

type helpItem struct{ key, description string }

func helpItemLinesWithSpacing(width int, items []helpItem, addRowSpacing bool) []string {
	if width < 8 {
		width = 8
	}
	keyStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(colorMuted)

	n := len(items)
	styled := make([]string, n)
	widths := make([]int, n)
	// The box spans the terminal width; reserve cells only for its border and
	// horizontal padding when calculating the responsive grid.
	inner := width - 4
	if inner < 4 {
		inner = 4
	}
	for i, item := range items {
		keyWidth := lipgloss.Width(item.key)
		maxDescWidth := inner - keyWidth - 2 // ": " between key and description
		description := truncateRunes(item.description, maxDescWidth)
		styled[i] = keyStyle.Render(item.key) + descStyle.Render(":")
		widths[i] = keyWidth + 1
		if description != "" {
			styled[i] += descStyle.Render(" " + description)
			widths[i] += 1 + lipgloss.Width(description)
		}
	}

	grid := helpGridLines(styled, widths, inner)
	if len(grid) < 2 || !addRowSpacing {
		return grid
	}

	// A blank line between grid rows keeps neighboring actions visually
	// distinct without adding internal borders or hardcoded decoration.
	spaced := make([]string, 0, len(grid)*2-1)
	for i, line := range grid {
		if i > 0 {
			spaced = append(spaced, "")
		}
		spaced = append(spaced, line)
	}
	return spaced
}

func helpGridLines(styled []string, widths []int, inner int) []string {
	n := len(styled)
	if n == 0 {
		return nil
	}
	const gap = 2

	for columns := n; columns >= 1; columns-- {
		columnWidths := make([]int, columns)
		for i, itemWidth := range widths {
			column := i % columns
			if itemWidth > columnWidths[column] {
				columnWidths[column] = itemWidth
			}
		}
		totalWidth := gap * (columns - 1)
		for _, columnWidth := range columnWidths {
			totalWidth += columnWidth
		}
		if totalWidth > inner {
			continue
		}

		rows := (n + columns - 1) / columns
		lines := make([]string, 0, rows)
		for row := 0; row < rows; row++ {
			var line strings.Builder
			for column := 0; column < columns; column++ {
				index := row*columns + column
				if index >= n {
					break
				}
				line.WriteString(styled[index])
				if index+1 < n && column+1 < columns {
					line.WriteString(strings.Repeat(" ", columnWidths[column]-widths[index]+gap))
				}
			}
			lines = append(lines, line.String())
		}
		return lines
	}

	return styled
}
