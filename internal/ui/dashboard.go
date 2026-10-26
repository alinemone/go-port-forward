package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/alinemone/go-port-forward/internal/model"
)

// historyLen is how many throughput samples each service keeps — enough to fill
// the widest chart (two samples per braille cell).
const historyLen = 240

// traffic turns a service's cumulative byte counters into a throughput history
// sampled once per UI tick.
type traffic struct {
	lastBytes int64
	lastAt    time.Time
	rates     []float64 // bytes/s, oldest first
}

func (t *traffic) current() float64 {
	if t == nil || len(t.rates) == 0 {
		return 0
	}
	return t.rates[len(t.rates)-1]
}

func (t *traffic) history() []float64 {
	if t == nil {
		return nil
	}
	return t.rates
}

// sampleTraffic records one throughput sample per service and drops the
// history of services that are no longer running.
func (u *UI) sampleTraffic(now time.Time) {
	if u.traffic == nil {
		u.traffic = make(map[string]*traffic)
	}
	live := make(map[string]bool, len(u.services))
	for i := range u.services {
		svc := &u.services[i]
		live[svc.Name] = true
		total := svc.BytesIn + svc.BytesOut

		t, ok := u.traffic[svc.Name]
		if !ok || total < t.lastBytes { // new, or restarted with fresh counters
			u.traffic[svc.Name] = &traffic{lastBytes: total, lastAt: now}
			continue
		}
		elapsed := now.Sub(t.lastAt).Seconds()
		if elapsed <= 0 {
			continue
		}
		t.rates = append(t.rates, float64(total-t.lastBytes)/elapsed)
		if len(t.rates) > historyLen {
			t.rates = t.rates[len(t.rates)-historyLen:]
		}
		t.lastBytes, t.lastAt = total, now
	}
	for name := range u.traffic {
		if !live[name] {
			delete(u.traffic, name)
		}
	}
}

func (u *UI) ensureCursorInRange() {
	if u.cursorIndex >= len(u.services) && len(u.services) > 0 {
		u.cursorIndex = len(u.services) - 1
	}
	if len(u.services) == 0 {
		u.cursorIndex = 0
	}
}

// dashLayout splits the screen height:
//
//	header
//	forwards box: service rows (+ a scroll hint when they don't all fit)
//	logs box
//	footer
type dashLayout struct {
	rows     int  // service rows shown
	scrolled bool // more services than rows: a scroll hint line is shown
	logs     int  // log viewport height
}

const (
	dashChrome  = 5 // header + two box borders each; the footer (1–2 lines) is added in layout
	minLogLines = 3
)

func (u *UI) layout() dashLayout {
	avail := max(0, u.height-dashChrome-len(u.footerLines(max(1, u.width-2*len(margin)))))
	n := max(1, len(u.services)) // the empty state takes one line

	l := dashLayout{rows: n}
	if room := avail - minLogLines; n > room {
		l.rows = max(1, room-1)
		l.scrolled = true
	}
	used := l.rows
	if l.scrolled {
		used++
	}
	l.logs = max(1, avail-used)
	return l
}

func (u *UI) ensureCursorVisible(visible int) {
	if visible <= 0 {
		u.tableOffset = 0
		return
	}
	if u.cursorIndex < u.tableOffset {
		u.tableOffset = u.cursorIndex
	}
	if u.cursorIndex >= u.tableOffset+visible {
		u.tableOffset = u.cursorIndex - visible + 1
	}
	if maxOffset := max(0, len(u.services)-visible); u.tableOffset > maxOffset {
		u.tableOffset = maxOffset
	}
}

func (u *UI) onCursorMoved() {
	if u.logFilterSelected {
		u.refreshViewportContent()
		u.viewport.GotoBottom()
	}
}

// logScope is the name of the service whose logs are shown, or "" for all.
func (u *UI) logScope() string {
	if u.logFilterSelected && u.cursorIndex >= 0 && u.cursorIndex < len(u.services) {
		return u.services[u.cursorIndex].Name
	}
	return ""
}

// ensureViewportSize fits the log viewport into the log area.
func (u *UI) ensureViewportSize() {
	if u.width == 0 || u.height == 0 {
		return
	}
	w, h := max(1, u.width-4), u.layout().logs
	if u.viewport.Width() != w {
		u.viewport.SetWidth(w)
	}
	if u.viewport.Height() != h {
		u.viewport.SetHeight(h)
	}
}

func (u *UI) refreshViewportContent() {
	if !u.ready {
		return
	}
	u.ensureViewportSize()

	services := u.services
	scope := u.logScope()
	if scope != "" {
		services = []model.Service{u.services[u.cursorIndex]}
	}

	newContent := renderLogsContent(services, u.viewport.Width(), scope == "")
	version := logVersion(services)
	changed := newContent != u.lastLogContent || version != u.lastLogVersion
	u.viewport.SetContent(newContent)
	u.lastLogContent = newContent
	u.lastLogVersion = version
	if changed {
		u.viewport.GotoBottom()
	}
}

// logVersion detects output that renders identically at second precision (for
// example, a burst of repeated lines while the bounded log buffer rotates).
// Nanosecond timestamps ensure that such new output still activates follow mode.
func logVersion(services []model.Service) string {
	var version strings.Builder
	for i := range services {
		logs := services[i].Logs
		fmt.Fprintf(&version, "%s:%d", services[i].Name, len(logs))
		if len(logs) > 0 {
			fmt.Fprintf(&version, ":%d", logs[len(logs)-1].Time.UnixNano())
		}
		version.WriteByte(';')
	}
	return version.String()
}
