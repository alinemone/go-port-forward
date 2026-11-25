package cli

import (
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alinemone/go-port-forward/internal/hostsfile"
	"github.com/alinemone/go-port-forward/internal/loopback"
)

// hostnameRe guards what the elevated helper is willing to write into the hosts
// file — a conservative DNS-ish charset — so a tampered temp file can't inject
// arbitrary lines.
var hostnameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

// runHostsHelper implements the hidden `pf __hosts` command. It is an internal
// worker (not shown in help) that the unprivileged process launches elevated to
// edit the protected system hosts file on its behalf.
//
//	pf __hosts watch <parentPID> <controlFile>
//	    keep the managed alias block in sync with <controlFile> (one `IP [host]`
//	    per line, rewritten by the unprivileged parent whenever its running set
//	    changes) until <parentPID> exits, then remove the block and the file.
//	    A line without a host only asks for the loopback IP to exist (macOS lo0
//	    alias) for a forward with no cluster hostname.
//	    One elevation covers the whole session — adds/removes need no new prompt.
//	pf __hosts clear
//	    remove the managed alias block (used by `pf alias clear`).
func runHostsHelper(args []string) {
	if len(args) == 0 {
		os.Exit(2)
	}

	switch args[0] {
	case "watch":
		if len(args) < 3 {
			os.Exit(2)
		}
		pid, err := strconv.Atoi(args[1])
		if err != nil {
			os.Exit(2)
		}
		watchHostsControl(pid, args[2])

	case "clear":
		if err := hostsfile.Clear(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)

	default:
		os.Exit(2)
	}
}

// watchHostsControl reconciles the hosts block with the control file until the
// parent process exits, then cleans up. It re-applies only when the file's
// content actually changes, so it is cheap to poll.
func watchHostsControl(parentPID int, controlFile string) {
	last := "\x00" // sentinel that never equals real content, forcing a first apply
	var ips []string
	for parentAlive(parentPID) {
		data, err := os.ReadFile(controlFile)
		raw := string(data)
		if err != nil {
			raw = ""
		}
		if raw != last {
			entries, wantIPs := parseAliasControl(raw)
			ips = syncAliases(entries, wantIPs, ips)
			last = raw
		}
		time.Sleep(400 * time.Millisecond)
	}
	syncAliases(nil, nil, ips)
	os.Remove(controlFile)
	os.Exit(0)
}

// syncAliases makes the hosts block equal entries and the set of dedicated
// loopback IPs equal want (adding/removing macOS lo0 aliases; a no-op
// elsewhere). prev is the IP set applied last time; the new set is returned.
// Needs admin/root.
func syncAliases(entries []hostsfile.Entry, want, prev []string) []string {
	keep := make(map[string]bool, len(want))
	for _, ip := range want {
		keep[ip] = true
	}
	var stale []string
	for _, ip := range prev {
		if !keep[ip] {
			stale = append(stale, ip)
		}
	}
	loopback.Unconfigure(stale)
	loopback.Configure(want) // re-adding an existing alias is harmless

	if len(entries) == 0 {
		_ = hostsfile.Clear()
	} else {
		_ = hostsfile.Apply(entries)
	}
	return want
}

// formatAliasControl serialises the desired state as `IP [host]` lines, sorted
// so identical state always yields identical content.
func formatAliasControl(entries []hostsfile.Entry, ips []string) string {
	lines := make([]string, 0, len(entries)+len(ips))
	for _, e := range entries {
		lines = append(lines, e.IP+" "+e.Host)
	}
	lines = append(lines, ips...)
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// parseAliasControl reads `IP [host]` lines back. Only well-formed hostnames,
// 127.0.0.0/8 IPs for hosts-file lines and pool IPs for lo0 aliases are kept,
// so a tampered file can't inject arbitrary hosts lines or touch other
// interfaces' addresses.
func parseAliasControl(raw string) (entries []hostsfile.Entry, ips []string) {
	seenIP := make(map[string]bool)
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(strings.ToLower(line))
		if len(fields) == 0 || len(fields) > 2 {
			continue
		}
		ip := net.ParseIP(fields[0]).To4()
		if ip == nil || ip[0] != 127 {
			continue
		}
		addr := ip.String()
		if loopback.InPool(addr) && !seenIP[addr] {
			seenIP[addr] = true
			ips = append(ips, addr)
		}
		if len(fields) == 2 && hostnameRe.MatchString(fields[1]) {
			entries = append(entries, hostsfile.Entry{IP: addr, Host: fields[1]})
		}
	}
	return entries, ips
}
