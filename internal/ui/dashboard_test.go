package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alinemone/go-port-forward/internal/icons"
	"github.com/alinemone/go-port-forward/internal/model"
	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/theme"
	"github.com/charmbracelet/x/ansi"
)

// fakeController reports a fixed set of running services and records stops.
type fakeController struct {
	mu           sync.Mutex
	services     map[string]model.Service
	restartedAll bool
}

func newFakeController(running ...model.Service) *fakeController {
	f := &fakeController{services: map[string]model.Service{}}
	for _, s := range running {
		f.services[s.Name] = s
	}
	return f
}

func (f *fakeController) ListServiceStates() []model.Service {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]model.Service, 0, len(f.services))
	for _, s := range f.services {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (f *fakeController) StartStoredService(context.Context, string) error { return nil }

func (f *fakeController) StopService(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.services, name)
}

func (f *fakeController) StopAllServices()                             {}
func (f *fakeController) RestartService(context.Context, string) error { return nil }
func (f *fakeController) RestartAllServices(context.Context)           { f.restartedAll = true }
func (f *fakeController) ClearLogs(string)                             {}

// newTestUI returns a sized, ready dashboard over an isolated config dir.
func newTestUI(t *testing.T, width, height int, running ...model.Service) (*UI, *fakeController) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	ctrl := newFakeController(running...)
	u := NewUI(context.Background(), ctrl, storage.NewStorage())
	u.Update(tea.WindowSizeMsg{Width: width, Height: height})
	u.services = ctrl.ListServiceStates()
	u.refreshViewportContent()
	return u, ctrl
}

func sampleServices() []model.Service {
	now := time.Now()
	return []model.Service{
		{
			Name: "order-pg", LocalPort: "1150", Status: model.StatusHealthy,
			ConnectedSince: now.Add(-62 * time.Minute), BytesIn: 1 << 20,
			Logs: []model.LogEntry{{Time: now, Message: "Forwarding from 127.0.0.1:1150 -> 5432"}},
		},
		{
			Name: "salamgate-redis", LocalPort: "6383", Status: model.StatusError,
			LastError: "no running pod found for service/prod-salam-gate-redis",
		},
		{Name: "core", LocalPort: "1119", Status: model.StatusConnecting},
	}
}

func assertFits(t *testing.T, out string, width, height int) {
	t.Helper()
	lines := strings.Split(out, "\n")
	if len(lines) != height {
		t.Fatalf("%dx%d: rendered %d lines, want %d\n%s", width, height, len(lines), height, ansi.Strip(out))
	}
	for _, line := range lines {
		if got := lipgloss.Width(line); got != width {
			t.Fatalf("%dx%d: line is %d wide, want %d: %q", width, height, got, width, ansi.Strip(line))
		}
	}
}

func key(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func TestDashboardFillsTerminalExactly(t *testing.T) {
	for _, size := range []struct{ w, h int }{{40, 12}, {60, 20}, {90, 24}, {120, 32}, {200, 50}} {
		u, _ := newTestUI(t, size.w, size.h, sampleServices()...)
		assertFits(t, u.viewContent(), size.w, size.h)

		empty, _ := newTestUI(t, size.w, size.h)
		assertFits(t, empty.viewContent(), size.w, size.h)
	}
}

func TestRowShowsOnlyEssentials(t *testing.T) {
	u, _ := newTestUI(t, 120, 30, sampleServices()...)
	plain := ansi.Strip(u.viewContent())
	for _, want := range []string{
		"order-pg", "live", ":1150", "1h 02m", // name, status, port, uptime
		"salamgate-redis", "error", "no running pod found", // why it failed
		"core", "connecting",
		"Forwarding from 127.0.0.1:1150", // logs
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("dashboard is missing %q\n%s", want, plain)
		}
	}
}

func TestLiveRowHasStreamingChart(t *testing.T) {
	u, _ := newTestUI(t, 120, 30, sampleServices()...)
	start := time.Now()
	u.sampleTraffic(start)
	for i := 1; i <= 10; i++ {
		u.services[1].BytesIn += int64(i * 4096) // order-pg (core sorts first)
		u.sampleTraffic(start.Add(time.Duration(i) * 500 * time.Millisecond))
	}

	var row string
	for _, line := range strings.Split(ansi.Strip(u.viewContent()), "\n") {
		if strings.Contains(line, "order-pg") {
			row = line
			break // the service row; later matches are log lines
		}
	}
	if !strings.ContainsAny(row, "⣤⣶⣿⣷⣦") {
		t.Fatalf("live row has no chart peaks: %q", row)
	}
	if !strings.Contains(row, "K") {
		t.Errorf("live row is missing the current rate: %q", row)
	}
}

func TestStreamChartShapes(t *testing.T) {
	hue := serviceHue("db")
	idle := ansi.Strip(streamChart(nil, 6, hue))
	if idle != strings.Repeat("⣀", 6) {
		t.Fatalf("idle chart = %q, want a flat baseline", idle)
	}
	peak := ansi.Strip(streamChart([]float64{0, 0, 0, chartReference}, 2, hue))
	if []rune(peak)[1] != brailleCell(1, 4) {
		t.Fatalf("newest sample should peak at the right edge: %q", peak)
	}
	// Steady small chatter stays a low ripple instead of a solid wall.
	steady := []rune(ansi.Strip(streamChart([]float64{1000, 1000, 1000, 1000}, 2, hue)))
	if steady[0] == brailleCell(4, 4) || steady[1] == brailleCell(4, 4) {
		t.Fatalf("1 KB/s chatter drawn at full height: %q", string(steady))
	}
	if got := lipgloss.Width(streamChart([]float64{5, 9, 1}, 10, hue)); got != 10 {
		t.Fatalf("chart width = %d, want 10", got)
	}
}

func TestServiceHueIsStable(t *testing.T) {
	if serviceHue("order-pg") != serviceHue("order-pg") {
		t.Fatal("a service's color must not change between renders")
	}
}

func TestOriginalKeysStillWork(t *testing.T) {
	u, ctrl := newTestUI(t, 120, 30, sampleServices()...)

	u.Update(key('a'))
	if !u.manage.active {
		t.Fatal("a must open the add/edit overlay as before")
	}
	u.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	u.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if u.manage.active {
		t.Fatal("esc should leave the overlay")
	}

	u.Update(key('l'))
	if !u.logFilterSelected {
		t.Fatal("l must still toggle selected-service logs")
	}

	_, cmd := u.Update(key('s'))
	if cmd != nil {
		cmd()
	}
	if _, ok := ctrl.services["core"]; ok {
		t.Error("s must still stop the selected service")
	}
}

func TestEscDoesNotQuitDashboard(t *testing.T) {
	u, _ := newTestUI(t, 120, 30, sampleServices()...)
	u.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if u.quitting {
		t.Fatal("Esc must not stop every forward; quitting is q / Ctrl+C")
	}
	u.Update(key('q'))
	if !u.quitting {
		t.Fatal("q must quit")
	}
}

func TestTableScrollsToKeepCursorVisible(t *testing.T) {
	var many []model.Service
	for i := 0; i < 30; i++ {
		many = append(many, model.Service{Name: fmt.Sprintf("svc-%02d", i), LocalPort: "1", Status: model.StatusHealthy})
	}
	u, _ := newTestUI(t, 120, 24, many...)
	for i := 0; i < 29; i++ {
		u.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if out := ansi.Strip(u.viewContent()); !strings.Contains(out, "svc-29") || !strings.Contains(out, "above") {
		t.Fatalf("cursor row scrolled out of view:\n%s", out)
	}
	assertFits(t, u.viewContent(), 120, 24)
}

func TestRefreshViewportFollowsNewLogs(t *testing.T) {
	logs := make([]model.LogEntry, 30)
	for i := range logs {
		logs[i] = model.LogEntry{Time: time.Unix(int64(i+1), 0), Message: fmt.Sprintf("line %d", i)}
	}
	u := &UI{
		services: []model.Service{{Name: "api", Logs: logs}},
		width:    80, height: 20, ready: true,
		viewport: viewport.New(),
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

	u.services[0].Logs = append(u.services[0].Logs, model.LogEntry{Time: time.Unix(31, 0), Message: "newest line"})
	u.refreshViewportContent()
	if !u.viewport.AtBottom() {
		t.Fatal("new log did not move viewport to the latest output")
	}
}

func TestSampleTrafficComputesRates(t *testing.T) {
	u := &UI{services: []model.Service{{Name: "db"}}}
	start := time.Now()
	u.sampleTraffic(start)

	u.services[0].BytesIn, u.services[0].BytesOut = 3000, 1000
	u.sampleTraffic(start.Add(2 * time.Second))
	if got := u.traffic["db"].current(); got != 2000 {
		t.Fatalf("rate = %v B/s, want 2000", got)
	}

	u.services = nil
	u.sampleTraffic(start.Add(3 * time.Second))
	if _, ok := u.traffic["db"]; ok {
		t.Fatal("history of a stopped service was kept")
	}
}

func TestServiceStatusColorsAreThemeIndependent(t *testing.T) {
	theme.Set("sunset") // a theme whose accent/accentAlt are pink, not green
	ApplyTheme()
	defer func() { theme.Set(""); ApplyTheme() }()

	u, _ := newTestUI(t, 120, 30, model.Service{Name: "db", LocalPort: "5432", Status: model.StatusHealthy})
	// live must stay the fixed green (#73FFB6 = 115;255;182) under any theme.
	if out := u.viewContent(); !strings.Contains(out, "115;255;182") {
		t.Fatalf("live status must be fixed green regardless of theme")
	}
}

func TestFormatHelpers(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{formatRate(0), ""},
		{formatRate(512), "512B"},
		{formatRate(2048), "2K"},
		{formatRate(3.4 * 1024 * 1024), "3.4M"},
		{formatDuration(42 * time.Second), "42s"},
		{formatDuration(62 * time.Minute), "1h 02m"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}

func TestRowsShowPortIconOnlyWhenEnabled(t *testing.T) {
	svc := model.Service{Name: "db", LocalPort: "15432", MainPort: "5432", Status: model.StatusHealthy}
	u, _ := newTestUI(t, 120, 30, svc)
	if strings.Contains(u.viewContent(), icons.ForPort("5432").Glyph) {
		t.Fatal("icon drawn although icons are off")
	}

	svc.IconEnabled = true
	u, _ = newTestUI(t, 120, 30, svc)
	out := u.viewContent()
	if !strings.Contains(out, icons.ForPort("5432").Glyph) {
		t.Fatal("port icon missing with icons on")
	}
	assertFits(t, out, 120, 30)

	svc.MainPort = "18081"
	u, _ = newTestUI(t, 120, 30, svc)
	if !strings.Contains(u.viewContent(), icons.DefaultGlyph) {
		t.Fatal("default icon missing for an unknown port")
	}
}

func TestCtrlRRestartsAll(t *testing.T) {
	u, ctrl := newTestUI(t, 120, 30, sampleServices()...)
	u.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if !ctrl.restartedAll {
		t.Fatal("ctrl+r must restart every service")
	}
}

func TestFooterListsEveryKey(t *testing.T) {
	for _, width := range []int{70, 120, 200} {
		u, _ := newTestUI(t, width, 30, sampleServices()...)
		u.aliasEnabled = true
		out := ansi.Strip(u.viewContent())
		// Each key must be present, in its full or (narrow terminal) compact label.
		for _, want := range [][2]string{
			{"L/Ctrl+L view / clear logs", "L/^L logs"}, {"y copy alias", "y copy"},
			{"a add/edit services", "a add/edit"}, {"d discover namespace", "d discover"},
			{"c edit config", "c config"},
			{"r/Ctrl+R restart selected / all", "r/^R restart"}, {"s stop selected", "s stop"},
			{"q quit & stop all", "q quit"},
		} {
			if !strings.Contains(out, want[0]) && !strings.Contains(out, want[1]) {
				t.Errorf("width %d: footer is missing %q\n%s", width, want[0], out)
			}
		}
		assertFits(t, u.viewContent(), width, 30)
	}
}

func TestFooterHasNoSeparateGroupsKey(t *testing.T) {
	u, _ := newTestUI(t, 200, 30, sampleServices()...)
	if strings.Contains(ansi.Strip(u.viewContent()), "g groups") {
		t.Fatal("groups live inside the a overlay; no separate g hint")
	}
}

func infoServices() []model.Service {
	svcs := sampleServices()
	svcs[0].MainPort, svcs[0].Namespace, svcs[0].Context = "5432", "orders", "rostam"
	svcs[1].MainPort, svcs[1].Namespace, svcs[1].Context = "6379", "salam", "prod"
	svcs[2].MainPort, svcs[2].Namespace, svcs[2].Context = "5432", "core", "rostam"
	return svcs
}

// storeServices saves the running services to the config so they can be grouped.
func storeServices(t *testing.T, u *UI) {
	t.Helper()
	for _, svc := range u.services {
		if err := u.store.AddService(svc.Name, "kubectl port-forward svc/"+svc.Name+" "+svc.LocalPort+":"+svc.MainPort); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInfoToggleShowsDetailColumnsAndPersists(t *testing.T) {
	u, _ := newTestUI(t, 160, 30, infoServices()...)
	storeServices(t, u)
	if err := u.store.AddGroup("db", []string{"order-pg", "core"}); err != nil {
		t.Fatal(err)
	}

	plain := ansi.Strip(u.viewContent())
	if strings.Contains(plain, "orders") || strings.Contains(plain, "→5432") {
		t.Fatalf("details must be off by default\n%s", plain)
	}

	u.Update(key('i'))
	plain = ansi.Strip(u.viewContent())
	for _, want := range []string{"db", "rostam", "prod", "orders", " 1150→5432", " 6383→6379"} {
		if !strings.Contains(plain, want) {
			t.Errorf("details view is missing %q\n%s", want, plain)
		}
	}
	assertFits(t, u.viewContent(), 160, 30)
	if on, _ := u.store.InfoEnabled(); !on {
		t.Fatal("the details toggle must be saved")
	}
	if again := NewUI(context.Background(), newFakeController(), u.store); !again.info {
		t.Fatal("a new dashboard must start with the saved details setting")
	}

	u.Update(key('i'))
	if plain = ansi.Strip(u.viewContent()); strings.Contains(plain, "orders") {
		t.Fatalf("a second i must hide the details\n%s", plain)
	}
}

func TestInfoHidesUniformContextAndMissingGroups(t *testing.T) {
	svcs := infoServices()
	for i := range svcs {
		svcs[i].Context = "rostam"
	}
	u, _ := newTestUI(t, 160, 30, svcs...)
	u.Update(key('i'))
	plain := ansi.Strip(u.viewContent())
	if !strings.Contains(plain, "● order-pg") {
		t.Errorf("no service has a group: the name must follow the status glyph\n%s", plain)
	}
	for _, hidden := range []string{"rostam"} {
		if strings.Contains(plain, hidden) {
			t.Errorf("%q must be hidden when it says nothing\n%s", hidden, plain)
		}
	}
	if !strings.Contains(plain, "orders") {
		t.Errorf("namespace is always shown with details\n%s", plain)
	}
}

func TestInfoFitsEveryTerminal(t *testing.T) {
	for _, size := range []struct{ w, h int }{{40, 12}, {60, 20}, {90, 24}, {120, 32}, {200, 50}} {
		u, _ := newTestUI(t, size.w, size.h, infoServices()...)
		storeServices(t, u)
		if err := u.store.AddGroup("a-rather-long-group-name", []string{"core"}); err != nil {
			t.Fatal(err)
		}
		u.Update(key('i'))
		assertFits(t, u.viewContent(), size.w, size.h)
	}
}

func TestServiceGroupsJoinsSorted(t *testing.T) {
	got := serviceGroups(map[string][]string{"web": {"api"}, "db": {"api", "pg"}})
	if got["api"] != "db, web" || got["pg"] != "db" || got["x"] != "" {
		t.Fatalf("serviceGroups = %v", got)
	}
}

func TestLongNamesShowInFullAndAlign(t *testing.T) {
	svcs := infoServices()
	svcs[0].Name = "a-very-long-service-name-for-orders-pg"
	svcs[1].Namespace = "a-really-long-namespace-name"
	u, _ := newTestUI(t, 200, 30, svcs...)
	u.Update(key('i'))
	plain := ansi.Strip(u.viewContent())
	for _, want := range []string{svcs[0].Name, svcs[1].Namespace} {
		if !strings.Contains(plain, want) {
			t.Errorf("%q must not be truncated\n%s", want, plain)
		}
	}
	// every row's port starts in the same column
	col := -1
	for _, line := range strings.Split(plain, "\n") {
		if i := strings.Index(line, "→"); i >= 0 && strings.Contains(line, "│") {
			if w := lipgloss.Width(line[:i]); col == -1 {
				col = w
			} else if w != col {
				t.Fatalf("ports are not aligned (%d vs %d)\n%s", w, col, plain)
			}
		}
	}
	if col == -1 {
		t.Fatalf("no detail rows found\n%s", plain)
	}
}

func TestInfoOrdersByGroupAndKeepsCursor(t *testing.T) {
	svcs := []model.Service{
		{Name: "alpha", LocalPort: "1"}, {Name: "beta", LocalPort: "2"},
		{Name: "gamma", LocalPort: "3"}, {Name: "zeta", LocalPort: "4"},
	}
	u, _ := newTestUI(t, 160, 30, svcs...)
	storeServices(t, u)
	_ = u.store.AddGroup("x-group", []string{"alpha"})
	_ = u.store.AddGroup("a-group", []string{"zeta", "gamma"})
	u.cursorIndex = 3 // zeta

	u.Update(key('i'))
	var got []string
	for _, s := range u.services {
		got = append(got, s.Name)
	}
	if want := "gamma zeta alpha beta"; strings.Join(got, " ") != want {
		t.Fatalf("order = %v, want %s (by group, ungrouped last)", got, want)
	}
	if u.services[u.cursorIndex].Name != "zeta" {
		t.Fatalf("cursor moved to %s, must stay on zeta", u.services[u.cursorIndex].Name)
	}
	if plain := ansi.Strip(u.viewContent()); !strings.Contains(plain, "zeta       a-group") {
		t.Fatalf("the group must follow the service name\n%s", plain)
	}

	u.Update(key('i'))
	if u.services[0].Name != "alpha" || u.services[u.cursorIndex].Name != "zeta" {
		t.Fatal("without details the order is by name again, cursor kept")
	}
}
