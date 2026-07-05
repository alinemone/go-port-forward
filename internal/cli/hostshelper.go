package cli

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alinemone/go-port-forward/internal/hostsfile"
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
//	    keep the managed alias block in sync with <controlFile> (one hostname per
//	    line, rewritten by the unprivileged parent whenever its running set
//	    changes) until <parentPID> exits, then remove the block and the file.
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
	for parentAlive(parentPID) {
		fqdns, raw := readControlFile(controlFile)
		if raw != last {
			if len(fqdns) == 0 {
				_ = hostsfile.Clear()
			} else {
				_ = hostsfile.Apply(fqdns)
			}
			last = raw
		}
		time.Sleep(400 * time.Millisecond)
	}
	_ = hostsfile.Clear()
	os.Remove(controlFile)
	os.Exit(0)
}

// readControlFile returns the validated hostnames plus the raw file content (for
// change detection). Only well-formed hostnames are kept, so a tampered file
// can't inject arbitrary lines.
func readControlFile(path string) (fqdns []string, raw string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ""
	}
	raw = string(data)
	for _, line := range strings.Split(raw, "\n") {
		h := strings.ToLower(strings.TrimSpace(line))
		if h != "" && hostnameRe.MatchString(h) {
			fqdns = append(fqdns, h)
		}
	}
	return fqdns, raw
}
