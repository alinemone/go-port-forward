package loopback

import "testing"

func TestForNameIsStableAndInPool(t *testing.T) {
	a := ForName("salam-gate-db", nil)
	if a != ForName("salam-gate-db", nil) {
		t.Fatal("ForName not deterministic")
	}
	if !InPool(a) {
		t.Fatalf("%s not in pool", a)
	}
}

func TestForNameSkipsTaken(t *testing.T) {
	first := ForName("svc", nil)
	second := ForName("svc", func(ip string) bool { return ip == first })
	if second == first || !InPool(second) {
		t.Fatalf("expected a different pool IP, got %s (first %s)", second, first)
	}
}

func TestInPool(t *testing.T) {
	for ip, want := range map[string]bool{
		"127.77.0.1":     true,
		"127.77.255.254": true,
		"127.77.3.0":     false,
		"127.77.3.255":   false,
		"127.0.0.1":      false,
		"10.77.0.1":      false,
		"::1":            false,
		"junk":           false,
	} {
		if got := InPool(ip); got != want {
			t.Errorf("InPool(%q) = %v, want %v", ip, got, want)
		}
	}
}
