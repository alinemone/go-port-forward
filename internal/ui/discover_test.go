package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/alinemone/go-port-forward/internal/kube"
	"github.com/charmbracelet/x/ansi"
)

// fakeCluster stands in for the kube calls the wizard makes, with services
// shaped like the real prod-salam-gate namespace.
func fakeCluster(t *testing.T) {
	t.Helper()
	oc, on, os := kubeContexts, listNamespaces, discoverServices
	t.Cleanup(func() { kubeContexts, listNamespaces, discoverServices = oc, on, os })

	kubeContexts = func() ([]string, string, error) { return []string{"production", "stage"}, "production", nil }
	listNamespaces = func(context.Context, string, kube.Options) ([]string, error) {
		return []string{"prod-api-ticket", "prod-salam-gate", "prod-salam-pay-api"}, nil
	}
	discoverServices = func(_ context.Context, _, ns string, _ kube.Options) ([]kube.Candidate, error) {
		if ns != "prod-salam-gate" {
			return nil, errors.New(`namespaces "` + ns + `" not found`)
		}
		return []kube.Candidate{
			{Service: "prod-salam-gate-psql-postgresql-ha", Kind: "postgres", Short: "pg", Port: 5432, Tier: kube.TierSuggested},
			{Service: "prod-salam-gate-redis-haproxy", Kind: "redis", Short: "redis", Port: 6379, Tier: kube.TierSuggested},
			{Service: "prod-salam-gate-psql-postgresql-ha-slave", Kind: "postgres", Short: "pg", Port: 5432, Tier: kube.TierMore, Note: "replica"},
			{Service: "prod-salam-gate", Port: 80, Tier: kube.TierOther},
		}, nil
	}
}

// drain runs cmd and feeds every message it yields back into the UI (timers
// and cursor blinks are skipped).
func drain(u *UI, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		switch msg := msg.(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				drain(u, c)
			}
		case namespacesMsg, scanMsg:
			_, next := u.Update(msg)
			drain(u, next)
		}
	case <-time.After(200 * time.Millisecond):
	}
}

func press(u *UI, k tea.KeyPressMsg) { _, cmd := u.Update(k); drain(u, cmd) }

func TestDiscoverWizardEndToEnd(t *testing.T) {
	fakeCluster(t)
	u := newHangarUI(t, 120, 36)
	if err := u.store.AddService("old-redis", "kubectl -n prod-salam-gate port-forward svc/prod-salam-gate-redis-haproxy 6383:6379"); err != nil {
		t.Fatal(err)
	}

	// Hangar → Ctrl+N → d opens the wizard.
	press(u, tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	press(u, key('d'))
	if !u.discover.active || u.discover.step != stepTarget {
		t.Fatal("Ctrl+N, d did not open discovery")
	}
	assertFits(t, u.viewContent(), 120, 36)

	// Typing filters namespace suggestions; Tab completes.
	for _, r := range "salam-g" {
		press(u, key(r))
	}
	if plain := ansi.Strip(u.viewContent()); !strings.Contains(plain, "prod-salam-gate") || strings.Contains(plain, "prod-api-ticket") {
		t.Fatalf("suggestions not filtered:\n%s", plain)
	}
	press(u, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := u.discover.nsInput.Value(); got != "prod-salam-gate" {
		t.Fatalf("Tab completed to %q", got)
	}

	// Enter scans. Nothing is pre-selected; saved services can't be picked.
	press(u, tea.KeyPressMsg{Code: tea.KeyEnter})
	if u.discover.step != stepSelect || u.discover.loading {
		t.Fatalf("scan did not finish: step=%d loading=%v err=%q", u.discover.step, u.discover.loading, u.discover.scanError)
	}
	assertFits(t, u.viewContent(), 120, 36)
	plain := ansi.Strip(u.viewContent())
	for _, want := range []string{"SUGGESTED", "MORE DATA STORES", "OTHER SERVICES", "✓ saved as old-redis", "replica"} {
		if !strings.Contains(plain, want) {
			t.Errorf("select step is missing %q\n%s", want, plain)
		}
	}
	if u.discoverSelectionCount() != 0 {
		t.Fatalf("something was pre-selected: %v", u.discover.selected)
	}
	press(u, tea.KeyPressMsg{Code: tea.KeyEnter})
	if u.discover.step != stepSelect {
		t.Fatal("Enter with nothing picked must stay on the select step")
	}

	// Pick postgres; try the already-saved redis (refused); pick the replica.
	space := tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	press(u, space)
	press(u, tea.KeyPressMsg{Code: tea.KeyDown})
	press(u, space)
	if u.discover.selected[1] {
		t.Fatal("an already-saved service could be picked")
	}
	press(u, tea.KeyPressMsg{Code: tea.KeyDown})
	press(u, space)
	press(u, tea.KeyPressMsg{Code: tea.KeyEnter})
	if u.discover.step != stepReview || len(u.discover.items) != 2 {
		t.Fatalf("review has %d items", len(u.discover.items))
	}
	assertFits(t, u.viewContent(), 120, 36)

	pg, replica := &u.discover.items[0], &u.discover.items[1]
	if pg.name.Value() != "salam-gate-pg" || replica.name.Value() != "salam-gate-pg-replica" {
		t.Fatalf("proposed names %q, %q", pg.name.Value(), replica.name.Value())
	}
	if pg.port.Value() == replica.port.Value() {
		t.Fatal("two items proposed the same local port")
	}

	// Editing the port rewrites the command…
	press(u, tea.KeyPressMsg{Code: tea.KeyTab}) // → pg port
	pg.port.SetValue("")
	for _, r := range "1199" {
		press(u, key(r))
	}
	if want := "kubectl -n prod-salam-gate port-forward svc/prod-salam-gate-psql-postgresql-ha 1199:5432"; pg.command.Value() != want {
		t.Fatalf("command = %q, want %q", pg.command.Value(), want)
	}
	// …until the command is edited by hand.
	press(u, tea.KeyPressMsg{Code: tea.KeyTab}) // → pg command
	press(u, key(' '))
	before := pg.command.Value()
	press(u, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}) // ← pg port
	press(u, key('9'))
	if pg.command.Value() != before {
		t.Fatal("a hand-edited command was overwritten")
	}

	// Enter saves everything and returns to the Hangar with them selected.
	press(u, tea.KeyPressMsg{Code: tea.KeyEnter})
	if u.discover.active {
		t.Fatalf("wizard still open: %s", u.discover.errorMsg)
	}
	for _, name := range []string{"salam-gate-pg", "salam-gate-pg-replica"} {
		if _, err := u.store.GetService(name); err != nil {
			t.Errorf("%s not saved: %v", name, err)
		}
		if !u.manage.selectedServices[name] {
			t.Errorf("%s not selected for launch", name)
		}
	}
	if !strings.Contains(u.manage.infoMsg, "Enter to launch") {
		t.Errorf("info = %q", u.manage.infoMsg)
	}
}

func TestDiscoverBlocksDuplicateNames(t *testing.T) {
	fakeCluster(t)
	u := newHangarUI(t, 120, 36)
	_ = u.store.AddService("salam-gate-pg", "kubectl port-forward svc/x 1:1")

	drain(u, u.openDiscover("prod-salam-gate", false))
	press(u, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}) // pick postgres
	press(u, tea.KeyPressMsg{Code: tea.KeyEnter})
	if u.discover.step != stepReview {
		t.Fatal("did not reach review")
	}
	// The proposal already avoids the taken name…
	if got := u.discover.items[0].name.Value(); got != "salam-gate-pg-2" {
		t.Fatalf("proposed %q, want salam-gate-pg-2", got)
	}
	// …and a name forced back onto it is refused.
	u.discover.items[0].name.SetValue("salam-gate-pg")
	press(u, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !u.discover.active || !strings.Contains(u.discover.errorMsg, "already exists") {
		t.Fatalf("duplicate name was not refused: %q", u.discover.errorMsg)
	}
}

func TestDiscoverScanErrorIsShown(t *testing.T) {
	fakeCluster(t)
	u := newHangarUI(t, 100, 24)
	drain(u, u.openDiscover("no-such-ns", false))
	plain := ansi.Strip(u.viewContent())
	if !strings.Contains(plain, `namespaces "no-such-ns" not found`) {
		t.Fatalf("scan error not shown:\n%s", plain)
	}
	press(u, tea.KeyPressMsg{Code: tea.KeyEsc})
	if u.discover.step != stepTarget {
		t.Fatal("Esc should go back to the namespace step")
	}
}

func TestProposeName(t *testing.T) {
	for _, tc := range []struct {
		ns   string
		c    kube.Candidate
		want string
	}{
		{"prod-salam-gate", kube.Candidate{Short: "pg"}, "salam-gate-pg"},
		{"prod-api-ticket", kube.Candidate{Short: "redis"}, "ticket-redis"},
		{"prod-salam-gate", kube.Candidate{Short: "pg", Note: "replica"}, "salam-gate-pg-replica"},
		{"prod-salam-gate", kube.Candidate{Service: "prod-salam-gate-report"}, "salam-gate-report"},
	} {
		if got := proposeName(tc.ns, tc.c); got != tc.want {
			t.Errorf("proposeName(%q, %+v) = %q, want %q", tc.ns, tc.c, got, tc.want)
		}
	}
}

func TestDiscoverFallsBackToSavedNamespaces(t *testing.T) {
	fakeCluster(t)
	listNamespaces = func(context.Context, string, kube.Options) ([]string, error) {
		return nil, errors.New(`namespaces is forbidden: User "x" cannot list resource "namespaces"`)
	}
	u := newHangarUI(t, 120, 30) // saved services live in namespace "prod"
	drain(u, u.openDiscover("", false))
	plain := ansi.Strip(u.viewContent())
	if !strings.Contains(plain, "from your saved services") || !strings.Contains(plain, "    prod") {
		t.Fatalf("no fallback suggestions:\n%s", plain)
	}
}

func TestDashboardDKeyOpensDiscovery(t *testing.T) {
	fakeCluster(t)
	u, _ := newTestUI(t, 120, 30, sampleServices()...)

	press(u, key('d'))
	if !u.discover.active || u.discover.step != stepTarget {
		t.Fatal("d on the dashboard did not open discovery")
	}
	assertFits(t, u.viewContent(), 120, 30)

	// Cancelling goes straight back to the dashboard, not the Hangar.
	press(u, tea.KeyPressMsg{Code: tea.KeyEsc})
	if u.discover.active || u.manage.active {
		t.Fatalf("Esc should return to the dashboard (discover=%v hangar=%v)", u.discover.active, u.manage.active)
	}
	if !strings.Contains(ansi.Strip(u.viewContent()), "MISSION CONTROL") {
		t.Fatal("not back on the dashboard")
	}

	// Adding lands in the Hangar with the new services ready to launch.
	press(u, key('d'))
	u.discover.nsInput.SetValue("prod-salam-gate")
	press(u, tea.KeyPressMsg{Code: tea.KeyEnter})            // scan
	press(u, tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}) // pick postgres
	press(u, tea.KeyPressMsg{Code: tea.KeyEnter})            // review
	press(u, tea.KeyPressMsg{Code: tea.KeyEnter})            // add
	if u.discover.active || !u.manage.active || !u.manage.selectedServices["salam-gate-pg"] {
		t.Fatalf("after adding: discover=%v hangar=%v selected=%v", u.discover.active, u.manage.active, u.manage.selectedServices)
	}
}

func TestDiscoveryIsDocumentedOnScreen(t *testing.T) {
	empty, _ := newTestUI(t, 120, 30)
	if !strings.Contains(ansi.Strip(empty.viewContent()), "d to discover a namespace") {
		t.Error("empty dashboard does not mention discovery")
	}

	u := &UI{width: 100, height: 30, aliasEnabled: true}
	if !strings.Contains(ansi.Strip(u.renderShortcutOverlay(false)), "discover DBs & caches") {
		t.Error("dashboard manual does not list D")
	}
	if !strings.Contains(ansi.Strip(u.renderShortcutOverlay(true)), "discover from a namespace") {
		t.Error("hangar manual does not list Ctrl+N, D")
	}

	h := newHangarUI(t, 120, 30)
	if !strings.Contains(ansi.Strip(h.viewContent()), "create / discover") {
		t.Error("hangar key bar does not mention discovery")
	}
}
