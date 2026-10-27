package ui

import (
	"hash/fnv"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/alinemone/go-port-forward/internal/theme"
)

// serviceHues gives every service an identity color, used for its chart and its
// name in the logs. They are derived from the active theme (see
// deriveServiceHues), so switching themes recolors the whole dashboard.
var serviceHues []color.Color

// hueOffsets are the steps (degrees around the color wheel) from the theme's
// accent to each service color. 20° apart keeps all eight within the theme's
// family while staying tellable apart.
var hueOffsets = []float64{0, 20, -20, 40, -40, 60, -60, 80}

// deriveServiceHues builds the service palette from the theme accent by
// rotating its hue in CIE-LCh, which keeps every color as light and as vivid
// as the accent itself — so each theme gets its own family (teal/cyan for
// default, blues for ocean, ambers and pinks for sunset), custom themes too.
func deriveServiceHues(accent string) []color.Color {
	base, err := colorful.Hex(accent)
	if err != nil {
		base, _ = colorful.Hex("#2DD4BF")
	}
	h, c, l := base.Hcl()
	out := make([]color.Color, len(hueOffsets))
	for i, off := range hueOffsets {
		out[i] = colorful.Hcl(math.Mod(h+off+360, 360), c, l).Clamped()
	}
	return out
}

// serviceHue picks a service's color from its name, so the same service keeps
// the same color across runs (within a theme).
func serviceHue(name string) color.Color {
	if len(serviceHues) == 0 {
		serviceHues = deriveServiceHues(theme.Active.Accent)
	}
	h := fnv.New32a()
	h.Write([]byte(name))
	return serviceHues[h.Sum32()%uint32(len(serviceHues))]
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
