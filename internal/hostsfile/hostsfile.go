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
	// LocalhostIP is the address used for services bound on localhost.
	LocalhostIP = "127.0.0.1"
	aliasIP     = LocalhostIP
)

// Entry is one `IP hostname` line in the managed block. Services forwarded on
// localhost use LocalhostIP; services moved to a dedicated loopback IP (because
// their local port is shared) use that IP, so each hostname reaches its own
// forward even when several listen on the same port.
type Entry struct {
	IP   string
	Host string
}

// Localhost maps every hostname to 127.0.0.1.
func Localhost(hosts []string) []Entry {
	out := make([]Entry, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, Entry{IP: LocalhostIP, Host: h})
	}
	return out
}

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
// fresh `<ip> <host>` lines for entries. Any previous managed block is stripped
// first, so Render is idempotent. When entries is empty the block is removed
// entirely. Content outside the markers is preserved; entries are sorted by
// host and de-duplicated by host (first wins) so identical inputs always
// produce identical output. crlf selects the line ending used for the rebuilt
// file.
func Render(existing []byte, entries []Entry, crlf bool) []byte {
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

	if lines := dedupeSorted(entries); len(lines) > 0 {
		out = append(out, beginMarker)
		for _, e := range lines {
			out = append(out, e.IP+"  "+e.Host)
		}
		out = append(out, endMarker)
	}

	if len(out) == 0 {
		return nil
	}
	return []byte(strings.Join(out, eol) + eol)
}

// Apply writes entries as the managed block in the system hosts file,
// preserving everything else. Requires write permission on the hosts file
// (admin/root).
func Apply(entries []Entry) error {
	path := DefaultPath()

	existing, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		existing = nil
	}

	crlf := runtime.GOOS == "windows" || bytes.Contains(existing, []byte("\r\n"))
	return os.WriteFile(path, Render(existing, entries, crlf), 0o644)
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

// List returns the entries currently in the pf-managed block of the system
// hosts file. Read-only, so it needs no elevation.
func List() ([]Entry, error) {
	data, err := os.ReadFile(DefaultPath())
	if err != nil {
		return nil, err
	}
	return parseBlock(data), nil
}

// parseBlock extracts the `IP host` lines between the managed markers.
func parseBlock(data []byte) []Entry {
	var out []Entry
	inBlock := false
	for _, ln := range splitLines(data) {
		switch t := strings.TrimSpace(ln); {
		case t == beginMarker:
			inBlock = true
		case t == endMarker:
			inBlock = false
		case inBlock:
			if f := strings.Fields(t); len(f) == 2 {
				out = append(out, Entry{IP: f[0], Host: f[1]})
			}
		}
	}
	return out
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

func dedupeSorted(entries []Entry) []Entry {
	if len(entries) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(entries))
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		e.Host = strings.TrimSpace(e.Host)
		e.IP = strings.TrimSpace(e.IP)
		if e.Host == "" {
			continue
		}
		if e.IP == "" {
			e.IP = LocalhostIP
		}
		if _, ok := seen[e.Host]; ok {
			continue
		}
		seen[e.Host] = struct{}{}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out
}
