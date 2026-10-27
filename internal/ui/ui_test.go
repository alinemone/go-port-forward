package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alinemone/go-port-forward/internal/icons"
	"github.com/alinemone/go-port-forward/internal/model"
	"github.com/charmbracelet/x/ansi"
)

var screenSizes = []struct{ w, h int }{{40, 14}, {55, 24}, {120, 40}}

// newHangarUI opens the Hangar over a few saved services and a group.
func newHangarUI(t *testing.T, width, height int) *UI {
	t.Helper()
	u, _ := newTestUI(t, width, height, model.Service{Name: "order-pg", LocalPort: "1150", Status: model.StatusHealthy})
	for name, cmd := range map[string]string{
		"order-pg":  "kubectl -n prod port-forward svc/order 1150:5432",
		"wallet-pg": "kubectl -n prod port-forward svc/wallet 1153:5432",
		"redis":     "kubectl -n prod port-forward svc/redis 6379:6379",
	} {
		if err := u.store.AddService(name, cmd); err != nil {
			t.Fatal(err)
		}
	}
	if err := u.store.AddGroup("order", []string{"order-pg", "redis"}); err != nil {
		t.Fatal(err)
	}
	u.Update(key('a'))
	return u
}

func TestHangarFillsTerminalInEveryState(t *testing.T) {
	for _, size := range screenSizes {
		for _, tc := range []struct {
			name      string
			configure func(*UI)
			want      []string
		}{
			{name: "list", want: []string{"HANGAR", "INVENTORY", "SCAN", "GROUPS", "SERVICES", "wallet-pg"}},
			{name: "new prompt", configure: func(u *UI) { u.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}) },
				want: []string{"CREATE", "g group", "s service"}},
			{name: "delete confirm", configure: func(u *UI) {
				u.manage.confirmDeleteName, u.manage.confirmDeleteKind = "redis", "service"
			}, want: []string{"CONFIRM", "Delete service 'redis'", "y delete"}},
		} {
			u := newHangarUI(t, size.w, size.h)
			if tc.configure != nil {
				tc.configure(u)
			}
			out := u.viewContent()
			assertFits(t, out, size.w, size.h)
			if size.w >= 120 {
				plain := ansi.Strip(out)
				for _, want := range tc.want {
					if !strings.Contains(plain, want) {
						t.Errorf("%s: missing %q\n%s", tc.name, want, plain)
					}
				}
			}
		}
	}
}

func TestHangarMarksSelectionAndRunning(t *testing.T) {
	u := newHangarUI(t, 120, 40)
	typeInto(u, "wallet")
	u.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	plain := ansi.Strip(u.viewContent())
	if !strings.Contains(plain, "◉ wallet-pg") {
		t.Fatalf("selected service has no ◉ mark:\n%s", plain)
	}
	if !strings.Contains(plain, "1 selected") {
		t.Errorf("header does not count the selection:\n%s", plain)
	}

	u.Update(tea.KeyPressMsg{Code: tea.KeyEsc}) // clear search
	if plain := ansi.Strip(u.viewContent()); !strings.Contains(plain, "● running") {
		t.Errorf("running service is not marked:\n%s", plain)
	}
}

func typeInto(u *UI, s string) {
	for _, r := range s {
		u.Update(key(r))
	}
}

func TestServiceFormPreviewsCommandAsYouType(t *testing.T) {
	u := newHangarUI(t, 120, 30)
	u.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	u.Update(key('s'))
	if u.serviceForm.mode != "new" {
		t.Fatal("Ctrl+N then s did not open the service form")
	}
	assertFits(t, u.viewContent(), 120, 30)

	u.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	u.Update(tea.PasteMsg{Content: "kubectl -n prod port-forward svc/db 15432:5432"})
	plain := ansi.Strip(u.viewContent())
	for _, want := range []string{"NEW SERVICE", "CONFIGURE", "✓ service/db in prod", "localhost:15432 ⇢ 5432"} {
		if !strings.Contains(plain, want) {
			t.Errorf("form is missing %q\n%s", want, plain)
		}
	}

	u.serviceForm.commandInput.SetValue("kubectl get pods")
	if plain := ansi.Strip(u.viewContent()); !strings.Contains(plain, "! only kubectl port-forward") {
		t.Errorf("invalid command is not flagged:\n%s", plain)
	}
}

func TestGroupFormFillsTerminalAndScrolls(t *testing.T) {
	u := newHangarUI(t, 60, 16)
	for i := 0; i < 20; i++ {
		_ = u.store.AddService("svc"+string(rune('a'+i)), "kubectl port-forward svc/x 1:1")
	}
	u.Update(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	u.Update(key('g'))
	if u.groupForm.mode != "new" {
		t.Fatal("Ctrl+N then g did not open the group form")
	}
	u.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	for i := 0; i < 22; i++ {
		u.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	out := u.viewContent()
	assertFits(t, out, 60, 16)
	cursorName := u.groupForm.serviceNames[u.groupForm.serviceCursor]
	if !strings.Contains(ansi.Strip(out), cursorName) {
		t.Fatalf("cursor row %q scrolled out of view:\n%s", cursorName, ansi.Strip(out))
	}
}

func TestShortcutOverlayIsFullScreenAndPageSpecific(t *testing.T) {
	u := &UI{width: 70, height: 26, aliasEnabled: true}
	for _, manage := range []bool{false, true} {
		out := u.renderShortcutOverlay(manage)
		assertFits(t, out, 70, 26)
		plain := ansi.Strip(out)
		if !strings.Contains(plain, "FLIGHT MANUAL") {
			t.Fatalf("manual header missing: %q", plain)
		}
		if manage {
			if !strings.Contains(plain, "MANAGEMENT") || !strings.Contains(plain, "create item") {
				t.Fatalf("hangar shortcuts missing: %q", plain)
			}
		} else if !strings.Contains(plain, "LOGS") || !strings.Contains(plain, "clear visible logs") {
			t.Fatalf("dashboard shortcuts missing: %q", plain)
		}
	}
}

func TestShortcutOverlayDoesNotOverflowTinySplit(t *testing.T) {
	for _, size := range []struct{ width, height int }{{32, 16}, {28, 10}} {
		u := &UI{width: size.width, height: size.height}
		assertFits(t, u.renderShortcutOverlay(false), max(24, size.width), size.height)
	}
}

func TestQuestionMarkTogglesShortcutOverlay(t *testing.T) {
	u := &UI{width: 70, height: 26, ready: true}
	u.Update(key('?'))
	if !u.helpVisible || !strings.Contains(ansi.Strip(u.viewContent()), "SHORTCUTS") {
		t.Fatal("? did not open shortcut overlay")
	}
	u.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if u.helpVisible {
		t.Fatal("Esc did not close shortcut overlay")
	}
}

func TestEmptyStatesShowStarfield(t *testing.T) {
	u, _ := newTestUI(t, 120, 30)
	plain := ansi.Strip(u.viewContent())
	for _, want := range []string{"MISSION CONTROL", "no active links", "awaiting telemetry"} {
		if !strings.Contains(plain, want) {
			t.Errorf("empty dashboard is missing %q\n%s", want, plain)
		}
	}
}

func TestShutdownScreenFillsTerminal(t *testing.T) {
	u, _ := newTestUI(t, 100, 30, sampleServices()...)
	u.quitting = true
	out := u.viewContent()
	assertFits(t, out, 100, 30)
	if !strings.Contains(ansi.Strip(out), "DISENGAGING") {
		t.Fatal("shutdown screen has no message")
	}
}

func TestStarfieldIsDeterministicAndExact(t *testing.T) {
	a := starfield(50, 4, 1, 0, "")
	b := starfield(50, 4, 1, 0, "")
	if strings.Join(a, "\n") != strings.Join(b, "\n") {
		t.Fatal("the same field must render the same stars")
	}
	for _, line := range starfield(50, 5, 2, 9, "hello") {
		if w := len([]rune(ansi.Strip(line))); w != 50 {
			t.Fatalf("starfield line is %d wide, want 50", w)
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
