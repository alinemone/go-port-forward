package ui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/alinemone/go-port-forward/internal/theme"
)

func withTheme(t *testing.T, name string) {
	t.Helper()
	if !theme.Set(name) {
		t.Fatalf("unknown theme %q", name)
	}
	ApplyTheme()
	t.Cleanup(func() { theme.Set(""); ApplyTheme() })
}

func sgr(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("38;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

func hexSGR(hex string) string {
	var r, g, b int
	fmt.Sscanf(strings.TrimPrefix(hex, "#"), "%02x%02x%02x", &r, &g, &b)
	return fmt.Sprintf("38;2;%d;%d;%d", r, g, b)
}

// Every screen must be drawn in the active theme's accent.
func TestEveryScreenFollowsTheTheme(t *testing.T) {
	for _, name := range []string{"default", "ocean", "sunset"} {
		t.Run(name, func(t *testing.T) {
			withTheme(t, name)
			accent := hexSGR(theme.Active.Accent)

			screens := map[string]func() string{}
			u, _ := newTestUI(t, 120, 30, sampleServices()...)
			screens["dashboard"] = u.viewContent
			e, _ := newTestUI(t, 120, 30)
			screens["dashboard-empty"] = e.viewContent
			h := newHangarUI(t, 120, 30)
			screens["hangar"] = h.viewContent
			f := newHangarUI(t, 120, 30)
			f.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
			f.Update(key('s'))
			screens["service-form"] = f.viewContent
			m := &UI{width: 120, height: 30}
			screens["manual"] = func() string { return m.renderShortcutOverlay(false) }
			fakeCluster(t)
			d := newHangarUI(t, 120, 30)
			drain(d, d.openDiscover("prod-salam-gate", false))
			screens["discover"] = d.viewContent
			q, _ := newTestUI(t, 120, 30, sampleServices()...)
			q.quitting = true
			screens["shutdown"] = q.viewContent

			for screen, render := range screens {
				if !strings.Contains(render(), accent) {
					t.Errorf("%s screen does not use the %s accent %s", screen, name, theme.Active.Accent)
				}
			}
		})
	}
}

// The per-service chart colors come from the theme, so switching themes
// recolors the dashboard's charts and log names too.
func TestServiceColorsFollowTheTheme(t *testing.T) {
	seen := map[string]string{}
	for _, name := range []string{"default", "ocean", "sunset"} {
		withTheme(t, name)
		if got := sgr(serviceHue("order-pg")); seen[got] != "" {
			t.Errorf("order-pg has the same color under %s and %s", name, seen[got])
		} else {
			seen[got] = name
		}

		u, _ := newTestUI(t, 120, 30, sampleServices()...)
		now := time.Now()
		u.sampleTraffic(now)
		u.services[1].BytesIn += 64 << 10 // order-pg (core sorts first)
		u.sampleTraffic(now.Add(time.Second))
		if out := u.viewContent(); !strings.Contains(out, sgr(serviceHue("order-pg"))) {
			t.Errorf("%s: dashboard does not draw order-pg in its theme color", name)
		}
	}

	withTheme(t, "default")
	a, b := sgr(serviceHue("order-pg")), sgr(serviceHue("order-pg"))
	if a != b {
		t.Error("a service's color must be stable within a theme")
	}
}

func TestCustomThemeGetsItsOwnServiceColors(t *testing.T) {
	theme.Register(theme.Palette{Name: "zz-custom", Text: "#EEEEEE", Muted: "#888888", Border: "#333333",
		Accent: "#FF5C35", AccentAlt: "#FFD166", Warn: "#FFD166", Error: "#FF6B6B", Heading: "#CCCCCC", Selected: "#442211"})
	withTheme(t, "zz-custom")
	if got := sgr(serviceHues[0]); got != hexSGR("#FF5C35") {
		t.Errorf("first service color = %s, want the custom accent", got)
	}
}
