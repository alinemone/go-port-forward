package ui

import (
	"fmt"
	"strings"

	"github.com/alinemone/go-port-forward/internal/model"
)

func (u *UI) ensureCursorInRange() {
	if u.cursorIndex >= len(u.services) && len(u.services) > 0 {
		u.cursorIndex = len(u.services) - 1
	}
	if len(u.services) == 0 {
		u.cursorIndex = 0
	}
}

const minLogViewportHeight = 1

func maxVisibleServices(serviceCount, totalHeight, chromeBelow int) int {
	if totalHeight <= 0 {
		return 8
	}

	// Keep the complete help grid on-screen first. The service table receives
	// whatever remains after the help and the smallest usable bordered log pane.
	cap := totalHeight - chromeBelow - 2 - minLogViewportHeight - 4
	halfHeightCap := totalHeight / 2
	if cap > halfHeightCap {
		cap = halfHeightCap
	}
	if cap < 1 {
		cap = 1
	}
	if cap > 20 {
		cap = 20
	}
	// A truncated table adds its own "more above/below" indicator row.
	if serviceCount > cap && cap > 1 {
		cap--
	}
	return cap
}

func (u *UI) ensureCursorVisible(maxVisible int) {
	if maxVisible <= 0 {
		u.tableOffset = 0
		return
	}
	if u.cursorIndex < u.tableOffset {
		u.tableOffset = u.cursorIndex
	}
	if u.cursorIndex >= u.tableOffset+maxVisible {
		u.tableOffset = u.cursorIndex - maxVisible + 1
	}
	maxOffset := len(u.services) - maxVisible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if u.tableOffset > maxOffset {
		u.tableOffset = maxOffset
	}
	if u.tableOffset < 0 {
		u.tableOffset = 0
	}
}

func (u *UI) refreshViewportContent() {
	if !u.ready {
		return
	}

	u.ensureViewportSize()
	contentWidth := u.viewport.Width() - 4
	if contentWidth < 8 {
		contentWidth = 8
	}

	services := u.services
	if u.logFilterSelected && u.cursorIndex >= 0 && u.cursorIndex < len(u.services) {
		services = []model.Service{u.services[u.cursorIndex]}
	}

	newContent := renderLogsContent(services, contentWidth)
	version := logVersion(services)
	contentChanged := newContent != u.lastLogContent || version != u.lastLogVersion
	u.viewport.SetContent(newContent)
	u.lastLogContent = newContent
	u.lastLogVersion = version
	if contentChanged {
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

func (u *UI) onCursorMoved() {
	if u.logFilterSelected {
		u.refreshViewportContent()
		u.viewport.GotoBottom()
	}
}

func (u *UI) logScopeLabel() string {
	if u.logFilterSelected && u.cursorIndex >= 0 && u.cursorIndex < len(u.services) {
		return truncateRunes(u.services[u.cursorIndex].Name, 14)
	}
	return "ALL"
}

func (u *UI) ensureViewportSize() {
	if u.width == 0 || u.height == 0 {
		return
	}

	viewportHeight := calculateViewportHeight(len(u.services), u.height, u.chromeBelowLog())
	if u.viewport.Height() != viewportHeight {
		u.viewport.SetHeight(viewportHeight)
	}
	if u.viewport.Width() != u.width {
		u.viewport.SetWidth(u.width)
	}
}

// chromeBelowLog measures the responsive help grid, its outer border, and the
// optional status line so narrow layouts can yield height to the full help.
func (u *UI) chromeBelowLog() int {
	h := len(dashboardHelpLines(u.width, u.height, u.logScopeLabel(), u.aliasEnabled)) + 2
	if u.editStatus != "" {
		h++
	}
	return h
}

func calculateViewportHeight(serviceCount, totalHeight, chromeBelow int) int {
	if chromeBelow < 3 {
		chromeBelow = 3
	}
	visible := serviceCount
	maxVis := maxVisibleServices(serviceCount, totalHeight, chromeBelow)
	if visible > maxVis {
		visible = maxVis
	}
	tableLines := 4 + visible
	if serviceCount == 0 {
		tableLines = 1
	}
	if serviceCount > maxVis {
		tableLines++
	}
	overhead := tableLines + 2 + chromeBelow
	viewportHeight := totalHeight - overhead
	if viewportHeight < minLogViewportHeight {
		viewportHeight = minLogViewportHeight
	}
	return viewportHeight
}
