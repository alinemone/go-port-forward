package version

import "testing"

func TestDefaultValues(t *testing.T) {
	if Version != "dev" {
		t.Errorf("Version = %q, want %q", Version, "dev")
	}
	if Commit != "none" {
		t.Errorf("Commit = %q, want %q", Commit, "none")
	}
	if BuildDate != "unknown" {
		t.Errorf("BuildDate = %q, want %q", BuildDate, "unknown")
	}
}
