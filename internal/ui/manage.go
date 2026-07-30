package ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/alinemone/go-port-forward/internal/icons"
	"github.com/alinemone/go-port-forward/internal/storage"
)

type manageRowKind int

const (
	rowHeaderGroups manageRowKind = iota
	rowHeaderServices
	rowGroup
	rowService
	rowEmptyGroups
	rowEmptyServices
)

type manageRow struct {
	kind manageRowKind
	name string
}

func (r manageRow) selectable() bool {
	return r.kind == rowGroup || r.kind == rowService
}

// overlayIcons holds the resolved icon state for the add/edit overlay: the
// resolver (built-ins + user config overrides), whether icons are drawn, and
// each service's main port. Loaded once when the overlay opens rather than per
// render, and grouped here so the icon feature stays cohesive and easy to grow.
type overlayIcons struct {
	set     *icons.Set
	enabled bool
	ports   map[string]string // service name → main port
}

// manageState is the full state of the unified manage overlay: the combined
// groups+services list, its cursor/scroll position, multi-select marks, the
// live search filter, and any pending confirm/info/error line.
type manageState struct {
	active            bool
	rows              []manageRow
	cursor            int
	offset            int
	groups            map[string][]string
	groupNames        []string
	serviceNames      []string
	icons             overlayIcons // resolved icon state for the overlay list
	selectedGroups    map[string]bool
	selectedServices  map[string]bool
	confirmDeleteName string
	confirmDeleteKind string // "group" | "service"
	errorMsg          string
	infoMsg           string // transient success/info line (e.g. "Started N service(s)")
	searchQuery       string // live filter query for the groups+services list
	showNewPrompt     bool   // ^n → choose group vs service
}

func (u *UI) enterManageMode(focusServices bool) {
	u.manage.active = true
	u.serviceForm.mode = ""
	u.groupForm.mode = ""
	u.manage.errorMsg = ""
	u.manage.infoMsg = ""
	u.manage.searchQuery = ""
	u.manage.showNewPrompt = false
	u.manage.confirmDeleteName = ""
	u.manage.confirmDeleteKind = ""
	u.manage.selectedGroups = make(map[string]bool)
	u.manage.selectedServices = make(map[string]bool)
	u.manage.cursor = 0
	u.manage.offset = 0
	u.reloadManageRowsFromStorage()
	if focusServices {
		u.focusFirstService()
	} else {
		u.focusFirstSelectable()
	}
}

func (u *UI) exitManageMode() {
	u.manage.active = false
	u.serviceForm.mode = ""
	u.groupForm.mode = ""
	u.manage.errorMsg = ""
	u.manage.infoMsg = ""
	u.manage.searchQuery = ""
	u.manage.showNewPrompt = false
	u.manage.confirmDeleteName = ""
	u.manage.confirmDeleteKind = ""
	u.manage.rows = nil
	u.manage.groups = nil
	u.manage.groupNames = nil
	u.manage.serviceNames = nil
	u.manage.icons = overlayIcons{}
	u.manage.selectedGroups = nil
	u.manage.selectedServices = nil
	u.manage.cursor = 0
	u.manage.offset = 0
	u.serviceForm.nameInput.Blur()
	u.serviceForm.commandInput.Blur()
	u.groupForm.nameInput.Blur()
}

// reloadManageRowsFromStorage refreshes the combined groups+services list from storage,
// prunes stale selections, and re-clamps the cursor onto a selectable row.
func (u *UI) reloadManageRowsFromStorage() {
	st := u.store
	groups, err := st.ListGroups()
	if err != nil {
		groups = map[string][]string{}
	}
	groupNames := make([]string, 0, len(groups))
	for name := range groups {
		groupNames = append(groupNames, name)
	}
	sort.Strings(groupNames)

	svcNames, err := st.ListServiceNames()
	if err != nil {
		svcNames = nil
	}

	// Resolve icons for the overlay: the icon set (built-ins + user overrides),
	// whether icons are enabled, and each service's main port. Loaded here, on
	// demand, rather than per-render so list drawing stays allocation-light.
	iconSet, iconsEnabled, err := st.IconSet()
	if err != nil {
		iconSet, iconsEnabled = icons.NewSet(nil, nil), false
	}
	commands, err := st.LoadServices()
	if err != nil {
		commands = nil
	}
	ports := make(map[string]string, len(commands))
	for name, command := range commands {
		if _, main := storage.ParsePortsFromCommand(command); main != "" {
			ports[name] = main
		}
	}

	u.manage.groups = groups
	u.manage.groupNames = groupNames
	u.manage.serviceNames = svcNames
	u.manage.icons = overlayIcons{set: iconSet, enabled: iconsEnabled, ports: ports}

	if u.manage.selectedGroups != nil {
		valid := make(map[string]bool, len(groupNames))
		for _, n := range groupNames {
			valid[n] = true
		}
		for n := range u.manage.selectedGroups {
			if !valid[n] {
				delete(u.manage.selectedGroups, n)
			}
		}
	}
	if u.manage.selectedServices != nil {
		valid := make(map[string]bool, len(svcNames))
		for _, n := range svcNames {
			valid[n] = true
		}
		for n := range u.manage.selectedServices {
			if !valid[n] {
				delete(u.manage.selectedServices, n)
			}
		}
	}

	u.applyManageFilter()
}

// applyManageFilter reconstructs the visible row list from the already-loaded
// group and service names, applying the live search filter. Section headers are
// always shown; a section with no matches shows its empty placeholder. Call this
// (instead of reloadManageRowsFromStorage) when only the filter changed — it avoids a disk
// reload.
func (u *UI) applyManageFilter() {
	q := strings.ToLower(strings.TrimSpace(u.manage.searchQuery))
	match := func(name string) bool {
		return q == "" || strings.Contains(strings.ToLower(name), q)
	}

	rows := make([]manageRow, 0, len(u.manage.groupNames)+len(u.manage.serviceNames)+2)
	rows = append(rows, manageRow{kind: rowHeaderGroups})
	groupMatches := 0
	for _, n := range u.manage.groupNames {
		if match(n) {
			rows = append(rows, manageRow{kind: rowGroup, name: n})
			groupMatches++
		}
	}
	if groupMatches == 0 {
		rows = append(rows, manageRow{kind: rowEmptyGroups})
	}
	rows = append(rows, manageRow{kind: rowHeaderServices})
	svcMatches := 0
	for _, n := range u.manage.serviceNames {
		if match(n) {
			rows = append(rows, manageRow{kind: rowService, name: n})
			svcMatches++
		}
	}
	if svcMatches == 0 {
		rows = append(rows, manageRow{kind: rowEmptyServices})
	}
	u.manage.rows = rows
	u.manage.offset = 0
	u.clampManageCursor()
}

// clampManageCursor snaps the cursor onto the nearest selectable row, searching
// forward first then backward. Used after refresh/delete shifts rows.
func (u *UI) clampManageCursor() {
	n := len(u.manage.rows)
	if n == 0 {
		u.manage.cursor = 0
		return
	}
	if u.manage.cursor < 0 {
		u.manage.cursor = 0
	}
	if u.manage.cursor >= n {
		u.manage.cursor = n - 1
	}
	if u.manage.rows[u.manage.cursor].selectable() {
		return
	}
	for i := u.manage.cursor; i < n; i++ {
		if u.manage.rows[i].selectable() {
			u.manage.cursor = i
			return
		}
	}
	for i := u.manage.cursor; i >= 0; i-- {
		if u.manage.rows[i].selectable() {
			u.manage.cursor = i
			return
		}
	}
}

// moveManageCursor walks in the given direction to the next selectable row,
// skipping headers and placeholders. Stays put if none exist that way.
func (u *UI) moveManageCursor(step int) {
	n := len(u.manage.rows)
	if n == 0 || step == 0 {
		return
	}
	for i := u.manage.cursor + step; i >= 0 && i < n; i += step {
		if u.manage.rows[i].selectable() {
			u.manage.cursor = i
			return
		}
	}
}

func (u *UI) focusFirstSelectable() {
	for i := range u.manage.rows {
		if u.manage.rows[i].selectable() {
			u.manage.cursor = i
			return
		}
	}
}

func (u *UI) focusFirstService() {
	for i := range u.manage.rows {
		if u.manage.rows[i].kind == rowService {
			u.manage.cursor = i
			return
		}
	}
	u.focusFirstSelectable()
}

func (u *UI) focusManage(kind manageRowKind, name string) {
	for i := range u.manage.rows {
		if u.manage.rows[i].kind == kind && u.manage.rows[i].name == name {
			u.manage.cursor = i
			return
		}
	}
	u.clampManageCursor()
}

func (u *UI) currentManageRow() manageRow {
	if u.manage.cursor < 0 || u.manage.cursor >= len(u.manage.rows) {
		return manageRow{}
	}
	return u.manage.rows[u.manage.cursor]
}

func (u *UI) runningNameSet() map[string]bool {
	set := make(map[string]bool, len(u.services))
	for i := range u.services {
		set[u.services[i].Name] = true
	}
	return set
}

func (u *UI) forwardOverlayInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if u.serviceForm.mode != "" {
		return u.updateServiceForm(msg)
	}
	if u.groupForm.mode != "" {
		return u.updateGroupForm(msg)
	}
	return u, nil
}

func (u *UI) updateManageMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if u.serviceForm.mode != "" || u.groupForm.mode != "" {
		return u.forwardOverlayInput(msg)
	}

	keyRaw := msg.String()
	key := normalizeKeyToken(msg)

	if u.manage.confirmDeleteName != "" {
		switch key {
		case "y", "enter":
			name := u.manage.confirmDeleteName
			kind := u.manage.confirmDeleteKind
			u.manage.confirmDeleteName = ""
			u.manage.confirmDeleteKind = ""
			st := u.store
			var err error
			if kind == "group" {
				err = st.DeleteGroup(name)
				delete(u.manage.selectedGroups, name)
			} else {
				err = st.DeleteService(name)
				delete(u.manage.selectedServices, name)
			}
			if err != nil {
				u.manage.errorMsg = fmt.Sprintf("delete failed: %v", err)
				return u, nil
			}
			u.manage.errorMsg = ""
			u.reloadManageRowsFromStorage()
		case "n", "esc":
			u.manage.confirmDeleteName = ""
			u.manage.confirmDeleteKind = ""
		}
		return u, nil
	}

	if u.manage.showNewPrompt {
		switch key {
		case "g":
			u.manage.showNewPrompt = false
			return u, u.openNewGroupForm()
		case "s":
			u.manage.showNewPrompt = false
			return u, u.openNewServiceForm()
		case "n", "esc":
			u.manage.showNewPrompt = false
		}
		return u, nil
	}

	switch key {
	case "esc":
		// First Esc clears an active search; a second one closes the overlay.
		if u.manage.searchQuery != "" {
			u.manage.searchQuery = ""
			u.manage.infoMsg = ""
			u.applyManageFilter()
		} else {
			u.exitManageMode()
		}
	case "up":
		u.moveManageCursor(-1)
	case "down":
		u.moveManageCursor(1)
	case "space":
		u.manage.infoMsg = ""
		row := u.currentManageRow()
		switch row.kind {
		case rowGroup:
			u.manage.selectedGroups[row.name] = !u.manage.selectedGroups[row.name]
		case rowService:
			if !u.runningNameSet()[row.name] {
				u.manage.selectedServices[row.name] = !u.manage.selectedServices[row.name]
			}
		}
	case "ctrl+n":
		u.manage.errorMsg = ""
		u.manage.infoMsg = ""
		u.manage.showNewPrompt = true
	case "ctrl+e":
		row := u.currentManageRow()
		switch row.kind {
		case rowGroup:
			return u, u.openEditGroupFormFor(row.name)
		case rowService:
			return u, u.openEditServiceFormFor(row.name)
		}
	case "ctrl+d":
		row := u.currentManageRow()
		switch row.kind {
		case rowGroup:
			u.manage.errorMsg = ""
			u.manage.confirmDeleteName = row.name
			u.manage.confirmDeleteKind = "group"
		case rowService:
			if u.runningNameSet()[row.name] {
				u.manage.errorMsg = fmt.Sprintf("stop '%s' before deleting", row.name)
			} else {
				u.manage.errorMsg = ""
				u.manage.confirmDeleteName = row.name
				u.manage.confirmDeleteKind = "service"
			}
		}
	case "ctrl+c":
		return u, u.launchEditor()
	case "enter":
		if u.runManageSelection() {
			u.exitManageMode()
		}
	case "backspace":
		if u.manage.searchQuery != "" {
			r := []rune(u.manage.searchQuery)
			u.manage.searchQuery = string(r[:len(r)-1])
			u.manage.infoMsg = ""
			u.applyManageFilter()
		}
	default:
		// Live search: any single printable character typed extends the query and
		// re-filters immediately — no key needed to "enter" search first.
		if rs := []rune(keyRaw); len(rs) == 1 && unicode.IsPrint(rs[0]) {
			u.manage.searchQuery += keyRaw
			u.manage.infoMsg = ""
			u.applyManageFilter()
		}
	}
	return u, nil
}

// runManageSelection starts every non-running service across the selected groups
// and selected loose services (each at most once). Returns true when something was
// selected (caller closes the overlay so the main list shows the run); false when
// nothing was selected (overlay stays open with a hint).
func (u *UI) runManageSelection() bool {
	u.manage.errorMsg = ""
	if len(u.manage.selectedGroups) == 0 && len(u.manage.selectedServices) == 0 {
		u.manage.infoMsg = "Select groups/services with Space first, then Enter to run"
		return false
	}

	running := u.runningNameSet()
	seen := make(map[string]bool)
	start := func(name string) {
		if name == "" || running[name] || seen[name] {
			return
		}
		seen[name] = true
		_ = u.manager.StartStoredService(u.ctx, name)
	}
	for _, g := range u.manage.groupNames {
		if u.manage.selectedGroups[g] {
			for _, svc := range u.manage.groups[g] {
				start(svc)
			}
		}
	}
	for _, s := range u.manage.serviceNames {
		if u.manage.selectedServices[s] {
			start(s)
		}
	}
	u.services = u.manager.ListServiceStates()
	return true
}

func (u *UI) manageVisibleRows(dataHeight int) int {
	if dataHeight <= 0 {
		return 30
	}
	// Border, title, and search consume four lines. If the list is truncated,
	// reserve one more line for its range indicator.
	v := dataHeight - 4
	if v < 1 {
		v = 1
	}
	if len(u.manage.rows) > v && v > 1 {
		v--
	}
	return v
}

func (u *UI) ensureManageVisible(visible int) {
	if len(u.manage.rows) <= visible {
		u.manage.offset = 0
		return
	}
	if u.manage.cursor < u.manage.offset {
		u.manage.offset = u.manage.cursor
	}
	if u.manage.cursor >= u.manage.offset+visible {
		u.manage.offset = u.manage.cursor - visible + 1
	}
	if maxOff := len(u.manage.rows) - visible; u.manage.offset > maxOff {
		u.manage.offset = maxOff
	}
	if u.manage.offset < 0 {
		u.manage.offset = 0
	}
}
