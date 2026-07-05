// Package hostsfile manages a single, clearly delimited block of cluster-host
// aliases inside the system hosts file. Everything outside the managed block is
// preserved byte-for-byte, so the file's other entries are never disturbed.
package hostsfile

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	beginMarker = "# >>> pf managed aliases >>>"
	endMarker   = "# <<< pf managed aliases <<<"
	aliasIP     = "127.0.0.1"
)

// DefaultPath returns the OS hosts-file location.
func DefaultPath() string {
	if runtime.GOOS == "windows" {
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		return filepath.Join(root, "System32", "drivers", "etc", "hosts")
	}
	return "/etc/hosts"
}

// Render returns the hosts-file contents with the pf-managed block replaced by
// fresh `127.0.0.1 <fqdn>` lines for fqdns. Any previous managed block is
// stripped first, so Render is idempotent. When fqdns is empty the block is
// removed entirely. Content outside the markers is preserved; fqdns are sorted
// and de-duplicated so identical inputs always produce identical output. crlf
// selects the line ending used for the rebuilt file.
func Render(existing []byte, fqdns []string, crlf bool) []byte {
	eol := "\n"
	if crlf {
		eol = "\r\n"
	}

	out := make([]string, 0, 16)
	inBlock := false
	for _, ln := range splitLines(existing) {
		switch strings.TrimSpace(ln) {
		case beginMarker:
			inBlock = true
			continue
		case endMarker:
			inBlock = false
			continue
		}
		if inBlock {
			continue
		}
		out = append(out, ln)
	}

	// Drop trailing blank lines so repeated applies don't accumulate them.
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}

	if hosts := dedupeSorted(fqdns); len(hosts) > 0 {
		out = append(out, beginMarker)
		for _, f := range hosts {
			out = append(out, aliasIP+"  "+f)
		}
		out = append(out, endMarker)
	}

	if len(out) == 0 {
		return nil
	}
	return []byte(strings.Join(out, eol) + eol)
}

// Apply writes fqdns as the managed block in the system hosts file, preserving
// everything else. Requires write permission on the hosts file (admin/root).
func Apply(fqdns []string) error {
	path := DefaultPath()

	existing, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		existing = nil
	}

	crlf := runtime.GOOS == "windows" || bytes.Contains(existing, []byte("\r\n"))
	return os.WriteFile(path, Render(existing, fqdns, crlf), 0o644)
}

// Clear removes the pf-managed block from the hosts file, leaving the rest
// untouched.
func Clear() error {
	return Apply(nil)
}

// Writable reports whether the current process can write the hosts file (i.e. it
// is running elevated / as root). It opens the file for append and closes it
// immediately without modifying anything.
func Writable() bool {
	f, err := os.OpenFile(DefaultPath(), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// Verify reports whether the managed block is currently present and contains
// every fqdn. Read-only, so it works without elevation — used to confirm an
// elevated helper actually wrote the entries.
func Verify(fqdns []string) (bool, error) {
	data, err := os.ReadFile(DefaultPath())
	if err != nil {
		return false, err
	}
	s := string(data)
	if !strings.Contains(s, beginMarker) {
		return false, nil
	}
	for _, f := range fqdns {
		if !strings.Contains(s, f) {
			return false, nil
		}
	}
	return true, nil
}

// splitLines splits into lines without their terminators, tolerating both LF
// and CRLF.
func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimSuffix(ln, "\r")
	}
	return lines
}

func dedupeSorted(fqdns []string) []string {
	if len(fqdns) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(fqdns))
	out := make([]string, 0, len(fqdns))
	for _, f := range fqdns {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}
