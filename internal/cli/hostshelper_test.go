package cli

import (
	"reflect"
	"testing"

	"github.com/alinemone/go-port-forward/internal/hostsfile"
)

func TestAliasControlRoundTrip(t *testing.T) {
	entries := []hostsfile.Entry{
		{IP: "127.77.0.2", Host: "prod-salam-gate-psql-postgresql-ha.prod-salam-gate.svc.cluster.local"},
		{IP: "127.0.0.1", Host: "redis.prod.svc.cluster.local"},
	}
	ips := []string{"127.77.0.2", "127.77.4.9"} // 127.77.4.9: dedicated IP with no hostname

	gotEntries, gotIPs := parseAliasControl(formatAliasControl(entries, ips))

	wantEntries := []hostsfile.Entry{
		{IP: "127.0.0.1", Host: "redis.prod.svc.cluster.local"},
		{IP: "127.77.0.2", Host: "prod-salam-gate-psql-postgresql-ha.prod-salam-gate.svc.cluster.local"},
	}
	if !reflect.DeepEqual(gotEntries, wantEntries) {
		t.Errorf("entries = %v, want %v", gotEntries, wantEntries)
	}
	if !reflect.DeepEqual(gotIPs, []string{"127.77.0.2", "127.77.4.9"}) {
		t.Errorf("ips = %v", gotIPs)
	}
}

func TestParseAliasControlRejectsTampering(t *testing.T) {
	raw := "10.0.0.1 evil.example\n" + // non-loopback IP
		"127.0.0.1 bad host extra\n" + // too many fields
		"127.0.0.1 UPPER_score!\n" + // invalid hostname
		"8.8.8.8\n" // non-pool IP-only line
	entries, ips := parseAliasControl(raw)
	if len(entries) != 0 || len(ips) != 0 {
		t.Fatalf("expected nothing accepted, got %v / %v", entries, ips)
	}
}
