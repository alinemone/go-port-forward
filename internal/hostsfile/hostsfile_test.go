package hostsfile

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderAddsBlockAndPreservesContent(t *testing.T) {
	existing := []byte("127.0.0.1 localhost\n255.255.255.255 broadcasthost\n")
	out := string(Render(existing, []string{"b.svc.cluster.local", "a.svc.cluster.local"}, false))

	for _, want := range []string{
		"127.0.0.1 localhost",
		"255.255.255.255 broadcasthost",
		beginMarker,
		endMarker,
		aliasIP + "  a.svc.cluster.local",
		aliasIP + "  b.svc.cluster.local",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n---\n%s", want, out)
		}
	}

	// fqdns are sorted: a.* must come before b.*
	if strings.Index(out, "a.svc.cluster.local") > strings.Index(out, "b.svc.cluster.local") {
		t.Error("expected sorted fqdns (a before b)")
	}
	if !strings.HasSuffix(out, "\n") {
		t.Error("expected a trailing newline")
	}
}

func TestRenderIsIdempotent(t *testing.T) {
	existing := []byte("127.0.0.1 localhost\n")
	fqdns := []string{"x.svc.cluster.local", "y.svc.cluster.local"}

	first := Render(existing, fqdns, false)
	second := Render(first, fqdns, false)

	if !bytes.Equal(first, second) {
		t.Errorf("Render not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	// Exactly one managed block, even after re-applying.
	if n := strings.Count(string(second), beginMarker); n != 1 {
		t.Errorf("expected 1 managed block, got %d", n)
	}
}

func TestRenderNilRemovesBlock(t *testing.T) {
	existing := []byte("127.0.0.1 localhost\n")
	withBlock := Render(existing, []string{"z.svc.cluster.local"}, false)

	out := string(Render(withBlock, nil, false))
	if strings.Contains(out, beginMarker) || strings.Contains(out, "z.svc.cluster.local") {
		t.Errorf("expected managed block removed, got:\n%s", out)
	}
	if !strings.Contains(out, "127.0.0.1 localhost") {
		t.Errorf("expected original content preserved, got:\n%s", out)
	}
}

func TestRenderCRLF(t *testing.T) {
	existing := []byte("127.0.0.1 localhost\r\n")
	out := string(Render(existing, []string{"w.svc.cluster.local"}, true))

	if !strings.Contains(out, "\r\n") {
		t.Error("expected CRLF line endings")
	}
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Error("found a bare LF not part of a CRLF")
	}
	if !strings.Contains(out, aliasIP+"  w.svc.cluster.local") {
		t.Errorf("missing alias line, got:\n%q", out)
	}
}
