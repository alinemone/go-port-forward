package kube

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func svc(name, clusterIP string, ports ...int32) *corev1.Service {
	s := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       corev1.ServiceSpec{ClusterIP: clusterIP, Selector: map[string]string{"app": name}},
	}
	for _, p := range ports {
		s.Spec.Ports = append(s.Spec.Ports, corev1.ServicePort{Port: p})
	}
	return s
}

// Shapes taken from a real namespace (prod-salam-gate and friends).
func TestClassify(t *testing.T) {
	for _, tc := range []struct {
		svc  *corev1.Service
		kind string
		port int
		tier Tier
		note string
	}{
		{svc("prod-salam-gate-psql-postgresql-ha", "10.0.0.1", 5432, 8080), "postgres", 5432, TierSuggested, ""},
		{svc("prod-salam-gate-psql-postgresql-ha-slave", "10.0.0.2", 5432, 8080), "postgres", 5432, TierMore, "replica"},
		{svc("prod-salam-gate-psql-postgresql-ha-keeper", "None", 5432, 8080, 9187), "postgres", 5432, TierMore, "headless"},
		{svc("prod-salam-gate-redis-haproxy", "10.0.0.3", 6379, 9101), "redis", 6379, TierSuggested, ""},
		{svc("prod-salam-gate-redis-announce-0", "10.0.0.4", 6379, 26379, 9121), "redis", 6379, TierMore, "per-pod"},
		{svc("prod-salam-gate-redis", "None", 6379, 26379, 9121), "redis", 6379, TierMore, "headless"},
		{svc("prod-kafka-cdc-kafka-bootstrap", "10.0.0.5", 9091, 9092), "kafka", 9092, TierSuggested, ""},
		{svc("prod-kafka-cdc-zookeeper-client", "10.0.0.6", 2181), "zookeeper", 2181, TierMore, ""},
		{svc("clickhouse-adwiser", "10.0.0.7", 8123, 9000), "clickhouse", 8123, TierSuggested, ""},
		{svc("minio", "10.0.0.8", 9000), "", 9000, TierOther, ""},
		{svc("hermes-pgbouncer", "10.0.0.10", 5432), "pgbouncer", 5432, TierSuggested, ""},
		{svc("prod-salam-gate", "10.0.0.9", 80), "", 80, TierOther, ""},
	} {
		got := classify(tc.svc)
		if got.Kind != tc.kind || got.Port != tc.port || got.Tier != tc.tier || got.Note != tc.note {
			t.Errorf("%s: got kind=%q port=%d tier=%d note=%q, want kind=%q port=%d tier=%d note=%q",
				tc.svc.Name, got.Kind, got.Port, got.Tier, got.Note, tc.kind, tc.port, tc.tier, tc.note)
		}
	}
}

func TestBuildCommandRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		ctx, current string
		wantCtx      string
	}{
		{"production", "production", ""},
		{"", "production", ""},
		{"stage", "production", "stage"},
	} {
		cmd := BuildCommand(tc.ctx, tc.current, "prod-salam-gate", "prod-salam-gate-redis-haproxy", 16379, 6379)
		spec, err := ParseCommand(cmd)
		if err != nil {
			t.Fatalf("%q does not parse: %v", cmd, err)
		}
		if spec.Context != tc.wantCtx || spec.Namespace != "prod-salam-gate" || spec.Name != "prod-salam-gate-redis-haproxy" ||
			spec.Ports[0] != (PortMapping{Local: 16379, Remote: "6379"}) {
			t.Errorf("%q parsed to %+v", cmd, spec)
		}
	}
}
