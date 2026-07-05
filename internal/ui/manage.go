package ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/alinemone/go-port-forward/internal/icons"
	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/stringutil"
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

func (u *UI) enterManageMode(focusServices bool) {
	u.manageMode = true
	u.addFormMode = ""
	u.groupFormMode = ""
	u.manageErr = ""
	u.manageInfo = ""
	u.manageSearch = ""
	u.manageNewPrompt = false
	u.manageConfirmDelete = ""
	u.manageConfirmKind = ""
	u.manageSelGroups = make(map[string]bool)
	u.manageSelSvcs = make(map[string]bool)
	u.manageCursor = 0
	u.manageOffset = 0
	u.buildManageRows()
	if focusServices {
		u.focusFirstService()
	} else {
		u.focusFirstSelectable()
	}
}

func (u *UI) exitManageMode() {
	u.manageMode = false
	u.addFormMode = ""
	u.groupFormMode = ""
	u.manageErr = ""
	u.manageInfo = ""
	u.manageSearch = ""
	u.manageNewPrompt = false
	u.manageConfirmDelete = ""
	u.manageConfirmKind = ""
	u.manageRows = nil
	u.manageGroups = nil
	u.manageGroupNames = nil
	u.manageServices = nil
	u.manageIcons = overlayIcons{}
	u.manageSelGroups = nil
	u.manageSelSvcs = nil
	u.manageCursor = 0
	u.manageOffset = 0
	u.addFormName.Blur()
	u.addFormCmd.Blur()
	u.groupFormName.Blur()
}

// buildManageRows refreshes the combined groups+services list from storage,
// prunes stale selections, and re-clamps the cursor onto a selectable row.
func (u *UI) buildManageRows() {
	st := storage.NewStorage()
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

	u.manageGroups = groups
	u.manageGroupNames = groupNames
	u.manageServices = svcNames
	u.manageIcons = overlayIcons{set: iconSet, enabled: iconsEnabled, ports: ports}

	if u.manageSelGroups != nil {
		valid := make(map[string]bool, len(groupNames))
		for _, n := range groupNames {
			valid[n] = true
		}
		for n := range u.manageSelGroups {
			if !valid[n] {
				delete(u.manageSelGroups, n)
			}
		}
	}
	if u.manageSelSvcs != nil {
		valid := make(map[string]bool, len(svcNames))
		for _, n := range svcNames {
			valid[n] = true
		}
		for n := range u.manageSelSvcs {
			if !valid[n] {
				delete(u.manageSelSvcs, n)
			}
		}
	}

	u.rebuildManageRows()
}

// rebuildManageRows reconstructs the visible row list from the already-loaded
// group and service names, applying the live search filter. Section headers are
// always shown; a section with no matches shows its empty placeholder. Call this
// (instead of buildManageRows) when only the filter changed — it avoids a disk
// reload.
func (u *UI) rebuildManageRows() {
	q := strings.ToLower(strings.TrimSpace(u.manageSearch))
	match := func(name string) bool {
		return q == "" || strings.Contains(strings.ToLower(name), q)
	}

	rows := make([]manageRow, 0, len(u.manageGroupNames)+len(u.manageServices)+2)
	rows = append(rows, manageRow{kind: rowHeaderGroups})
	groupMatches := 0
	for _, n := range u.manageGroupNames {
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
	for _, n := range u.manageServices {
		if match(n) {
			rows = append(rows, manageRow{kind: rowService, name: n})
			svcMatches++
		}
	}
	if svcMatches == 0 {
		rows = append(rows, manageRow{kind: rowEmptyServices})
	}
	u.manageRows = rows
	u.manageOffset = 0
	u.clampManageCursor()
}

// clampManageCursor snaps the cursor onto the nearest selectable row, searching
// forward first then backward. Used after refresh/delete shifts rows.
func (u *UI) clampManageCursor() {
	n := len(u.manageRows)
	if n == 0 {
		u.manageCursor = 0
		return
	}
	if u.manageCursor < 0 {
		u.manageCursor = 0
	}
	if u.manageCursor >= n {
		u.manageCursor = n - 1
	}
	if u.manageRows[u.manageCursor].selectable() {
		return
	}
	for i := u.manageCursor; i < n; i++ {
		if u.manageRows[i].selectable() {
			u.manageCursor = i
			return
		}
	}
	for i := u.manageCursor; i >= 0; i-- {
		if u.manageRows[i].selectable() {
			u.manageCursor = i
			return
		}
	}
}

// moveManageCursor walks in the given direction to the next selectable row,
// skipping headers and placeholders. Stays put if none exist that way.
func (u *UI) moveManageCursor(step int) {
	n := len(u.manageRows)
	if n == 0 || step == 0 {
		return
	}
	for i := u.manageCursor + step; i >= 0 && i < n; i += step {
		if u.manageRows[i].selectable() {
			u.manageCursor = i
			return
		}
	}
}

func (u *UI) focusFirstSelectable() {
	for i := range u.manageRows {
		if u.manageRows[i].selectable() {
			u.manageCursor = i
			return
		}
	}
}

func (u *UI) focusFirstService() {
	for i := range u.manageRows {
		if u.manageRows[i].kind == rowService {
			u.manageCursor = i
			return
		}
	}
	u.focusFirstSelectable()
}

func (u *UI) focusManage(kind manageRowKind, name string) {
	for i := range u.manageRows {
		if u.manageRows[i].kind == kind && u.manageRows[i].name == name {
			u.manageCursor = i
			return
		}
	}
	u.clampManageCursor()
}

func (u *UI) currentManageRow() manageRow {
	if u.manageCursor < 0 || u.manageCursor >= len(u.manageRows) {
		return manageRow{}
	}
	return u.manageRows[u.manageCursor]
}

func (u *UI) runningNameSet() map[string]bool {
	set := make(map[string]bool, len(u.services))
	for i := range u.services {
		set[u.services[i].Name] = true
	}
	return set
}

func (u *UI) updateManageInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if u.addFormMode != "" {
		return u.updateAddForm(msg)
	}
	if u.groupFormMode != "" {
		return u.updateGroupForm(msg)
	}
	return u, nil
}

func (u *UI) updateManageMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if u.addFormMode != "" || u.groupFormMode != "" {
		return u.updateManageInput(msg)
	}

	keyRaw := msg.String()
	key := keyRaw
	if keyRaw != "space" {
		key = stringutil.NormalizeToken(keyRaw)
	}

	if u.manageConfirmDelete != "" {
		switch key {
		case "y", "enter":
			name := u.manageConfirmDelete
			kind := u.manageConfirmKind
			u.manageConfirmDelete = ""
			u.manageConfirmKind = ""
			st := storage.NewStorage()
			var err error
			if kind == "group" {
				err = st.DeleteGroup(name)
				delete(u.manageSelGroups, name)
			} else {
				err = st.DeleteService(name)
				delete(u.manageSelSvcs, name)
			}
			if err != nil {
				u.manageErr = fmt.Sprintf("delete failed: %v", err)
				return u, nil
			}
			u.manageErr = ""
			u.buildManageRows()
		case "n", "esc":
			u.manageConfirmDelete = ""
			u.manageConfirmKind = ""
		}
		return u, nil
	}

	if u.manageNewPrompt {
		switch key {
		case "g":
			u.manageNewPrompt = false
			return u, u.openNewGroupForm()
		case "s":
			u.manageNewPrompt = false
			return u, u.openNewServiceForm()
		case "n", "esc":
			u.manageNewPrompt = false
		}
		return u, nil
	}

	switch key {
	case "esc":
		// First Esc clears an active search; a second one closes the overlay.
		if u.manageSearch != "" {
			u.manageSearch = ""
			u.manageInfo = ""
			u.rebuildManageRows()
		} else {
			u.exitManageMode()
		}
	case "up":
		u.moveManageCursor(-1)
	case "down":
		u.moveManageCursor(1)
	case "space":
		u.manageInfo = ""
		row := u.currentManageRow()
		switch row.kind {
		case rowGroup:
			u.manageSelGroups[row.name] = !u.manageSelGroups[row.name]
		case rowService:
			if !u.runningNameSet()[row.name] {
				u.manageSelSvcs[row.name] = !u.manageSelSvcs[row.name]
			}
		}
	case "ctrl+n":
		u.manageErr = ""
		u.manageInfo = ""
		u.manageNewPrompt = true
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
			u.manageErr = ""
			u.manageConfirmDelete = row.name
			u.manageConfirmKind = "group"
		case rowService:
			if u.runningNameSet()[row.name] {
				u.manageErr = fmt.Sprintf("stop '%s' before deleting", row.name)
			} else {
				u.manageErr = ""
				u.manageConfirmDelete = row.name
				u.manageConfirmKind = "service"
			}
		}
	case "ctrl+c":
		return u, u.launchEditor()
	case "enter":
		if u.runManageSelection() {
			u.exitManageMode()
		}
	case "backspace":
		if u.manageSearch != "" {
			r := []rune(u.manageSearch)
			u.manageSearch = string(r[:len(r)-1])
			u.manageInfo = ""
			u.rebuildManageRows()
		}
	default:
		// Live search: any single printable character typed extends the query and
		// re-filters immediately — no key needed to "enter" search first.
		if rs := []rune(keyRaw); len(rs) == 1 && unicode.IsPrint(rs[0]) {
			u.manageSearch += keyRaw
			u.manageInfo = ""
			u.rebuildManageRows()
		}
	}
	return u, nil
}

// runManageSelection starts every non-running service across the selected groups
// and selected loose services (each at most once). Returns true when something was
// selected (caller closes the overlay so the main list shows the run); false when
// nothing was selected (overlay stays open with a hint).
func (u *UI) runManageSelection() bool {
	u.manageErr = ""
	if len(u.manageSelGroups) == 0 && len(u.manageSelSvcs) == 0 {
		u.manageInfo = "Select groups/services with Space first, then Enter to run"
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
	for _, g := range u.manageGroupNames {
		if u.manageSelGroups[g] {
			for _, svc := range u.manageGroups[g] {
				start(svc)
			}
		}
	}
	for _, s := range u.manageServices {
		if u.manageSelSvcs[s] {
			start(s)
		}
	}
	u.services = u.manager.ListServiceStates()
	return true
}

func (u *UI) manageVisibleRows() int {
	if u.height <= 0 {
		return 30
	}
	v := u.height - 9 // chrome: title + search line + box border + action chips
	if v < 5 {
		v = 5
	}
	if v > 30 {
		v = 30
	}
	return v
}

func (u *UI) ensureManageVisible() {
	visible := u.manageVisibleRows()
	if len(u.manageRows) <= visible {
		u.manageOffset = 0
		return
	}
	if u.manageCursor < u.manageOffset {
		u.manageOffset = u.manageCursor
	}
	if u.manageCursor >= u.manageOffset+visible {
		u.manageOffset = u.manageCursor - visible + 1
	}
	if maxOff := len(u.manageRows) - visible; u.manageOffset > maxOff {
		u.manageOffset = maxOff
	}
	if u.manageOffset < 0 {
		u.manageOffset = 0
	}
}
