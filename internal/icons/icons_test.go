package icons

import (
	"testing"

	"github.com/alinemone/go-port-forward/internal/theme"
)

func TestGroupAndDefaultFollowActiveTheme(t *testing.T) {
	defer theme.Set("") // restore default for other tests

	theme.Set("default")
	if got := ForGroup().Color; got != "#2DD4BF" {
		t.Errorf("default group color = %q, want turquoise green", got)
	}
	if got := ForPort("99999").Color; got != "#AEB9CC" {
		t.Errorf("default fallback color = %q, want heading gray", got)
	}

	theme.Set("ocean")
	if got := ForGroup().Color; got != "#5BD4FF" {
		t.Errorf("ocean group color = %q, want blue", got)
	}

	// Per-technology brand colors must NOT change with the theme.
	if got := ForPort("5432").Color; got != "#24829E" {
		t.Errorf("postgres icon color changed with theme: %q", got)
	}
}

func TestForPortKnownPorts(t *testing.T) {
	for _, port := range []string{"80", "443", "5432", "3306", "6379", "27017", "9200", "5672", "9092"} {
		t.Run(port, func(t *testing.T) {
			icon := ForPort(port)
			if icon.Glyph == "" {
				t.Fatal("expected glyph")
			}
			if icon.Color == "" {
				t.Fatal("expected color")
			}
			if icon.Glyph == DefaultGlyph {
				t.Fatalf("known port %s returned default icon", port)
			}
		})
	}
}

func TestForPortDefault(t *testing.T) {
	for _, port := range []string{"", "12345"} {
		t.Run(port, func(t *testing.T) {
			icon := ForPort(port)
			if icon.Glyph != DefaultGlyph {
				t.Fatalf("Glyph = %q, want default %q", icon.Glyph, DefaultGlyph)
			}
			if icon.Color != DefaultColor {
				t.Fatalf("Color = %q, want default %q", icon.Color, DefaultColor)
			}
		})
	}
}

func TestForGroupHasGlyph(t *testing.T) {
	g := ForGroup()
	if g.Glyph == "" || g.Color == "" {
		t.Fatalf("group icon must have glyph and color, got %#v", g)
	}
}

func TestNilSetFallsBackToBuiltins(t *testing.T) {
	var s *Set
	if got := s.ForPort("5432"); got != ForPort("5432") {
		t.Errorf("nil set ForPort = %#v, want built-in", got)
	}
	if got := s.ForGroup(); got != ForGroup() {
		t.Errorf("nil set ForGroup = %#v, want built-in", got)
	}
}

func TestSetAddsUnknownPort(t *testing.T) {
	s := NewSet(map[string]Icon{"7000": {Glyph: "X", Color: "#FF0000"}}, nil)
	got := s.ForPort("7000")
	if got.Glyph != "X" || got.Color != "#FF0000" {
		t.Fatalf("ForPort(7000) = %#v, want custom override", got)
	}
}

func TestSetOverridesColorOnlyKeepsBuiltinGlyph(t *testing.T) {
	s := NewSet(map[string]Icon{"5432": {Color: "#000000"}}, nil)
	got := s.ForPort("5432")
	if got.Glyph != ForPort("5432").Glyph {
		t.Errorf("glyph should stay built-in, got %q", got.Glyph)
	}
	if got.Color != "#000000" {
		t.Errorf("color should be overridden, got %q", got.Color)
	}
}

func TestSetOverridesGroupIcon(t *testing.T) {
	s := NewSet(nil, &Icon{Glyph: "G", Color: "#123456"})
	got := s.ForGroup()
	if got.Glyph != "G" || got.Color != "#123456" {
		t.Fatalf("ForGroup = %#v, want custom override", got)
	}
}
