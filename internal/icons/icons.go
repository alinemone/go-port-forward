package icons

import "github.com/alinemone/go-port-forward/internal/theme"

// Per-port colors below are deliberately fixed brand colors (a Postgres icon is
// always Postgres-blue, etc.) and don't follow the switchable chrome theme. The
// fallback (default) and group icon colors DO follow the active theme.
const (
	DefaultGlyph = "" // nf-fa-cube
	DefaultColor = "#AEB9CC"

	// Built-in group icon (nf-fa-folder, U+F07B). Lives in the Private Use
	// Area, so it renders as a blank box without a Nerd Font installed.
	groupGlyph = ""
)

type Icon struct {
	Glyph string
	Color string
}

func ForPort(port string) Icon {
	switch port {
	case "80", "8000", "8080", "3000":
		return Icon{Glyph: "", Color: "#5BD4FF"} // nf-fa-globe
	case "443", "8443":
		return Icon{Glyph: "", Color: "#FFD166"} // nf-fa-lock
	case "5432":
		return Icon{Glyph: "", Color: "#24829E"} // nf-dev-postgresql
	case "3306":
		return Icon{Glyph: "", Color: "#4479A1"} // nf-dev-mysql
	case "6379":
		return Icon{Glyph: "", Color: "#D82C20"} // nf-dev-redis
	case "27017":
		return Icon{Glyph: "", Color: "#47A248"} // nf-dev-mongodb
	case "9200":
		return Icon{Glyph: "", Color: "#00BFB3"} // nf-fa-database
	case "5672":
		return Icon{Glyph: "", Color: "#FF6600"} // nf-fa-cubes
	case "9092":
		return Icon{Glyph: "󱀏", Color: "#8B5CF6"} // nf-md-apache_kafka
	default:
		// Unknown ports use the neutral "default" icon, colored from the theme.
		return Icon{Glyph: DefaultGlyph, Color: theme.Active.Heading}
	}
}

// ForGroup returns the built-in icon shown for a group of services. Its color
// follows the active chrome theme (the glyph is fixed).
func ForGroup() Icon {
	return Icon{Glyph: groupGlyph, Color: theme.Active.Accent}
}

// Set resolves icons from user-supplied overrides layered on top of the
// built-ins. It lets a user re-skin a known port, add a glyph for a port the
// built-ins don't cover, or restyle the group icon — all from config.
//
// A nil *Set is valid and returns only the built-ins, so callers never need a
// guard before resolving.
type Set struct {
	ports map[string]Icon
	group *Icon
}

// NewSet builds a resolver from per-port overrides and an optional group
// override. Either may be nil/empty.
func NewSet(ports map[string]Icon, group *Icon) *Set {
	return &Set{ports: ports, group: group}
}

// ForPort resolves the icon for a port. A user override (with any empty
// glyph/color filled in from the built-in) wins over the built-in mapping.
func (s *Set) ForPort(port string) Icon {
	base := ForPort(port)
	if s == nil {
		return base
	}
	if override, ok := s.ports[port]; ok {
		return merge(base, override)
	}
	return base
}

// ForGroup resolves the group icon, applying the user's override if present.
func (s *Set) ForGroup() Icon {
	base := ForGroup()
	if s == nil || s.group == nil {
		return base
	}
	return merge(base, *s.group)
}

// merge overlays the non-empty fields of override onto base, so a user can
// customize just the color, just the glyph, or both.
func merge(base, override Icon) Icon {
	if override.Glyph != "" {
		base.Glyph = override.Glyph
	}
	if override.Color != "" {
		base.Color = override.Color
	}
	return base
}
