// Package theme is the single source of truth for the app's color palette.
// Every UI surface — the interactive TUI, the CLI output, and the chrome icon
// colors — reads from the Active palette, so the look stays consistent and
// adding a new theme is a one-place change.
//
// Palettes are plain hex-string data (no lipgloss dependency); each consumer
// converts to its own color type at the edge. Don't hardcode chrome colors
// elsewhere — read them from Active (refreshed at startup from the saved choice).
package theme

// Semantic service-health colors are FIXED across every theme — a service's
// status must read the same everywhere regardless of the chosen palette:
// green = healthy, yellow = connecting, red = error.
const (
	StatusHealthy    = "#73FFB6"
	StatusConnecting = "#FFD166"
	StatusError      = "#FF6B6B"
)

// Palette is one named color scheme, expressed as hex strings. A turquoise
// green is the default brand accent.
type Palette struct {
	Name      string
	Text      string // primary foreground text
	Muted     string // secondary / dim text
	Border    string // box borders, rules
	Accent    string // primary accent (cursor, names, titles, group icon)
	AccentAlt string // success / healthy
	Warn      string // warnings, section emphasis
	Error     string // errors
	Heading   string // headings / table headers / default icon
	Selected  string // selected-row background
}

// Built-in palettes. "default" is green; "ocean" is the classic blue/teal;
// "sunset" is a warm amber/pink.
var (
	defaultPalette = Palette{
		Name: "default",
		Text: "#EAEEF5", Muted: "#7C879B", Border: "#33415A",
		Accent: "#2DD4BF", AccentAlt: "#73FFB6",
		Warn: "#FFD166", Error: "#FF6B6B",
		Heading: "#AEB9CC", Selected: "#134E4A",
	}
	oceanPalette = Palette{
		Name: "ocean",
		Text: "#EAEEF5", Muted: "#7C879B", Border: "#2A3A5A",
		Accent: "#5BD4FF", AccentAlt: "#56E0C7",
		Warn: "#FFD166", Error: "#FF6B6B",
		Heading: "#AEB9CC", Selected: "#1E3A5F",
	}
	sunsetPalette = Palette{
		Name: "sunset",
		Text: "#F5ECEF", Muted: "#9B8794", Border: "#4A3550",
		Accent: "#FFB454", AccentAlt: "#FF7E9D",
		Warn: "#FFD166", Error: "#FF6B6B",
		Heading: "#C9B8D8", Selected: "#3A1E2F",
	}

	palettes = map[string]Palette{
		defaultPalette.Name: defaultPalette,
		oceanPalette.Name:   oceanPalette,
		sunsetPalette.Name:  sunsetPalette,
	}

	// order is the display order for Names().
	order = []string{"default", "ocean", "sunset"}

	// Active is the currently selected palette. Defaults to "default".
	Active = defaultPalette
)

// Names returns the available theme names in display order.
func Names() []string { return append([]string(nil), order...) }

// Exists reports whether name is a known theme.
func Exists(name string) bool {
	_, ok := palettes[name]
	return ok
}

// Get returns a palette by name and whether it exists.
func Get(name string) (Palette, bool) {
	p, ok := palettes[name]
	return p, ok
}

// Set switches the Active palette by name. Unknown names leave Active unchanged
// and return false. An empty name selects the default theme.
func Set(name string) bool {
	if name == "" {
		Active = defaultPalette
		return true
	}
	p, ok := palettes[name]
	if !ok {
		return false
	}
	Active = p
	return true
}
