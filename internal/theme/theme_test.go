package theme

import "testing"

func TestDefaultActive(t *testing.T) {
	Set("") // reset to default
	if Active.Name != "default" {
		t.Fatalf("default active = %q, want default", Active.Name)
	}
}

func TestSetSwitchesAndValidates(t *testing.T) {
	defer Set("") // restore default for other tests

	if !Set("ocean") {
		t.Fatal("Set(ocean) returned false")
	}
	if Active.Name != "ocean" {
		t.Fatalf("active = %q, want ocean", Active.Name)
	}

	if Set("nope") {
		t.Fatal("Set(unknown) should return false")
	}
	if Active.Name != "ocean" {
		t.Fatalf("active changed on unknown theme: %q", Active.Name)
	}

	if !Set("") || Active.Name != "default" {
		t.Fatalf("empty name should reset to default, got %q", Active.Name)
	}
}

func TestNamesAndExists(t *testing.T) {
	names := Names()
	if len(names) != 3 {
		t.Fatalf("expected 3 themes, got %v", names)
	}
	for _, n := range names {
		if !Exists(n) {
			t.Errorf("Exists(%q) = false", n)
		}
		if _, ok := Get(n); !ok {
			t.Errorf("Get(%q) missing", n)
		}
	}
	if Exists("nope") {
		t.Error("Exists(nope) should be false")
	}
}

func TestNamesIsCopy(t *testing.T) {
	n := Names()
	if len(n) > 0 {
		n[0] = "mutated"
	}
	if Names()[0] == "mutated" {
		t.Fatal("Names() must return a copy, not the backing slice")
	}
}
