package ui

import (
	"strings"
	"testing"

	"github.com/alinemone/go-port-forward/internal/icons"
	"github.com/alinemone/go-port-forward/internal/model"
	"github.com/alinemone/go-port-forward/internal/theme"
)

func TestRenderServiceTableHidesIconsWhenDisabled(t *testing.T) {
	out := renderServiceTable([]model.Service{{
		Name:        "db",
		LocalPort:   "15432",
		MainPort:    "5432",
		Status:      model.StatusHealthy,
		IconEnabled: false,
	}}, 0, 0, 10, 120)

	if strings.Contains(out, icons.ForPort("5432").Glyph) {
		t.Fatalf("expected no icon when IconEnabled=false, output: %q", out)
	}
	if !strings.Contains(out, "db") {
		t.Fatalf("expected service name in output: %q", out)
	}
}

func TestRenderServiceTableShowsColoredPortIcon(t *testing.T) {
	icon := icons.ForPort("5432")
	out := renderServiceTable([]model.Service{{
		Name:        "db",
		LocalPort:   "15432",
		MainPort:    "5432",
		Status:      model.StatusHealthy,
		IconEnabled: true,
	}}, 0, 0, 10, 120)

	if !strings.Contains(out, icon.Glyph) {
		t.Fatalf("expected mapped icon %q in output: %q", icon.Glyph, out)
	}
	if !strings.Contains(out, "db") {
		t.Fatalf("expected service name in output: %q", out)
	}
}

func TestManageServiceRowShowsIconWhenEnabled(t *testing.T) {
	u := &UI{manageIcons: overlayIcons{
		enabled: true,
		set:     icons.NewSet(nil, nil),
		ports:   map[string]string{"db": "5432"},
	}}
	out := u.renderManageServiceRow("db", false, 10, map[string]bool{})
	if !strings.Contains(out, icons.ForPort("5432").Glyph) {
		t.Fatalf("expected port icon in overlay row: %q", out)
	}
}

func TestManageServiceRowHidesIconWhenDisabled(t *testing.T) {
	u := &UI{manageIcons: overlayIcons{
		enabled: false,
		set:     icons.NewSet(nil, nil),
		ports:   map[string]string{"db": "5432"},
	}}
	out := u.renderManageServiceRow("db", false, 10, map[string]bool{})
	if strings.Contains(out, icons.ForPort("5432").Glyph) {
		t.Fatalf("icons disabled: no glyph expected, got: %q", out)
	}
}

func TestManageGroupRowShowsFolderIconWhenEnabled(t *testing.T) {
	u := &UI{
		manageIcons:     overlayIcons{enabled: true, set: icons.NewSet(nil, nil)},
		manageGroups:    map[string][]string{"backend": {"db"}},
		manageSelGroups: map[string]bool{},
	}
	out := u.renderManageGroupRow("backend", false, 10, map[string]bool{})
	if !strings.Contains(out, icons.ForGroup().Glyph) {
		t.Fatalf("expected group folder icon in overlay row: %q", out)
	}
}

func TestServiceStatusColorsAreThemeIndependent(t *testing.T) {
	theme.Set("sunset") // a theme whose accent/accentAlt are pink, not green
	ApplyTheme()
	defer func() { theme.Set(""); ApplyTheme() }()

	out := renderServiceTable([]model.Service{{
		Name:      "db",
		LocalPort: "5432",
		Status:    model.StatusHealthy,
	}}, 0, 0, 10, 120)

	// HEALTHY must stay the fixed green (#73FFB6 = 115;255;182) under any theme.
	if !strings.Contains(out, "115;255;182") {
		t.Fatalf("HEALTHY must be fixed green regardless of theme: %q", out)
	}
	// The sunset accentAlt pink (#FF7E9D = 255;126;157) must not leak into status.
	if strings.Contains(out, "255;126;157") {
		t.Fatalf("status color leaked the themed accent: %q", out)
	}
}

func TestRenderServiceTableShowsDefaultIconForUnknownPort(t *testing.T) {
	out := renderServiceTable([]model.Service{{
		Name:        "custom",
		LocalPort:   "18081",
		MainPort:    "18081",
		Status:      model.StatusHealthy,
		IconEnabled: true,
	}}, 0, 0, 10, 120)

	if !strings.Contains(out, icons.DefaultGlyph) {
		t.Fatalf("expected default icon %q in output: %q", icons.DefaultGlyph, out)
	}
}
