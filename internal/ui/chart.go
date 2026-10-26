package ui

import (
	"hash/fnv"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// serviceHues gives every service a stable identity color, used for its chart
// and its name in the logs. Cool, saturated hues only: green, yellow and red
// stay reserved for status so a service's color never reads as its health.
var serviceHues = []string{
	"#2DD4BF", // teal
	"#38BDF8", // sky
	"#818CF8", // indigo
	"#A78BFA", // violet
	"#F472B6", // pink
	"#E879F9", // fuchsia
	"#67E8F9", // cyan
	"#FDA4AF", // rose
}

// serviceHue picks a service's color from its name, so the same service keeps
// the same color across runs.
func serviceHue(name string) color.Color {
	h := fnv.New32a()
	h.Write([]byte(name))
	return lipgloss.Color(serviceHues[h.Sum32()%uint32(len(serviceHues))])
}

// Braille dot bits, bottom row first, for the left and right sub-columns of a
// cell. Each braille cell is a 2×4 dot grid, so one character draws two
// samples at four levels each — a smooth curve in a single text row.
var (
	brailleLeft  = [4]rune{0x40, 0x04, 0x02, 0x01}
	brailleRight = [4]rune{0x80, 0x20, 0x10, 0x08}
)

// brailleCell fills the bottom left/right dots of one cell (levels 0–4).
func brailleCell(left, right int) rune {
	r := rune(0x2800)
	for i := 0; i < left; i++ {
		r |= brailleLeft[i]
	}
	for i := 0; i < right; i++ {
		r |= brailleRight[i]
	}
	return r
}

// chartReference is the throughput (bytes/s) that always fills a chart cell.
// Scaling against at least this — rather than only the window's own peak —
// keeps steady small chatter low instead of drawing it as a solid wall.
const chartReference = 512 * 1024

// streamChart draws a live area chart of samples (oldest first) in width
// cells: the newest sample sits at the right edge in the service's full color
// and history fades into the background as it scrolls left. Levels use a log
// scale (in KB/s) against the window's peak or chartReference, whichever is
// larger, so a trickle of queries still shows as a low ripple while a bulk
// transfer reaches the top. Idle time is a faint baseline.
func streamChart(samples []float64, width int, hue color.Color) string {
	if width <= 0 {
		return ""
	}
	n := width * 2
	if len(samples) > n {
		samples = samples[len(samples)-n:]
	}
	padded := make([]float64, n)
	copy(padded[n-len(samples):], samples)

	ref := float64(chartReference)
	for _, v := range padded {
		ref = math.Max(ref, v)
	}
	level := func(v float64) int {
		if v <= 0 {
			return 1 // baseline
		}
		return 1 + int(math.Round(math.Log1p(v/1024)/math.Log1p(ref/1024)*3))
	}

	fade := lipgloss.Blend1D(width, lipgloss.Darken(hue, 0.7), hue)
	baseline := lipgloss.NewStyle().Foreground(lipgloss.Darken(hue, 0.88))

	var b strings.Builder
	for i := 0; i < width; i++ {
		l, r := padded[2*i], padded[2*i+1]
		cell := string(brailleCell(level(l), level(r)))
		if l <= 0 && r <= 0 {
			b.WriteString(baseline.Render(cell))
			continue
		}
		b.WriteString(lipgloss.NewStyle().Foreground(fade[i]).Render(cell))
	}
	return b.String()
}

// scanChart is the chart slot of a service that is still connecting: a soft
// glow sweeping along the baseline, so waiting reads as motion, not a freeze.
func scanChart(width, frame int, c color.Color) string {
	if width <= 0 {
		return ""
	}
	pos := frame % (width + 6)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Darken(c, 0.8))

	var b strings.Builder
	for i := 0; i < width; i++ {
		switch d := pos - i; {
		case d == 0:
			b.WriteString(lipgloss.NewStyle().Foreground(c).Render(string(brailleCell(3, 3))))
		case d == 1 || d == -1:
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Darken(c, 0.35)).Render(string(brailleCell(2, 2))))
		case d == 2 || d == -2:
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Darken(c, 0.6)).Render(string(brailleCell(1, 1))))
		default:
			b.WriteString(dim.Render(string(brailleCell(1, 1))))
		}
	}
	return b.String()
}
