package ui

import (
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

func maxVisibleServices(totalHeight int) int {
	if totalHeight <= 0 {
		return 8
	}
	cap := totalHeight / 2
	if cap < 3 {
		cap = 3
	}
	if cap > 20 {
		cap = 20
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
	if contentWidth < 40 {
		contentWidth = 40
	}

	services := u.services
	if u.logFilterSelected && u.cursorIndex >= 0 && u.cursorIndex < len(u.services) {
		services = []model.Service{u.services[u.cursorIndex]}
	}

	follow := u.viewport.AtBottom()
	newContent := renderLogsContent(services, contentWidth)
	u.viewport.SetContent(newContent)
	if follow {
		u.viewport.GotoBottom()
	}
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

// chromeBelowLog returns the number of lines occupied below the log box: the
// help bar (content rows + its border) plus the optional status line. The help
// bar can wrap to multiple rows on narrow terminals, so this must be measured,
// not assumed, or the bottom border gets clipped off-screen.
func (u *UI) chromeBelowLog() int {
	h := len(helpLines(u.width, u.logScopeLabel(), u.aliasEnabled)) + 2 // help box border
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
	maxVis := maxVisibleServices(totalHeight)
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
	if viewportHeight < 3 {
		viewportHeight = 3
	}
	return viewportHeight
}
