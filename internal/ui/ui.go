package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/configedit"
	"github.com/alinemone/go-port-forward/internal/icons"
	"github.com/alinemone/go-port-forward/internal/manager"
	"github.com/alinemone/go-port-forward/internal/model"
	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/stringutil"
	"github.com/alinemone/go-port-forward/internal/theme"
)

type tickMsg time.Time

type spinnerTickMsg time.Time

type shutdownDoneMsg struct{}

type clearStatusMsg struct{ seq int }

const statusClearDelay = 5 * time.Second

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type editResultMsg struct {
	ok       bool
	err      error
	services int
	groups   int
	tmpPath  string
}

// Chrome colors come from the active palette (internal/theme). They are filled
// by ApplyTheme so the user's theme choice takes effect process-wide; the TUI
// builds its styles per-render from these vars, so no caching gets stale.
var (
	colorText      color.Color
	colorMuted     color.Color
	colorBorder    color.Color
	colorAccent    color.Color
	colorAccentAlt color.Color
	colorWarn      color.Color
	colorError     color.Color
	colorHeading   color.Color
	colorSelected  color.Color
)

// Service-health colors are fixed (never themed) so HEALTHY always reads green,
// CONNECTING yellow, and ERROR red regardless of the active palette.
var (
	statusHealthyColor    = lipgloss.Color(theme.StatusHealthy)
	statusConnectingColor = lipgloss.Color(theme.StatusConnecting)
	statusErrorColor      = lipgloss.Color(theme.StatusError)
)

func init() { ApplyTheme() }

// ApplyTheme refreshes the package-level chrome colors from theme.Active. Call
// it once at startup after selecting the theme (init seeds the default).
func ApplyTheme() {
	p := theme.Active
	colorText = lipgloss.Color(p.Text)
	colorMuted = lipgloss.Color(p.Muted)
	colorBorder = lipgloss.Color(p.Border)
	colorAccent = lipgloss.Color(p.Accent)
	colorAccentAlt = lipgloss.Color(p.AccentAlt)
	colorWarn = lipgloss.Color(p.Warn)
	colorError = lipgloss.Color(p.Error)
	colorHeading = lipgloss.Color(p.Heading)
	colorSelected = lipgloss.Color(p.Selected)
}

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

type Controller interface {
	ListServiceStates() []model.Service
	StartStoredService(ctx context.Context, name string) error
	StopService(name string)
	StopAllServices()
	RestartService(ctx context.Context, name string) error
	RestartAllServices(ctx context.Context)
}

type UI struct {
	manager     Controller
	services    []model.Service
	cursorIndex int
	quitting    bool
	width       int
	height      int
	viewport    viewport.Model
	ready       bool
	ctx         context.Context
	// service form (new/edit) — shared, launched from the manage overlay
	addFormMode  string
	addFormName  textinput.Model
	addFormCmd   textinput.Model
	addFormFocus int // 0 = name, 1 = command
	addFormOrig  string
	addFormErr   string
	// group form (new/edit) — shared, launched from the manage overlay
	groupFormMode      string // "" = list, "new", "edit"
	groupFormOrig      string
	groupFormName      textinput.Model
	groupFormErr       string
	groupFormFocus     int // 0 = name, 1 = services list
	groupFormServices  []string
	groupFormSelected  map[string]bool
	groupFormSvcCursor int
	// unified manage overlay (groups + services in one list)
	manageMode          bool
	manageRows          []manageRow
	manageCursor        int
	manageOffset        int
	manageGroups        map[string][]string
	manageGroupNames    []string
	manageServices      []string
	manageIcons         overlayIcons // resolved icon state for the overlay list
	manageSelGroups     map[string]bool
	manageSelSvcs       map[string]bool
	manageConfirmDelete string
	manageConfirmKind   string // "group" | "service"
	manageErr           string
	manageInfo          string // transient success/info line (e.g. "Started N service(s)")
	manageSearch        string // live filter query for the groups+services list
	manageNewPrompt     bool   // "n" → choose group vs service
	editStatus          string
	editStatusSeq       int
	logFilterSelected   bool
	spinnerFrame        int
	tableOffset         int
}

const uiTickInterval = 500 * time.Millisecond

func NewUI(mgr Controller, ctx context.Context) *UI {
	return &UI{
		manager:  mgr,
		services: []model.Service{},
		ctx:      ctx,
	}
}

func (u *UI) Init() tea.Cmd {
	return tickCmd(uiTickInterval)
}

func (u *UI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		u.width = msg.Width
		u.height = msg.Height

		viewportHeight := calculateViewportHeight(len(u.services), u.height, u.chromeBelowLog())
		if !u.ready {
			u.viewport = viewport.New(viewport.WithWidth(msg.Width), viewport.WithHeight(viewportHeight))
			u.viewport.YPosition = 0
			u.ready = true
		} else {
			u.viewport.SetWidth(msg.Width)
			u.viewport.SetHeight(viewportHeight)
		}

		if u.manageMode && u.addFormMode != "" {
			inputWidth := u.formInputWidth()
			u.addFormName.SetWidth(inputWidth)
			u.addFormCmd.SetWidth(inputWidth)
		}
		if u.manageMode && u.groupFormMode != "" {
			u.groupFormName.SetWidth(u.formInputWidth())
		}

	case tea.MouseWheelMsg:
		switch {
		case u.manageMode:
			if u.addFormMode == "" && u.groupFormMode == "" {
				switch msg.Button {
				case tea.MouseWheelUp:
					u.moveManageCursor(-1)
				case tea.MouseWheelDown:
					u.moveManageCursor(1)
				}
			}
		default:
			u.viewport, cmd = u.viewport.Update(msg)
		}

	case tea.KeyPressMsg:
		if u.quitting {
			return u, nil
		}
		keyRaw := msg.String()
		key := keyRaw
		if keyRaw != "space" {
			key = stringutil.NormalizeToken(keyRaw)
		}
		if u.manageMode {
			return u.updateManageMode(msg)
		}

		switch key {
		case "q", "ctrl+c", "esc":
			u.quitting = true
			return u, tea.Batch(u.shutdownCmd(), spinnerTick())

		case "up", "k":
			if u.cursorIndex > 0 {
				u.cursorIndex--
				u.onCursorMoved()
			} else {
				u.viewport, cmd = u.viewport.Update(msg)
			}

		case "down", "j":
			if u.cursorIndex < len(u.services)-1 {
				u.cursorIndex++
				u.onCursorMoved()
			} else {
				u.viewport, cmd = u.viewport.Update(msg)
			}

		case "pgup", "pgdown", "home", "end", "ctrl+u", "ctrl+d":
			u.viewport, cmd = u.viewport.Update(msg)

		case "r":
			if u.cursorIndex < len(u.services) && len(u.services) > 0 {
				serviceName := u.services[u.cursorIndex].Name
				u.manager.RestartService(u.ctx, serviceName)
			}

		case "ctrl+r":
			if len(u.services) > 0 {
				u.manager.RestartAllServices(u.ctx)
			}

		case "s":
			if u.cursorIndex < len(u.services) && len(u.services) > 0 {
				name := u.services[u.cursorIndex].Name
				return u, func() tea.Msg {
					u.manager.StopService(name)
					return nil
				}
			}

		case "a":
			u.enterManageMode(true)

		case "g":
			u.enterManageMode(false)

		case "c":
			return u, u.launchEditor()

		case "l":
			u.logFilterSelected = !u.logFilterSelected
			u.refreshViewportContent()
			u.viewport.GotoBottom()

		default:
			u.viewport, cmd = u.viewport.Update(msg)
		}

	case editResultMsg:
		var status string
		switch {
		case msg.ok:
			status = fmt.Sprintf("✓ Config saved: %d service(s), %d group(s) — affects future runs", msg.services, msg.groups)
			if u.manageMode && u.addFormMode == "" && u.groupFormMode == "" {
				u.buildManageRows()
			}
		case msg.tmpPath != "":
			status = fmt.Sprintf("✗ Invalid config: %v — edits kept at %s (use 'pf edit' to fix)", msg.err, msg.tmpPath)
		case msg.err != nil:
			status = fmt.Sprintf("✗ Edit failed: %v", msg.err)
		}
		if status == "" {
			return u, nil
		}
		return u, u.setStatus(status)

	case clearStatusMsg:
		if msg.seq == u.editStatusSeq {
			u.editStatus = ""
		}
		return u, nil

	case spinnerTickMsg:
		if u.quitting {
			u.spinnerFrame++
			return u, spinnerTick()
		}
		return u, nil

	case shutdownDoneMsg:
		return u, tea.Quit

	case tickMsg:
		if u.quitting {
			return u, nil
		}
		u.services = u.manager.ListServiceStates()
		u.ensureCursorInRange()
		u.refreshViewportContent()
		return u, tickCmd(uiTickInterval)

	default:
		if u.manageMode {
			return u.updateManageInput(msg)
		}
	}

	return u, cmd
}

func (u *UI) shutdownCmd() tea.Cmd {
	return func() tea.Msg {
		u.manager.StopAllServices()
		return shutdownDoneMsg{}
	}
}

func (u *UI) setStatus(text string) tea.Cmd {
	u.editStatus = text
	u.editStatusSeq++
	seq := u.editStatusSeq
	return tea.Tick(statusClearDelay, func(time.Time) tea.Msg {
		return clearStatusMsg{seq: seq}
	})
}

func spinnerTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg(t)
	})
}

func (u *UI) launchEditor() tea.Cmd {
	st := storage.NewStorage()
	data, err := st.LoadData()
	if err != nil {
		return func() tea.Msg { return editResultMsg{err: err} }
	}

	seed, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return func() tea.Msg { return editResultMsg{err: err} }
	}

	tmp, err := os.CreateTemp("", "pf-config-*.json")
	if err != nil {
		return func() tea.Msg { return editResultMsg{err: err} }
	}
	tmpPath := tmp.Name()
	tmp.Write(seed)
	tmp.Close()

	cmd, err := configedit.EditorCommand(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return func() tea.Msg { return editResultMsg{err: err} }
	}

	return tea.ExecProcess(cmd, func(runErr error) tea.Msg {
		if runErr != nil {
			os.Remove(tmpPath)
			return editResultMsg{err: runErr}
		}

		edited, err := os.ReadFile(tmpPath)
		if err != nil {
			os.Remove(tmpPath)
			return editResultMsg{err: err}
		}

		validated, err := configedit.Validate(edited)
		if err != nil {
			return editResultMsg{err: err, tmpPath: tmpPath}
		}

		if err := st.SaveData(validated); err != nil {
			os.Remove(tmpPath)
			return editResultMsg{err: err}
		}

		os.Remove(tmpPath)
		return editResultMsg{ok: true, services: len(validated.Services), groups: len(validated.Groups)}
	})
}

func (u *UI) View() tea.View {
	v := tea.NewView(u.viewContent())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (u *UI) viewContent() string {
	if u.quitting {
		return u.renderShutdownScreen()
	}

	if !u.ready {
		return "Initializing..."
	}

	if u.manageMode {
		if u.addFormMode != "" {
			return u.renderServiceForm()
		}
		if u.groupFormMode != "" {
			return u.renderGroupForm()
		}
		return u.renderManageOverlay()
	}

	u.ensureViewportSize()

	sections := make([]string, 0, 3)
	if len(u.services) == 0 {
		sections = append(sections, renderEmptyState())
	} else {
		maxVis := maxVisibleServices(u.height)
		u.ensureCursorVisible(maxVis)
		sections = append(sections, renderServiceTable(u.services, u.cursorIndex, u.tableOffset, maxVis, u.width))
	}

	logBoxWidth := u.width - 2
	if logBoxWidth < 58 {
		logBoxWidth = 58
	}
	logBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Width(logBoxWidth).
		Render(u.viewport.View())
	sections = append(sections, logBox)

	if u.editStatus != "" {
		statusColor := colorAccentAlt
		if strings.HasPrefix(u.editStatus, "✗") {
			statusColor = colorError
		}
		sections = append(sections, lipgloss.NewStyle().Foreground(statusColor).Render(u.editStatus))
	}

	sections = append(sections, renderHelp(u.width, u.logScopeLabel()))
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
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

func (u *UI) formInputWidth() int {
	if u.width <= 0 {
		return 64
	}
	w := u.width - 11
	if w < 20 {
		w = 20
	}
	return w
}

func newServiceTextInput(placeholder, value string, width int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 1000
	ti.SetWidth(width)
	if value != "" {
		ti.SetValue(value)
	}
	return ti
}

func (u *UI) openNewServiceForm() tea.Cmd {
	u.addFormMode = "new"
	u.addFormOrig = ""
	u.addFormErr = ""
	inputWidth := u.formInputWidth()
	u.addFormName = newServiceTextInput("e.g. db", "", inputWidth)
	u.addFormCmd = newServiceTextInput("e.g. kubectl port-forward service/postgres 5432:5432", "", inputWidth)
	u.addFormFocus = 0
	u.addFormCmd.Blur()
	return u.addFormName.Focus()
}

func (u *UI) openEditServiceFormFor(name string) tea.Cmd {
	command, err := storage.NewStorage().GetService(name)
	if err != nil {
		return nil
	}
	u.addFormMode = "edit"
	u.addFormOrig = name
	u.addFormErr = ""
	inputWidth := u.formInputWidth()
	u.addFormName = newServiceTextInput("service name", name, inputWidth)
	u.addFormCmd = newServiceTextInput("command", command, inputWidth)
	u.addFormFocus = 0
	u.addFormCmd.Blur()
	return u.addFormName.Focus()
}

func (u *UI) closeAddForm() {
	u.addFormMode = ""
	u.addFormErr = ""
	u.addFormName.Blur()
	u.addFormCmd.Blur()
}

func (u *UI) toggleAddFormFocus() tea.Cmd {
	if u.addFormFocus == 0 {
		u.addFormFocus = 1
		u.addFormName.Blur()
		return u.addFormCmd.Focus()
	}
	u.addFormFocus = 0
	u.addFormCmd.Blur()
	return u.addFormName.Focus()
}

func (u *UI) updateAddForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if paste, ok := msg.(tea.PasteMsg); ok {
		return u.updateAddFormInput(paste)
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return u.updateAddFormInput(msg)
	}

	keyRaw := keyMsg.String()
	key := keyRaw
	if keyRaw != "space" {
		key = stringutil.NormalizeToken(keyRaw)
	}

	switch key {
	case "esc":
		u.closeAddForm()
		return u, nil
	case "tab", "shift+tab", "up", "down":
		return u, u.toggleAddFormFocus()
	case "enter":
		return u.submitServiceForm()
	}

	return u.updateAddFormInput(msg)
}

func (u *UI) updateAddFormInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if u.addFormFocus == 0 {
		u.addFormName, cmd = u.addFormName.Update(msg)
	} else {
		u.addFormCmd, cmd = u.addFormCmd.Update(msg)
	}
	return u, cmd
}

func (u *UI) submitServiceForm() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(u.addFormName.Value())
	command := strings.TrimSpace(u.addFormCmd.Value())

	if err := manager.ValidateServiceName(name); err != nil {
		u.addFormErr = err.Error()
		return u, nil
	}
	if err := manager.ValidateCommand(command); err != nil {
		u.addFormErr = err.Error()
		return u, nil
	}

	st := storage.NewStorage()
	var restartCmd tea.Cmd
	var status string

	switch u.addFormMode {
	case "new":
		if _, err := st.GetService(name); err == nil {
			u.addFormErr = fmt.Sprintf("a service named '%s' already exists", name)
			return u, nil
		}
		if err := st.AddService(name, command); err != nil {
			u.addFormErr = err.Error()
			return u, nil
		}
		status = fmt.Sprintf("✓ Service '%s' created — select it and press Enter to run", name)

	case "edit":
		orig := u.addFormOrig
		wasRunning := u.runningNameSet()[orig]

		if name != orig {
			if err := st.RenameService(orig, name); err != nil {
				u.addFormErr = err.Error()
				return u, nil
			}
		}
		if err := st.AddService(name, command); err != nil {
			u.addFormErr = err.Error()
			return u, nil
		}

		if wasRunning {
			newName := name
			restartCmd = func() tea.Msg {
				u.manager.StopService(orig)
				_ = u.manager.StartStoredService(u.ctx, newName)
				return nil
			}
			status = fmt.Sprintf("✓ Service '%s' updated — restarting to apply changes", name)
		} else {
			status = fmt.Sprintf("✓ Service '%s' updated", name)
		}
	}

	u.closeAddForm()
	u.manageSearch = "" // ensure the saved service is visible regardless of any active filter
	u.buildManageRows()
	u.focusManage(rowService, name)

	statusCmd := u.setStatus(status)
	if restartCmd != nil {
		return u, tea.Batch(restartCmd, statusCmd)
	}
	return u, statusCmd
}

func (u *UI) openNewGroupForm() tea.Cmd {
	names, err := storage.NewStorage().ListServiceNames()
	if err != nil {
		return nil
	}
	u.groupFormMode = "new"
	u.groupFormOrig = ""
	u.groupFormErr = ""
	u.groupFormName = newServiceTextInput("e.g. backend", "", u.formInputWidth())
	u.groupFormServices = names
	u.groupFormSelected = make(map[string]bool)
	u.groupFormFocus = 0
	u.groupFormSvcCursor = 0
	return u.groupFormName.Focus()
}

func (u *UI) openEditGroupFormFor(name string) tea.Cmd {
	names, err := storage.NewStorage().ListServiceNames()
	if err != nil {
		return nil
	}
	u.groupFormMode = "edit"
	u.groupFormOrig = name
	u.groupFormErr = ""
	u.groupFormName = newServiceTextInput("group name", name, u.formInputWidth())
	u.groupFormServices = names
	u.groupFormSelected = make(map[string]bool)
	for _, svc := range u.manageGroups[name] {
		u.groupFormSelected[svc] = true
	}
	u.groupFormFocus = 0
	u.groupFormSvcCursor = 0
	return u.groupFormName.Focus()
}

func (u *UI) closeGroupForm() {
	u.groupFormMode = ""
	u.groupFormErr = ""
	u.groupFormName.Blur()
}

func (u *UI) toggleGroupFormFocus() tea.Cmd {
	if u.groupFormFocus == 0 {
		u.groupFormFocus = 1
		u.groupFormName.Blur()
		return nil
	}
	u.groupFormFocus = 0
	return u.groupFormName.Focus()
}

func (u *UI) updateGroupForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if paste, ok := msg.(tea.PasteMsg); ok {
		return u.updateGroupNameInput(paste)
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return u.updateGroupNameInput(msg)
	}

	keyRaw := keyMsg.String()
	key := keyRaw
	if keyRaw != "space" {
		key = stringutil.NormalizeToken(keyRaw)
	}

	switch key {
	case "esc":
		u.closeGroupForm()
		return u, nil
	case "tab", "shift+tab":
		return u, u.toggleGroupFormFocus()
	case "enter":
		return u.submitGroupForm()
	}

	if u.groupFormFocus == 1 {
		switch key {
		case "up", "k":
			if u.groupFormSvcCursor > 0 {
				u.groupFormSvcCursor--
			}
		case "down", "j":
			if u.groupFormSvcCursor < len(u.groupFormServices)-1 {
				u.groupFormSvcCursor++
			}
		case "space":
			if u.groupFormSvcCursor >= 0 && u.groupFormSvcCursor < len(u.groupFormServices) {
				svc := u.groupFormServices[u.groupFormSvcCursor]
				u.groupFormSelected[svc] = !u.groupFormSelected[svc]
			}
		}
		return u, nil
	}

	return u.updateGroupNameInput(msg)
}

func (u *UI) updateGroupNameInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	u.groupFormName, cmd = u.groupFormName.Update(msg)
	return u, cmd
}

func (u *UI) submitGroupForm() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(u.groupFormName.Value())
	if err := manager.ValidateServiceName(name); err != nil {
		u.groupFormErr = err.Error()
		return u, nil
	}

	selected := make([]string, 0, len(u.groupFormSelected))
	for _, svc := range u.groupFormServices {
		if u.groupFormSelected[svc] {
			selected = append(selected, svc)
		}
	}

	st := storage.NewStorage()
	var status string

	switch u.groupFormMode {
	case "new":
		if _, exists := u.manageGroups[name]; exists {
			u.groupFormErr = fmt.Sprintf("a group named '%s' already exists", name)
			return u, nil
		}
		if err := st.AddGroup(name, selected); err != nil {
			u.groupFormErr = err.Error()
			return u, nil
		}
		status = fmt.Sprintf("✓ Group '%s' created with %d service(s)", name, len(selected))

	case "edit":
		orig := u.groupFormOrig
		if name != orig {
			if err := st.RenameGroup(orig, name); err != nil {
				u.groupFormErr = err.Error()
				return u, nil
			}
		}
		// AddGroup overwrites the membership of an existing group.
		if err := st.AddGroup(name, selected); err != nil {
			u.groupFormErr = err.Error()
			return u, nil
		}
		status = fmt.Sprintf("✓ Group '%s' updated (%d service(s))", name, len(selected))
	}

	u.closeGroupForm()
	u.manageSearch = "" // ensure the saved group is visible regardless of any active filter
	u.buildManageRows()
	u.focusManage(rowGroup, name)
	return u, u.setStatus(status)
}

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
	h := len(helpLines(u.width, u.logScopeLabel())) + 2 // help box border
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

func renderEmptyState() string {
	emptyStyle := lipgloss.NewStyle().
		Foreground(colorMuted).
		Italic(true)
	return emptyStyle.Render("⚬ No services running...")
}

func renderServiceTable(services []model.Service, selectedIndex, offset, maxVisible, width int) string {
	if width < 60 {
		width = 60
	}

	if maxVisible <= 0 {
		maxVisible = len(services)
	}
	start := offset
	if start < 0 {
		start = 0
	}
	end := start + maxVisible
	if end > len(services) {
		end = len(services)
	}

	compact := width < 90
	showIcons := false
	for i := start; i < end; i++ {
		if services[i].IconEnabled {
			showIcons = true
			break
		}
	}
	iconWidth := 0
	if showIcons {
		iconWidth = 2
	}
	statusWidth := 12
	uptimeWidth := 8
	portWidth := 6
	restartWidth := 8
	maxNameLen := 7
	for i := range services {
		nameLen := len(services[i].Name)
		if nameLen > maxNameLen {
			maxNameLen = nameLen
		}
	}
	if maxNameLen > 30 {
		maxNameLen = 30
	}

	available := width - 2
	if available < 60 {
		available = 60
	}
	if compact {
		minName := 8
		fixed := statusWidth + portWidth + iconWidth + 6
		nameWidth := available - fixed
		if nameWidth < minName {
			nameWidth = minName
		}
		if nameWidth > maxNameLen {
			nameWidth = maxNameLen
		}
		maxNameLen = nameWidth
	} else {
		minName := 10
		fixed := statusWidth + uptimeWidth + portWidth + restartWidth + iconWidth + 10
		nameWidth := available - fixed
		if nameWidth < minName {
			nameWidth = minName
		}
		if nameWidth > maxNameLen {
			nameWidth = maxNameLen
		}
		maxNameLen = nameWidth
	}

	rows := make([]string, 0, len(services)+2)
	headerPrefix := "  "
	nameCellWidth := maxNameLen + iconWidth
	headerLine := headerPrefix + padRightDisplayWidth("SERVICE", nameCellWidth) + fmt.Sprintf(
		"  %-*s",
		statusWidth, "STATUS",
	)
	if compact {
		headerLine += fmt.Sprintf("  %-*s", portWidth, "PORT")
	} else {
		headerLine += fmt.Sprintf(
			"  %-*s  %-*s  %-*s",
			uptimeWidth, "UPTIME",
			portWidth, "PORT",
			restartWidth, "RESTARTS",
		)
	}
	header := lipgloss.NewStyle().
		Foreground(colorHeading).
		Bold(true).
		Render(headerLine)
	rows = append(rows, header)

	sepWidth := width - 6
	if sepWidth < 50 {
		sepWidth = 50
	}
	if sepWidth > 200 {
		sepWidth = 200
	}
	rows = append(rows, lipgloss.NewStyle().Foreground(colorBorder).Render(strings.Repeat("─", sepWidth)))

	for i := start; i < end; i++ {
		svc := &services[i]
		var statusIcon, statusText string
		var statusColor color.Color

		selected := i == selectedIndex
		highlight := "  "
		if selected {
			highlight = "► "
		}

		switch svc.Status {
		case model.StatusHealthy:
			statusColor = statusHealthyColor
			statusIcon = "●"
			statusText = "HEALTHY"
		case model.StatusConnecting:
			statusColor = statusConnectingColor
			statusIcon = "◐"
			statusText = "CONNECTING"
		case model.StatusError:
			statusColor = statusErrorColor
			statusIcon = "✗"
			statusText = "ERROR"
		}

		uptime := formatUptime(svc.StartTime)

		status := fmt.Sprintf("%s %-*s", statusIcon, statusWidth-2, statusText)
		uptimeStr := fmt.Sprintf("%-*s", uptimeWidth, uptime)
		portStr := fmt.Sprintf("%-*s", portWidth, svc.LocalPort)
		restarts := fmt.Sprintf("%-*d", restartWidth, svc.RestartCount)

		nameColor := colorText
		if selected {
			nameColor = colorAccent
		}
		displayName := truncateRunes(svc.Name, maxNameLen)
		nameText := padRightDisplayWidth(displayName, maxNameLen)
		styledName := lipgloss.NewStyle().
			Foreground(nameColor).
			Bold(true).
			Render(nameText)
		if showIcons {
			cell := "  "
			if svc.IconEnabled {
				icon := serviceIcon(svc)
				cell = renderIconCell(icon.Glyph, icon.Color)
			}
			styledName = padRightDisplayWidth(cell+styledName, nameCellWidth)
		}

		styledStatus := lipgloss.NewStyle().
			Foreground(statusColor).
			Render(status)

		styledUptime := lipgloss.NewStyle().
			Foreground(colorMuted).
			Render(uptimeStr)

		styledRestarts := lipgloss.NewStyle().
			Foreground(colorMuted).
			Render(restarts)

		styledPort := lipgloss.NewStyle().
			Foreground(colorText).
			Render(portStr)

		marker := highlight
		if selected {
			marker = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(highlight)
		}

		row := marker + styledName + "  " + styledStatus
		if compact {
			row += "  " + styledPort
		} else {
			row += "  " + styledUptime + "  " + styledPort + "  " + styledRestarts
		}
		rows = append(rows, row)
	}

	if len(services) > maxVisible {
		above := start
		below := len(services) - end
		var parts []string
		if above > 0 {
			parts = append(parts, fmt.Sprintf("↑ %d more above", above))
		}
		if below > 0 {
			parts = append(parts, fmt.Sprintf("↓ %d more below", below))
		}
		indicator := fmt.Sprintf("%s   (%d–%d of %d • ↑↓ to scroll)",
			strings.Join(parts, "   "), start+1, end, len(services))
		rows = append(rows, lipgloss.NewStyle().
			Foreground(colorWarn).
			Bold(true).
			Render(indicator))
	}

	table := lipgloss.JoinVertical(lipgloss.Left, rows...)
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(width - 2)

	return style.Render(table)
}

func formatUptime(startTime time.Time) string {
	if startTime.IsZero() {
		return "-"
	}

	duration := time.Since(startTime)
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

func renderLogsContent(services []model.Service, maxWidth int) string {
	var content strings.Builder

	type logWithService struct {
		ServiceName string
		Entry       model.LogEntry
	}

	allLogs := make([]logWithService, 0)
	for i := range services {
		svc := &services[i]
		for _, log := range svc.Logs {
			allLogs = append(allLogs, logWithService{
				ServiceName: svc.Name,
				Entry:       log,
			})
		}
	}

	sort.Slice(allLogs, func(i, j int) bool {
		return allLogs[i].Entry.Time.Before(allLogs[j].Entry.Time)
	})

	if len(allLogs) == 0 {
		content.WriteString(lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true).
			Render("No logs yet..."))
		return content.String()
	}

	for i := 0; i < len(allLogs); i++ {
		log := allLogs[i]
		timestamp := log.Entry.Time.Format("15:04:05")

		nameWidth := maxWidth / 4
		if nameWidth < 8 {
			nameWidth = 8
		}
		if nameWidth > 24 {
			nameWidth = 24
		}
		serviceName := truncateRunes(log.ServiceName, nameWidth)
		namePlain := padRightRunes(serviceName, nameWidth)

		message := log.Entry.Message
		msgColor := colorText
		if log.Entry.IsError {
			msgColor = colorError
		} else if strings.Contains(message, "━━━━") {
			msgColor = colorWarn
		}

		prefixWidth := nameWidth + 12
		availableWidth := maxWidth - prefixWidth
		if availableWidth < 20 {
			availableWidth = 20
		}

		wrappedLines := wrapText(message, availableWidth)

		nameStyled := lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			Render(namePlain)

		timeStyled := lipgloss.NewStyle().
			Foreground(colorMuted).
			Render(timestamp)

		if len(wrappedLines) > 0 {
			msgStyled := lipgloss.NewStyle().
				Foreground(msgColor).
				Render(wrappedLines[0])
			logLine := fmt.Sprintf("[%s %s] %s", nameStyled, timeStyled, msgStyled)
			content.WriteString(logLine)
			content.WriteString("\n")

			if len(wrappedLines) > 1 {
				indent := strings.Repeat(" ", prefixWidth)
				for j := 1; j < len(wrappedLines); j++ {
					msgStyled := lipgloss.NewStyle().
						Foreground(msgColor).
						Render(wrappedLines[j])
					content.WriteString(indent + msgStyled + "\n")
				}
			}
		}
	}

	return content.String()
}

func wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}
	if len(text) <= maxWidth {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		for i := 0; i < len(text); i += maxWidth {
			end := i + maxWidth
			if end > len(text) {
				end = len(text)
			}
			lines = append(lines, text[i:end])
		}
		return lines
	}

	var currentLine strings.Builder
	for _, word := range words {
		if len(word) > maxWidth {
			if currentLine.Len() > 0 {
				lines = append(lines, currentLine.String())
				currentLine.Reset()
			}
			for i := 0; i < len(word); i += maxWidth {
				end := i + maxWidth
				if end > len(word) {
					end = len(word)
				}
				lines = append(lines, word[i:end])
			}
			continue
		}

		testLine := currentLine.String()
		if len(testLine) > 0 {
			testLine += " " + word
		} else {
			testLine = word
		}

		if len(testLine) > maxWidth {
			if currentLine.Len() > 0 {
				lines = append(lines, currentLine.String())
				currentLine.Reset()
			}
			currentLine.WriteString(word)
		} else {
			if currentLine.Len() > 0 {
				currentLine.WriteString(" ")
			}
			currentLine.WriteString(word)
		}
	}

	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return lines
}

func truncateRunes(text string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func padRightRunes(text string, width int) string {
	runes := []rune(text)
	if len(runes) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-len(runes))
}

func padRightDisplayWidth(text string, width int) string {
	textWidth := lipgloss.Width(text)
	if textWidth >= width {
		return text
	}
	return text + strings.Repeat(" ", width-textWidth)
}

// serviceIcon returns the icon for a running service. It prefers the glyph the
// manager already resolved (which honors the user's config overrides) and falls
// back to the built-in port mapping when none was carried — e.g. in tests that
// construct a Service from just a port.
func serviceIcon(svc *model.Service) icons.Icon {
	if svc.IconGlyph != "" {
		return icons.Icon{Glyph: svc.IconGlyph, Color: svc.IconColor}
	}
	return icons.ForPort(svc.MainPort)
}

// renderIconCell renders a fixed two-column icon cell from an already-resolved
// glyph/color. An empty glyph yields two blank columns so name columns stay
// aligned whether or not a row carries an icon.
func renderIconCell(glyph, colorHex string) string {
	if glyph == "" {
		return "  "
	}
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color(colorHex)).Render(glyph)
	return padRightDisplayWidth(styled+" ", 2)
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

func (u *UI) renderManageGroupRow(name string, cursorOn bool, maxNameLen int, running map[string]bool) string {
	highlight := "  "
	if cursorOn {
		highlight = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("► ")
	}
	box := u.renderSelectCheckbox(u.manageSelGroups[name])

	nameColor := colorText
	if cursorOn {
		nameColor = colorAccent
	}
	styledName := lipgloss.NewStyle().Foreground(nameColor).Bold(true).
		Render(padRightDisplayWidth(truncateRunes(name, maxNameLen), maxNameLen))

	members := u.manageGroups[name]
	run := 0
	for _, svc := range members {
		if running[svc] {
			run++
		}
	}
	info := lipgloss.NewStyle().Foreground(colorMuted).
		Render(fmt.Sprintf("%s  %d/%d running", summarizeMembers(members), run, len(members)))

	icon := u.overlayIconCell(u.manageIcons.set.ForGroup())

	return highlight + box + " " + icon + styledName + "  " + info
}

func (u *UI) renderManageServiceRow(name string, cursorOn bool, maxNameLen int, running map[string]bool) string {
	highlight := "  "
	if cursorOn {
		highlight = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("► ")
	}

	nameColor := colorText
	if cursorOn {
		nameColor = colorAccent
	}
	styledName := lipgloss.NewStyle().Foreground(nameColor).Bold(true).
		Render(padRightDisplayWidth(truncateRunes(name, maxNameLen), maxNameLen))

	var box, indicator string
	if running[name] {
		box = lipgloss.NewStyle().Foreground(colorMuted).Render("   ")
		indicator = lipgloss.NewStyle().Foreground(colorAccentAlt).Render("● running")
	} else {
		box = u.renderSelectCheckbox(u.manageSelSvcs[name])
		indicator = lipgloss.NewStyle().Foreground(colorMuted).Render("○ stopped")
	}

	icon := u.overlayIconCell(u.manageIcons.set.ForPort(u.manageIcons.ports[name]))

	return highlight + box + " " + icon + styledName + "  " + indicator
}

// renderSelectCheckbox draws a multi-select checkbox, brightening to the accent
// color when ticked so selected rows read at a glance.
func (u *UI) renderSelectCheckbox(selected bool) string {
	if selected {
		return lipgloss.NewStyle().Foreground(colorAccentAlt).Bold(true).Render("[✓]")
	}
	return lipgloss.NewStyle().Foreground(colorMuted).Render("[ ]")
}

// overlayIconCell renders the leading icon cell for an overlay row from an
// already-resolved icon, or an empty string when icons are disabled.
func (u *UI) overlayIconCell(icon icons.Icon) string {
	if !u.manageIcons.enabled {
		return ""
	}
	return renderIconCell(icon.Glyph, icon.Color)
}

func (u *UI) renderManageOverlay() string {
	width := u.width
	if width <= 0 {
		width = 120
	}
	if width < 60 {
		width = 60
	}

	running := u.runningNameSet()

	maxNameLen := 7
	for _, n := range u.manageGroupNames {
		if len(n) > maxNameLen {
			maxNameLen = len(n)
		}
	}
	for _, n := range u.manageServices {
		if len(n) > maxNameLen {
			maxNameLen = len(n)
		}
	}
	if maxNameLen > 30 {
		maxNameLen = 30
	}

	u.ensureManageVisible()
	visible := u.manageVisibleRows()
	start := u.manageOffset
	end := start + visible
	if end > len(u.manageRows) {
		end = len(u.manageRows)
	}
	if start > end {
		start = end
	}

	rows := make([]string, 0, end-start+3)
	rows = append(rows, lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("ADD / EDIT")+
		lipgloss.NewStyle().Foreground(colorMuted).Render("  — groups & services"))
	rows = append(rows, u.renderManageSearchLine())
	for i := start; i < end; i++ {
		row := u.manageRows[i]
		cursorOn := i == u.manageCursor
		switch row.kind {
		case rowHeaderGroups:
			rows = append(rows, lipgloss.NewStyle().Foreground(colorHeading).Bold(true).Render("GROUPS"))
		case rowHeaderServices:
			rows = append(rows, lipgloss.NewStyle().Foreground(colorHeading).Bold(true).Render("SERVICES"))
		case rowEmptyGroups:
			text := "  (no groups — ^n to create)"
			if u.manageSearch != "" {
				text = "  (no matching groups)"
			}
			rows = append(rows, lipgloss.NewStyle().Foreground(colorMuted).Italic(true).Render(text))
		case rowEmptyServices:
			text := "  (no services — ^n to create)"
			if u.manageSearch != "" {
				text = "  (no matching services)"
			}
			rows = append(rows, lipgloss.NewStyle().Foreground(colorMuted).Italic(true).Render(text))
		case rowGroup:
			rows = append(rows, u.renderManageGroupRow(row.name, cursorOn, maxNameLen, running))
		case rowService:
			rows = append(rows, u.renderManageServiceRow(row.name, cursorOn, maxNameLen, running))
		}
	}

	if len(u.manageRows) > visible {
		rows = append(rows, lipgloss.NewStyle().Foreground(colorMuted).
			Render(fmt.Sprintf("(%d–%d of %d)", start+1, end, len(u.manageRows))))
	}

	table := lipgloss.JoinVertical(lipgloss.Left, rows...)
	overlayBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(width - 2).
		Render(table)

	sections := []string{overlayBox}

	switch {
	case u.manageNewPrompt:
		promptText := lipgloss.NewStyle().Foreground(colorAccentAlt).Bold(true).Render("Create new —")
		promptKeys := renderActionChips([][2]string{{"g", "group"}, {"s", "service"}, {"esc", "cancel"}})
		promptBody := lipgloss.JoinVertical(lipgloss.Left, promptText, "", promptKeys)
		sections = append(sections, lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1).
			Width(width-2).
			Render(promptBody))
	case u.manageConfirmDelete != "":
		msg := fmt.Sprintf("Delete service '%s'? This cannot be undone.", u.manageConfirmDelete)
		if u.manageConfirmKind == "group" {
			msg = fmt.Sprintf("Delete group '%s'? Member services are kept.", u.manageConfirmDelete)
		}
		confirmText := lipgloss.NewStyle().Foreground(colorWarn).Bold(true).Render(msg)
		confirmKeys := renderActionChips([][2]string{{"y", "confirm"}, {"n", "cancel"}})
		confirmBody := lipgloss.JoinVertical(lipgloss.Left, confirmText, "", confirmKeys)
		sections = append(sections, lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1).
			Width(width-2).
			Render(confirmBody))
	case u.manageErr != "":
		sections = append(sections, lipgloss.NewStyle().Foreground(colorError).Render("✗ "+u.manageErr))
	case u.manageInfo != "":
		sections = append(sections, lipgloss.NewStyle().Foreground(colorAccentAlt).Bold(true).Render(u.manageInfo))
	}

	sections = append(sections, renderActionChips([][2]string{
		{"type", "search"},
		{"↑↓", "navigate"},
		{"Space", "select"},
		{"Enter", "run"},
		{"^n", "new"},
		{"^e", "edit"},
		{"^d", "delete"},
		{"^c", "config"},
		{"Esc", "clear/close"},
	}))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderManageSearchLine renders the always-focused search input shown at the top
// of the manage overlay. Typing anywhere in the overlay feeds this query.
func (u *UI) renderManageSearchLine() string {
	label := lipgloss.NewStyle().Foreground(colorMuted).Render("Search: ")
	cursor := lipgloss.NewStyle().Foreground(colorAccent).Render("▏")
	if u.manageSearch == "" {
		placeholder := lipgloss.NewStyle().Foreground(colorMuted).Italic(true).Render("type to filter…")
		return label + cursor + placeholder
	}
	query := lipgloss.NewStyle().Foreground(colorAccentAlt).Bold(true).Render(u.manageSearch)
	return label + query + cursor
}

func (u *UI) renderServiceForm() string {
	width := u.width
	if width <= 0 {
		width = 120
	}
	if width < 60 {
		width = 60
	}

	title := "Add new service"
	if u.addFormMode == "edit" {
		title = fmt.Sprintf("Edit service: %s", u.addFormOrig)
	}
	titleStyled := lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Render(title)

	labelStyle := lipgloss.NewStyle().Foreground(colorMuted)
	activeLabel := lipgloss.NewStyle().Foreground(colorAccentAlt).Bold(true)

	nameLabel := labelStyle.Render("  Name:")
	cmdLabel := labelStyle.Render("  Command:")
	if u.addFormFocus == 0 {
		nameLabel = activeLabel.Render("► Name:")
	} else {
		cmdLabel = activeLabel.Render("► Command:")
	}

	rows := []string{
		titleStyled,
		"",
		nameLabel,
		"  " + u.addFormName.View(),
		"",
		cmdLabel,
		"  " + u.addFormCmd.View(),
	}

	if u.addFormErr != "" {
		rows = append(rows, "", lipgloss.NewStyle().Foreground(colorError).Render("✗ "+u.addFormErr))
	}

	body := lipgloss.JoinVertical(lipgloss.Left, rows...)
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(width - 2)

	box := style.Render(body)

	instructionStyled := renderActionChips([][2]string{
		{"Tab/↑↓", "switch field"},
		{"Enter", "save"},
		{"Esc", "back"},
	})

	return lipgloss.JoinVertical(lipgloss.Left, box, instructionStyled)
}

func summarizeMembers(members []string) string {
	if len(members) == 0 {
		return "(empty)"
	}
	joined := strings.Join(members, ", ")
	if len(joined) > 48 {
		return fmt.Sprintf("%d services", len(members))
	}
	return joined
}

func (u *UI) renderGroupForm() string {
	width := u.width
	if width <= 0 {
		width = 120
	}
	if width < 60 {
		width = 60
	}

	title := "New group"
	if u.groupFormMode == "edit" {
		title = fmt.Sprintf("Edit group: %s", u.groupFormOrig)
	}
	titleStyled := lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Render(title)

	labelStyle := lipgloss.NewStyle().Foreground(colorMuted)
	activeLabel := lipgloss.NewStyle().Foreground(colorAccentAlt).Bold(true)

	nameLabel := labelStyle.Render("  Name:")
	servicesLabel := labelStyle.Render("  Services:")
	if u.groupFormFocus == 0 {
		nameLabel = activeLabel.Render("► Name:")
	} else {
		servicesLabel = activeLabel.Render("► Services:")
	}

	rows := []string{
		titleStyled,
		"",
		nameLabel,
		"  " + u.groupFormName.View(),
		"",
		servicesLabel,
	}

	if len(u.groupFormServices) == 0 {
		rows = append(rows, lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true).
			Render("  No services available — create services first"))
	} else {
		const maxVisible = 20
		start := 0
		if u.groupFormSvcCursor >= maxVisible {
			start = u.groupFormSvcCursor - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(u.groupFormServices) {
			end = len(u.groupFormServices)
		}

		for i := start; i < end; i++ {
			svc := u.groupFormServices[i]
			onCursor := u.groupFormFocus == 1 && i == u.groupFormSvcCursor
			marker := "  "
			if onCursor {
				marker = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("► ")
			}
			checkbox := "[ ]"
			if u.groupFormSelected[svc] {
				checkbox = "[✓]"
			}
			svcColor := colorText
			if onCursor {
				svcColor = colorAccent
			}
			line := marker +
				lipgloss.NewStyle().Foreground(colorMuted).Render(checkbox+" ") +
				lipgloss.NewStyle().Foreground(svcColor).Render(svc)
			rows = append(rows, line)
		}

		if len(u.groupFormServices) > maxVisible {
			rows = append(rows, lipgloss.NewStyle().
				Foreground(colorMuted).
				Render(fmt.Sprintf("  (%d–%d of %d)", start+1, end, len(u.groupFormServices))))
		}
	}

	if u.groupFormErr != "" {
		rows = append(rows, "", lipgloss.NewStyle().Foreground(colorError).Render("✗ "+u.groupFormErr))
	}

	body := lipgloss.JoinVertical(lipgloss.Left, rows...)
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(width - 2)

	box := style.Render(body)

	instructionStyled := renderActionChips([][2]string{
		{"Tab", "switch field"},
		{"↑↓", "navigate"},
		{"Space", "toggle"},
		{"Enter", "save"},
		{"Esc", "back"},
	})

	return lipgloss.JoinVertical(lipgloss.Left, box, instructionStyled)
}

func renderActionChips(pairs [][2]string) string {
	keyStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(colorMuted)
	sep := descStyle.Render("  •  ")
	chips := make([]string, 0, len(pairs))
	for _, p := range pairs {
		chips = append(chips, keyStyle.Render(p[0])+descStyle.Render(" "+p[1]))
	}
	return strings.Join(chips, sep)
}

// helpLines builds the wrapped, balanced content rows for the help bar (without
// the surrounding border). The height layout depends on len(helpLines(...)), so
// renderHelp must render exactly these lines.
func helpLines(width int, logScope string) []string {
	if width < 60 {
		width = 60
	}

	keyStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(colorMuted)
	const sepText = "  •  "
	sepStyled := descStyle.Render(sepText)
	sepW := lipgloss.Width(sepText)

	type chip struct{ k, d string }
	var chips []chip
	if width < 90 {
		chips = []chip{
			{"↑↓", "move"},
			{"l", "logs=" + logScope},
			{"a", "add/edit"},
			{"c", "config"},
			{"r", "restart"},
			{"s", "stop"},
			{"q", "quit"},
		}
	} else {
		chips = []chip{
			{"↑↓/j/k", "move"},
			{"l", "logs=" + logScope},
			{"a", "add/edit"},
			{"c", "config"},
			{"r", "restart"},
			{"^r", "restart all"},
			{"s", "stop"},
			{"q", "quit"},
		}
	}

	n := len(chips)
	styled := make([]string, n)
	widths := make([]int, n)
	for i, c := range chips {
		styled[i] = keyStyle.Render(c.k) + descStyle.Render(" "+c.d)
		widths[i] = lipgloss.Width(c.k + " " + c.d)
	}

	inner := width - 4 // 2 border + 2 padding
	if inner < 10 {
		inner = 10
	}

	// Minimum number of lines a greedy fit needs at this width.
	minLines := 1
	lineW := 0
	for i, w := range widths {
		if i == 0 {
			lineW = w
			continue
		}
		if lineW+sepW+w > inner {
			minLines++
			lineW = w
		} else {
			lineW += sepW + w
		}
	}

	// Split into minLines rows of (almost) equal chip count so the rows look
	// balanced (e.g. 4+4 instead of 7+1). Fall back to greedy if an even
	// split would overflow.
	return balancedHelpLines(styled, widths, sepStyled, sepW, inner, minLines)
}

func renderHelp(width int, logScope string) string {
	boxWidth := width
	if boxWidth < 60 {
		boxWidth = 60
	}

	help := strings.Join(helpLines(width, logScope), "\n")

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(boxWidth - 2)

	return style.Render(help)
}

// balancedHelpLines splits chips into L contiguous rows of as-equal-as-possible
// count. If any such row would exceed inner width, it falls back to greedy
// packing (which fits by construction).
func balancedHelpLines(styled []string, widths []int, sepStyled string, sepW, inner, L int) []string {
	n := len(styled)
	if L < 1 {
		L = 1
	}

	base, rem := n/L, n%L
	groups := make([][]string, 0, L)
	idx := 0
	for g := 0; g < L; g++ {
		cnt := base
		if g < rem {
			cnt++
		}
		w := 0
		for j := idx; j < idx+cnt; j++ {
			w += widths[j]
			if j > idx {
				w += sepW
			}
		}
		if w > inner {
			return greedyHelpLines(styled, widths, sepStyled, sepW, inner)
		}
		groups = append(groups, styled[idx:idx+cnt])
		idx += cnt
	}

	out := make([]string, 0, len(groups))
	for _, g := range groups {
		out = append(out, strings.Join(g, sepStyled))
	}
	return out
}

func greedyHelpLines(styled []string, widths []int, sepStyled string, sepW, inner int) []string {
	var lines []string
	var line string
	lineW := 0
	for i, s := range styled {
		switch {
		case line == "":
			line, lineW = s, widths[i]
		case lineW+sepW+widths[i] > inner:
			lines = append(lines, line)
			line, lineW = s, widths[i]
		default:
			line += sepStyled + s
			lineW += sepW + widths[i]
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func (u *UI) renderShutdownScreen() string {
	frame := spinnerFrames[u.spinnerFrame%len(spinnerFrames)]

	shutdownStyle := lipgloss.NewStyle().
		Foreground(colorAccentAlt).
		Bold(true)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(1, 4).
		Align(lipgloss.Center).
		Render(shutdownStyle.Render(fmt.Sprintf("%s  Stopping services, please wait...", frame)))

	if u.width <= 0 || u.height <= 0 {
		return box
	}
	return lipgloss.Place(u.width, u.height, lipgloss.Center, lipgloss.Center, box)
}

func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
