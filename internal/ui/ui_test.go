package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/alinemone/go-port-forward/internal/icons"
	"github.com/charmbracelet/x/ansi"
)

func TestManageOverlayFillsTerminalAndAnchorsResponsiveHelp(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*UI)
	}{
		{name: "normal"},
		{name: "new prompt", configure: func(u *UI) { u.manage.showNewPrompt = true }},
		{name: "delete confirmation", configure: func(u *UI) {
			u.manage.confirmDeleteName = "api"
			u.manage.confirmDeleteKind = "service"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := &UI{
				width: 55, height: 24,
				manage: manageState{
					active: true,
					rows: []manageRow{
						{kind: rowHeaderGroups}, {kind: rowEmptyGroups},
						{kind: rowHeaderServices}, {kind: rowEmptyServices},
					},
					selectedGroups:   map[string]bool{},
					selectedServices: map[string]bool{},
				},
			}
			if tc.configure != nil {
				tc.configure(u)
			}

			out := u.renderManageOverlay()
			plain := ansi.Strip(out)
			if got := lipgloss.Height(out); got != u.height {
				t.Fatalf("overlay height=%d, terminal height=%d\n%s", got, u.height, plain)
			}
			lines := strings.Split(out, "\n")
			if !strings.HasPrefix(ansi.Strip(lines[len(lines)-1]), "╰") {
				t.Fatalf("help is not anchored to terminal bottom: %q", ansi.Strip(lines[len(lines)-1]))
			}
			if !strings.Contains(plain, "?: all shortcuts") || !strings.Contains(plain, "Space: select") {
				t.Fatalf("manage compact help is missing essential actions: %q", plain)
			}
			for _, line := range lines {
				if got := lipgloss.Width(line); got > u.width {
					t.Fatalf("line exceeds terminal width: got %d, want <= %d: %q", got, u.width, ansi.Strip(line))
				}
			}
		})
	}
}

func TestManageCompactHelpAddsSpacingWhenHeightAllows(t *testing.T) {
	u := &UI{width: 55, height: 24}
	plain := ansi.Strip(u.renderManageHelp(55))
	blankRow := "│" + strings.Repeat(" ", 53) + "│"
	if !strings.Contains(plain, "\n"+blankRow+"\n") {
		t.Fatalf("manage help rows are not visually separated: %q", plain)
	}

	u.height = 12
	tiny := ansi.Strip(u.renderManageHelp(55))
	if strings.Contains(tiny, "\n"+blankRow+"\n") {
		t.Fatalf("tiny manage help kept spacing beyond its height budget: %q", tiny)
	}
}

func TestShortcutOverlayIsFullScreenAndPageSpecific(t *testing.T) {
	u := &UI{width: 70, height: 26, aliasEnabled: true}
	for _, manage := range []bool{false, true} {
		out := u.renderShortcutOverlay(manage)
		plain := ansi.Strip(out)
		if got := lipgloss.Width(strings.Split(out, "\n")[0]); got != u.width {
			t.Fatalf("overlay width=%d, want %d", got, u.width)
		}
		if got := lipgloss.Height(out); got != u.height {
			t.Fatalf("overlay height=%d, want %d", got, u.height)
		}
		if manage {
			if !strings.Contains(plain, "MANAGEMENT") || !strings.Contains(plain, "Ctrl+N: create item") {
				t.Fatalf("manage shortcuts missing: %q", plain)
			}
		} else if !strings.Contains(plain, "LOGS") || !strings.Contains(plain, "Ctrl+L: clear visible logs") {
			t.Fatalf("dashboard shortcuts missing: %q", plain)
		}
	}
}

func TestShortcutOverlayDoesNotOverflowTinySplit(t *testing.T) {
	for _, size := range []struct{ width, height int }{{32, 16}, {28, 10}} {
		u := &UI{width: size.width, height: size.height}
		out := u.renderShortcutOverlay(false)
		if got := lipgloss.Height(out); got != size.height {
			t.Fatalf("%dx%d split: overlay height=%d", size.width, size.height, got)
		}
		for _, line := range strings.Split(out, "\n") {
			if got := lipgloss.Width(line); got > size.width {
				t.Fatalf("%dx%d split: line width=%d: %q", size.width, size.height, got, ansi.Strip(line))
			}
		}
	}
}

func TestManageServiceRowShowsIconWhenEnabled(t *testing.T) {
	u := &UI{manage: manageState{icons: overlayIcons{
		enabled: true,
		set:     icons.NewSet(nil, nil),
		ports:   map[string]string{"db": "5432"},
	}}}
	out := u.renderManageServiceRow("db", false, 10, map[string]bool{})
	if !strings.Contains(out, icons.ForPort("5432").Glyph) {
		t.Fatalf("expected port icon in overlay row: %q", out)
	}
}

func TestManageServiceRowHidesIconWhenDisabled(t *testing.T) {
	u := &UI{manage: manageState{icons: overlayIcons{
		enabled: false,
		set:     icons.NewSet(nil, nil),
		ports:   map[string]string{"db": "5432"},
	}}}
	out := u.renderManageServiceRow("db", false, 10, map[string]bool{})
	if strings.Contains(out, icons.ForPort("5432").Glyph) {
		t.Fatalf("icons disabled: no glyph expected, got: %q", out)
	}
}

func TestManageGroupRowShowsFolderIconWhenEnabled(t *testing.T) {
	u := &UI{manage: manageState{
		icons:          overlayIcons{enabled: true, set: icons.NewSet(nil, nil)},
		groups:         map[string][]string{"backend": {"db"}},
		selectedGroups: map[string]bool{},
	}}
	out := u.renderManageGroupRow("backend", false, 10, map[string]bool{})
	if !strings.Contains(out, icons.ForGroup().Glyph) {
		t.Fatalf("expected group folder icon in overlay row: %q", out)
	}
}
