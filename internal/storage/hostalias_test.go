package storage

import (
	"os"
	"testing"
)

func TestClusterHostFromCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
		wantOK  bool
	}{
		{
			name:    "real dastyar-pg command",
			command: "kubectl -n prod-operation port-forward svc/operation-dastyar-psql-postgresql-ha 1116:5432",
			want:    "operation-dastyar-psql-postgresql-ha.prod-operation.svc.cluster.local",
			wantOK:  true,
		},
		{
			name:    "namespace equals form + service/ prefix",
			command: "kubectl --namespace=foo port-forward service/bar 1:2",
			want:    "bar.foo.svc.cluster.local",
			wantOK:  true,
		},
		{
			name:    "namespace space form + services/ prefix",
			command: "kubectl --namespace ns port-forward services/x 1:2",
			want:    "x.ns.svc.cluster.local",
			wantOK:  true,
		},
		{
			name:    "kafka bootstrap with hyphenated ns and name",
			command: "kubectl -n prod-kafka-cdc port-forward svc/prod-kafka-cdc-kafka-bootstrap 9301:9092",
			want:    "prod-kafka-cdc-kafka-bootstrap.prod-kafka-cdc.svc.cluster.local",
			wantOK:  true,
		},
		{
			name:    "deployment target, namespace last",
			command: "kubectl port-forward deploy/prod-api-dastyar-redis-haproxy 6399:6379 -n prod-api-dastyar",
			want:    "prod-api-dastyar-redis-haproxy.prod-api-dastyar.svc.cluster.local",
			wantOK:  true,
		},
		{
			name:    "statefulset target",
			command: "kubectl -n data port-forward statefulset/my-db 1:2",
			want:    "my-db.data.svc.cluster.local",
			wantOK:  true,
		},
		{
			name:    "namespace after the target",
			command: "kubectl port-forward svc/db 5432:5432 -n prod",
			want:    "db.prod.svc.cluster.local",
			wantOK:  true,
		},
		{
			name:    "no namespace flag -> skipped",
			command: "kubectl port-forward svc/db 5432:5432",
			wantOK:  false,
		},
		{
			name:    "pod forward -> skipped",
			command: "kubectl -n ns port-forward pod/x 1:2",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ClusterHostFromCommand(tt.command)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (host=%q)", ok, tt.wantOK, got)
			}
			if ok && got != tt.want {
				t.Errorf("host = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHostAliasEnabledDefaultsOff(t *testing.T) {
	s := newTestStorage(t)

	on, err := s.HostAliasEnabled()
	if err != nil {
		t.Fatalf("HostAliasEnabled: %v", err)
	}
	if on {
		t.Fatal("expected aliases OFF by default on a fresh config")
	}
}

func TestSetHostAliasEnabledRoundTrip(t *testing.T) {
	s := newTestStorage(t)

	if err := s.SetHostAliasEnabled(false); err != nil {
		t.Fatalf("SetHostAliasEnabled(false): %v", err)
	}
	if on, _ := s.HostAliasEnabled(); on {
		t.Fatal("expected OFF after disabling")
	}

	if err := s.SetHostAliasEnabled(true); err != nil {
		t.Fatalf("SetHostAliasEnabled(true): %v", err)
	}
	if on, _ := s.HostAliasEnabled(); !on {
		t.Fatal("expected ON after re-enabling")
	}
}

func TestSetHostAliasPreservesServicesAndCustomAliases(t *testing.T) {
	s := newTestStorage(t)

	if err := s.AddService("db", "kubectl -n ns port-forward svc/db 1116:5432"); err != nil {
		t.Fatalf("AddService: %v", err)
	}
	if err := s.SetHostAliasEnabled(false); err != nil {
		t.Fatalf("SetHostAliasEnabled: %v", err)
	}

	if _, err := s.GetService("db"); err != nil {
		t.Fatalf("service lost after toggling alias: %v", err)
	}
}

func TestHostAliasOnlyConfigParses(t *testing.T) {
	s := newTestStorage(t)

	// A config that carries only the hostAlias key must still parse as the new
	// format (not fall through to the legacy map path).
	if err := os.WriteFile(s.filePath, []byte(`{"hostAlias":{"enable":false}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	on, err := s.HostAliasEnabled()
	if err != nil {
		t.Fatalf("HostAliasEnabled: %v", err)
	}
	if on {
		t.Fatal("expected OFF from explicit {enable:false}")
	}
	if _, err := s.LoadServices(); err != nil {
		t.Fatalf("LoadServices on hostAlias-only config: %v", err)
	}
}

func TestCustomHostAliases(t *testing.T) {
	s := newTestStorage(t)

	if err := os.WriteFile(s.filePath,
		[]byte(`{"services":{},"hostAlias":{"enable":true,"ports":{"1116":["my-db.local","alt.local"]}}}`),
		0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := s.CustomHostAliases()
	if err != nil {
		t.Fatalf("CustomHostAliases: %v", err)
	}
	aliases := got["1116"]
	if len(aliases) != 2 || aliases[0] != "my-db.local" || aliases[1] != "alt.local" {
		t.Errorf("aliases for 1116 = %v, want [my-db.local alt.local]", aliases)
	}
}
