package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alinemone/go-port-forward/internal/icons"
	"github.com/alinemone/go-port-forward/internal/model"
	"github.com/alinemone/go-port-forward/internal/theme"
	"github.com/charmbracelet/x/ansi"
)

func TestRefreshViewportFollowsNewLogs(t *testing.T) {
	logs := make([]model.LogEntry, 30)
	for i := range logs {
		logs[i] = model.LogEntry{Time: time.Unix(int64(i+1), 0), Message: fmt.Sprintf("line %d", i)}
	}
	u := &UI{
		services: []model.Service{{Name: "api", Logs: logs}},
		width:    80,
		height:   20,
		ready:    true,
		viewport: viewport.New(viewport.WithWidth(80), viewport.WithHeight(5)),
	}

	u.refreshViewportContent()
	u.viewport.GotoTop()
	if u.viewport.AtBottom() {
		t.Fatal("test setup needs scrollable log content")
	}

	// A refresh without new output preserves intentional manual scrolling.
	u.refreshViewportContent()
	if u.viewport.AtBottom() {
		t.Fatal("unchanged logs unexpectedly cancelled manual scrolling")
	}

	u.services[0].Logs = append(u.services[0].Logs, model.LogEntry{
		Time: time.Unix(31, 0), Message: "newest line",
	})
	u.refreshViewportContent()
	if !u.viewport.AtBottom() {
		t.Fatal("new log did not move viewport to the latest output")
	}
}

func TestHelpLinesFitResponsiveWidthsAndShowClearLogs(t *testing.T) {
	for _, width := range []int{32, 55, 90, 140} {
		lines := helpLines(width, "a-very-long-service-name", true)
		if len(lines) < 2 && width < 90 {
			t.Fatalf("width %d: expected wrapped help, got %d line(s)", width, len(lines))
		}
		joined := strings.Join(lines, "\n")
		if !strings.Contains(joined, "Ctrl+L") || !strings.Contains(joined, "clear logs") {
			t.Fatalf("width %d: combined log controls missing: %q", width, joined)
		}
		inner := width - 4
		for _, line := range lines {
			if got := lipgloss.Width(line); got > inner {
				t.Fatalf("width %d: help line is %d columns (inner width %d): %q", width, got, inner, line)
			}
		}
	}
}

func TestHelpHasOnlyOuterBorderAndKeyDescriptionGrid(t *testing.T) {
	narrow := renderHelp(32, "ALL", false)
	plain := ansi.Strip(narrow)
	if !strings.ContainsAny(plain, "╭╮╰╯") {
		t.Fatalf("help outer border is missing: %q", plain)
	}
	if strings.ContainsAny(plain, "┼├┤┬┴") {
		t.Fatalf("help must not have internal grid borders: %q", plain)
	}
	if !strings.Contains(plain, "L/Ctrl+L: view / clear logs") {
		t.Fatalf("expected key: description format: %q", plain)
	}
	if !strings.Contains(plain, "Ctrl+R: restart all") || strings.Contains(plain, "^r:") {
		t.Fatalf("expected readable Ctrl shortcut label: %q", plain)
	}
	if len(strings.Split(plain, "\n")) < 2 {
		t.Fatalf("narrow help should use multiple grid rows: %q", plain)
	}

	wide := helpLines(220, "ALL", false)
	if got := len(wide); got != 1 {
		t.Fatalf("wide help should fit in one content row, got %d: %q", got, wide)
	}
}

func TestDashboardBoxesSpanTerminalWidth(t *testing.T) {
	for _, width := range []int{32, 55, 120} {
		help := renderHelp(width, "ALL", false)
		firstLine := strings.Split(help, "\n")[0]
		if got := lipgloss.Width(firstLine); got != width {
			t.Fatalf("terminal width %d: help width=%d", width, got)
		}

		table := renderServiceTable([]model.Service{{
			Name: "api", LocalPort: "8080", Status: model.StatusHealthy,
		}}, 0, 0, 10, width)
		if got := lipgloss.Width(strings.Split(table, "\n")[0]); got != width {
			t.Fatalf("terminal width %d: service table width=%d", width, got)
		}
	}
}

func TestRenderedDashboardBoxesShareFullWidth(t *testing.T) {
	const width = 55
	u := &UI{
		services: []model.Service{{
			Name: "api", LocalPort: "8080", Status: model.StatusHealthy,
			Logs: []model.LogEntry{{Time: time.Unix(1, 0), Message: "ready"}},
		}},
		width: width, height: 30, ready: true,
		viewport: viewport.New(viewport.WithWidth(width), viewport.WithHeight(3)),
	}
	u.refreshViewportContent()

	topBorders := 0
	for _, line := range strings.Split(u.viewContent(), "\n") {
		plain := ansi.Strip(line)
		if strings.HasPrefix(plain, "╭") {
			topBorders++
			if got := lipgloss.Width(line); got != width {
				t.Fatalf("dashboard box width=%d, want %d: %q", got, width, plain)
			}
		}
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("dashboard line exceeds terminal: width=%d, want <=%d: %q", got, width, plain)
		}
	}
	if topBorders != 3 {
		t.Fatalf("expected service, log, and help boxes; found %d", topBorders)
	}
}

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

func TestAdaptiveHelpUsesCompactFooterInSmallSplit(t *testing.T) {
	full := dashboardHelpLines(120, 40, "ALL", false)
	if strings.Contains(ansi.Strip(strings.Join(full, "\n")), "?: all shortcuts") {
		t.Fatal("large terminal unexpectedly used compact help")
	}

	compact := dashboardHelpLines(40, 16, "ALL", false)
	plain := ansi.Strip(strings.Join(compact, "\n"))
	if !strings.Contains(plain, "?: help") || !strings.Contains(plain, "Ctrl+L: clear") {
		t.Fatalf("small split did not use compact essentials: %q", plain)
	}
	if len(compact)+2 > 5 {
		t.Fatalf("compact help is too tall: %d content rows", len(compact))
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

func TestQuestionMarkTogglesShortcutOverlay(t *testing.T) {
	u := &UI{width: 70, height: 26, ready: true}
	question := tea.KeyPressMsg{Code: '?', Text: "?"}
	u.Update(question)
	if !u.helpVisible || !strings.Contains(ansi.Strip(u.viewContent()), "SHORTCUTS") {
		t.Fatal("? did not open shortcut overlay")
	}

	u.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if u.helpVisible {
		t.Fatal("Esc did not close shortcut overlay")
	}
}

func TestNarrowLayoutPreservesHelpAndShrinksLogs(t *testing.T) {
	chrome := len(dashboardHelpLines(32, 20, "ALL", false)) + 2
	if got := calculateViewportHeight(10, 20, chrome); got != minLogViewportHeight {
		t.Fatalf("expected minimum log height %d, got %d", minLogViewportHeight, got)
	}
	if got := maxVisibleServices(10, 20, chrome); got != 8 {
		t.Fatalf("expected compact help to preserve service rows, got %d visible rows", got)
	}
}

func TestServiceTableFitsNarrowTerminal(t *testing.T) {
	for _, width := range []int{32, 40, 55} {
		out := renderServiceTable([]model.Service{{
			Name: "long-service-name", LocalPort: "15432", Status: model.StatusHealthy,
		}}, 0, 0, 10, width)
		for _, line := range strings.Split(out, "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("terminal width %d: table line is %d columns: %q", width, got, line)
			}
		}
	}
}

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
