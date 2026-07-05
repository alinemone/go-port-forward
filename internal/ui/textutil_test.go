package ui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestWrapTextKeepsShortTextOnOneLine(t *testing.T) {
	lines := wrapText("short message", 40)
	if len(lines) != 1 || lines[0] != "short message" {
		t.Fatalf("expected single unchanged line, got %q", lines)
	}
}

func TestWrapTextBreaksOnWordBoundaries(t *testing.T) {
	lines := wrapText("alpha beta gamma delta", 11)
	want := []string{"alpha beta", "gamma delta"}
	if len(lines) != len(want) {
		t.Fatalf("expected %d lines, got %q", len(want), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d: expected %q, got %q", i, want[i], lines[i])
		}
	}
}

func TestWrapTextHardSplitsOverlongWords(t *testing.T) {
	lines := wrapText("abcdefghij", 4)
	want := []string{"abcd", "efgh", "ij"}
	if len(lines) != len(want) {
		t.Fatalf("expected %d chunks, got %q", len(want), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("chunk %d: expected %q, got %q", i, want[i], lines[i])
		}
	}
}

func TestWrapTextMeasuresDisplayWidthNotBytes(t *testing.T) {
	// Each CJK rune is 3 bytes but 2 display columns; byte-based wrapping
	// would cut mid-rune or overflow the line.
	lines := wrapText("日本語のログ行です", 6)
	for _, line := range lines {
		if w := lipgloss.Width(line); w > 6 {
			t.Fatalf("line %q is %d columns wide, exceeds 6: all lines %q", line, w, lines)
		}
	}
}
