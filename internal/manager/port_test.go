package manager

import (
	"reflect"
	"testing"
)

func TestParseNetstatListeners(t *testing.T) {
	output := `
Active Connections

  Proto  Local Address          Foreign Address        State           PID
  TCP    0.0.0.0:5432           0.0.0.0:0              LISTENING       1234
  TCP    127.0.0.1:5432         0.0.0.0:0              LISTENING       1234
  TCP    [::1]:5432             [::]:0                 LISTENING       5678
  TCP    127.0.0.1:15432        0.0.0.0:0              LISTENING       9999
  TCP    127.0.0.1:6379         0.0.0.0:0              ESTABLISHED     4321
`

	got := parseNetstatListeners(output, "5432")
	want := []int{1234, 5678}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseNetstatListeners = %v, want %v", got, want)
	}
}

func TestParseNetstatListenersNoMatch(t *testing.T) {
	output := "  TCP    127.0.0.1:8080   0.0.0.0:0   LISTENING   100"
	if got := parseNetstatListeners(output, "9090"); len(got) != 0 {
		t.Errorf("expected no pids, got %v", got)
	}
}

func TestParseLsofPIDs(t *testing.T) {
	got := parseLsofPIDs("1234\n1234\n5678\n")
	want := []int{1234, 5678}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseLsofPIDs = %v, want %v", got, want)
	}

	if got := parseLsofPIDs(""); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}
