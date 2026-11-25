package manager

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/alinemone/go-port-forward/internal/kube"
	"github.com/alinemone/go-port-forward/internal/model"
)

// addressForward binds the spec's addresses (127.0.0.1 when none) like the real
// tunnel does, so two forwards on one port only coexist on distinct IPs.
func addressForward(ctx context.Context, spec kube.Spec, _ kube.Options, hooks kube.Hooks) error {
	addrs := spec.Addresses
	if len(addrs) == 0 {
		addrs = []string{"127.0.0.1"}
	}
	for _, a := range addrs {
		ln, err := net.Listen("tcp4", fmt.Sprintf("%s:%d", a, spec.Ports[0].Local))
		if err != nil {
			return err
		}
		defer ln.Close()
	}
	hooks.Ready()
	<-ctx.Done()
	return nil
}

func stateOf(m *ServiceManager, name string) model.Service {
	for _, s := range m.ListServiceStates() {
		if s.Name == name {
			return s
		}
	}
	return model.Service{}
}

func newBindIPManager(t *testing.T, aliasOn bool, services map[string]string) *ServiceManager {
	t.Helper()
	m := newTestManager(t, addressForward, services)
	if err := m.storage.SetHostAliasEnabled(aliasOn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.StopAllServices)
	return m
}

func TestSharedPortsGetDedicatedIPs(t *testing.T) {
	port := freePort(t)
	m := newBindIPManager(t, true, map[string]string{
		"salam":    fmt.Sprintf("kubectl -n prod-salam-gate port-forward svc/psql %d:5432", port),
		"logistic": fmt.Sprintf("kubectl -n prod-logistic port-forward svc/psql %d:5432", port),
		"redis":    fmt.Sprintf("kubectl -n prod port-forward svc/redis %d:6379", freePort(t)),
	})
	m.SetSharedPorts([]string{fmt.Sprint(port)})

	ctx := context.Background()
	for _, n := range []string{"salam", "logistic", "redis"} {
		if err := m.StartService(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	a, b := stateOf(m, "salam").BindIP, stateOf(m, "logistic").BindIP
	if a == "" || b == "" || a == b {
		t.Fatalf("expected two distinct dedicated IPs, got %q and %q", a, b)
	}
	if ip := stateOf(m, "redis").BindIP; ip != "" {
		t.Fatalf("non-conflicting service should stay on localhost, got %q", ip)
	}
	waitFor(t, "all healthy", func() bool {
		for _, n := range []string{"salam", "logistic", "redis"} {
			if stateOf(m, n).Status != model.StatusHealthy {
				return false
			}
		}
		return true
	})
	if portFree(port) == false {
		t.Fatal("shared port must not be held on 127.0.0.1")
	}
}

func TestLateAddOnBusyPortMovesOffLocalhost(t *testing.T) {
	port := freePort(t)
	m := newBindIPManager(t, true, map[string]string{
		"first":  fmt.Sprintf("kubectl -n a port-forward svc/db %d:5432", port),
		"second": fmt.Sprintf("kubectl -n b port-forward svc/db %d:5432", port),
	})
	ctx := context.Background()
	if err := m.StartService(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	if err := m.StartService(ctx, "second"); err != nil {
		t.Fatal(err)
	}
	if ip := stateOf(m, "first").BindIP; ip != "" {
		t.Fatalf("first service should keep localhost, got %q", ip)
	}
	if ip := stateOf(m, "second").BindIP; ip == "" {
		t.Fatal("second service on the same port should get a dedicated IP")
	}
}

func TestPortBusyElsewhereMovesOffLocalhost(t *testing.T) {
	port := freePort(t)
	m := newBindIPManager(t, true, map[string]string{
		"db": fmt.Sprintf("kubectl -n a port-forward svc/db %d:5432", port),
	})
	m.portFree = func(p int) bool { return p != port }
	if err := m.StartService(context.Background(), "db"); err != nil {
		t.Fatal(err)
	}
	if ip := stateOf(m, "db").BindIP; ip == "" {
		t.Fatal("a port held by another program should push the service to a dedicated IP")
	}
}

func TestAliasOffKeepsLocalhost(t *testing.T) {
	port := freePort(t)
	m := newBindIPManager(t, false, map[string]string{
		"db": fmt.Sprintf("kubectl -n a port-forward svc/db %d:5432", port),
	})
	m.SetSharedPorts([]string{fmt.Sprint(port)})
	if err := m.StartService(context.Background(), "db"); err != nil {
		t.Fatal(err)
	}
	if ip := stateOf(m, "db").BindIP; ip != "" {
		t.Fatalf("alias off must not change binding, got %q", ip)
	}
}
