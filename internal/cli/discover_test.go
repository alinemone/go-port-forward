package cli

import (
	"reflect"
	"testing"

	"github.com/alinemone/go-port-forward/internal/storage"
)

func TestDiscoverAliasD(t *testing.T) {
	root := newRootCmd()
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"d", "prod-salam-gate"}, "discover"},
		{[]string{"scan"}, "discover"},
		{[]string{"discover"}, "discover"},
		{[]string{"rm", "db"}, "delete"},
		{[]string{"delete", "db"}, "delete"},
	} {
		cmd, _, err := root.Find(tc.args)
		if err != nil || cmd.Name() != tc.want {
			t.Errorf("pf %v → %q (err %v), want %q", tc.args, cmd.Name(), err, tc.want)
		}
	}
}

func TestDiscoverCompletesSavedNamespaces(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	st := storage.NewStorage()
	_ = st.AddService("a", "kubectl -n prod-salam-gate port-forward svc/x 1:1")
	_ = st.AddService("b", "kubectl -n prod-api-ticket port-forward svc/y 2:2")
	_ = st.AddService("c", "kubectl port-forward svc/z -n prod-salam-gate 3:3")

	cmd, _, _ := newRootCmd().Find([]string{"d"})
	got, _ := cmd.ValidArgsFunction(cmd, nil, "")
	if want := []string{"prod-api-ticket", "prod-salam-gate"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("completion = %v, want %v", got, want)
	}
	if got, _ := cmd.ValidArgsFunction(cmd, []string{"x"}, ""); got != nil {
		t.Errorf("a second argument must not be completed: %v", got)
	}
}
