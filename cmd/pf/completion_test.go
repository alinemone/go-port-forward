package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/alinemone/go-port-forward/internal/storage"
)

// withTempHome points storage at a throwaway config dir for the test.
func withTempHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

func TestCompleteServicesAndGroups(t *testing.T) {
	withTempHome(t)
	st := storage.NewStorage()
	if err := st.AddService("db", "kubectl port-forward svc/db 5432:5432"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddService("redis", "kubectl port-forward svc/redis 6379:6379"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddGroup("backend", []string{"db", "redis"}); err != nil {
		t.Fatal(err)
	}

	got, directive := completeServicesAndGroups(nil, nil, "")
	for _, want := range []string{"db", "redis", "backend"} {
		if !slices.Contains(got, want) {
			t.Errorf("completion missing %q, got %v", want, got)
		}
	}
	if directive&cobra.ShellCompDirectiveNoFileComp == 0 {
		t.Errorf("directive = %v, want NoFileComp set", directive)
	}

	svcs, _ := completeServices(nil, nil, "")
	if slices.Contains(svcs, "backend") {
		t.Errorf("completeServices must not include groups, got %v", svcs)
	}
}

func TestMultiCompletionBreakingShell(t *testing.T) {
	withTempHome(t)
	st := storage.NewStorage()
	for _, n := range []string{"db", "redis", "api"} {
		if err := st.AddService(n, "kubectl port-forward svc/"+n+" 1:1"); err != nil {
			t.Fatal(err)
		}
	}

	// Shell broke the comma list into args (or spaces were used): toComplete has
	// no comma → candidates are BARE names, already-chosen ones excluded.
	for _, prior := range [][]string{{"db"}, {"db", "api"}} {
		got, _ := completeServicesAndGroups(nil, prior, "")
		for _, g := range got {
			if strings.Contains(g, ",") {
				t.Fatalf("prior=%v: candidate must be bare, got %q", prior, g)
			}
		}
		if slices.Contains(got, "db") {
			t.Errorf("prior=%v: chosen 'db' must be excluded, got %v", prior, got)
		}
		if !slices.Contains(got, "redis") {
			t.Errorf("prior=%v: 'redis' must still complete, got %v", prior, got)
		}
	}
}

func TestMultiCompletionCommaList(t *testing.T) {
	withTempHome(t)
	st := storage.NewStorage()
	for _, n := range []string{"db", "redis", "api"} {
		if err := st.AddService(n, "kubectl port-forward svc/"+n+" 1:1"); err != nil {
			t.Fatal(err)
		}
	}

	// After "db," candidates are BARE names (so the shell substitutes the post-
	// comma segment and never duplicates the prefix into "db,db"). 'db' excluded.
	got, directive := completeServicesAndGroups(nil, nil, "db,")
	for _, g := range got {
		if strings.Contains(g, ",") {
			t.Fatalf("candidate must be a bare name, got %q", g)
		}
	}
	if !slices.Contains(got, "redis") || !slices.Contains(got, "api") {
		t.Fatalf("expected redis and api in %v", got)
	}
	if slices.Contains(got, "db") {
		t.Errorf("already-chosen 'db' must be excluded, got %v", got)
	}
	if directive&cobra.ShellCompDirectiveNoSpace == 0 {
		t.Errorf("comma list needs NoSpace to keep going, got %v", directive)
	}

	// A partial after the last comma filters by it (bare).
	got, _ = completeServicesAndGroups(nil, nil, "db,r")
	if !slices.Contains(got, "redis") || slices.Contains(got, "api") {
		t.Errorf(`"db,r" should offer only "redis", got %v`, got)
	}
}

func TestCompleteThemesAndIcons(t *testing.T) {
	themes, _ := completeThemes(nil, nil, "")
	for _, want := range []string{"default", "ocean", "sunset", "list"} {
		if !slices.Contains(themes, want) {
			t.Errorf("theme completion missing %q, got %v", want, themes)
		}
	}

	icons, _ := completeIconArgs(nil, nil, "")
	if !slices.Equal(icons, []string{"on", "off", "status"}) {
		t.Errorf("icon completion = %v", icons)
	}
}
