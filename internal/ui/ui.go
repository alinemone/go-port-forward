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

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/configedit"
	"github.com/alinemone/go-port-forward/internal/manager"
	"github.com/alinemone/go-port-forward/internal/model"
	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/stringutil"
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

var (
	colorText      = lipgloss.Color("#EAEEF5")
	colorMuted     = lipgloss.Color("#7C879B")
	colorBorder    = lipgloss.Color("#33415A")
	colorAccent    = lipgloss.Color("#5BD4FF")
	colorAccentAlt = lipgloss.Color("#73FFB6")
	colorWarn      = lipgloss.Color("#FFD166")
	colorError     = lipgloss.Color("#FF6B6B")
	colorHeading   = lipgloss.Color("#AEB9CC")
	colorSelected  = lipgloss.Color("#1E3A5F")
)

type Controller interface {
	ListServiceStates() []model.Service
	StartStoredService(ctx context.Context, name string) error
	StopService(name string)
	StopAllServices()
	RestartService(ctx context.Context, name string) error
	RestartAllServices(ctx context.Context)
}

type UI struct {
	manager           Controller
	services          []model.Service
	cursorIndex       int
	quitting          bool
	width             int
	height            int
	viewport          viewport.Model
	ready             bool
	ctx               context.Context
	addMode           bool
	addCandidates     []string
	addCursor         int
	addSelected       map[string]bool
	addFormMode       string
	addFormName       textinput.Model
	addFormCmd        textinput.Model
	addFormFocus      int // 0 = name, 1 = command
	addFormOrig       string
	addFormErr        string
	editStatus        string
	editStatusSeq     int
	logFilterSelected bool
	spinnerFrame      int
	tableOffset       int
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

		viewportHeight := calculateViewportHeight(len(u.services), u.height)
		if !u.ready {
			u.viewport = viewport.New(viewport.WithWidth(msg.Width), viewport.WithHeight(viewportHeight))
			u.viewport.YPosition = 0
			u.ready = true
		} else {
			u.viewport.SetWidth(msg.Width)
			u.viewport.SetHeight(viewportHeight)
		}

		if u.addMode && u.addFormMode != "" {
			inputWidth := u.formInputWidth()
			u.addFormName.SetWidth(inputWidth)
			u.addFormCmd.SetWidth(inputWidth)
		}

	case tea.MouseWheelMsg:
		if u.addMode {
			if u.addFormMode == "" {
				switch msg.Button {
				case tea.MouseWheelUp:
					if u.addCursor > 0 {
						u.addCursor--
					}
				case tea.MouseWheelDown:
					if u.addCursor < len(u.addCandidates)-1 {
						u.addCursor++
					}
				}
			}
		} else {
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
		if u.addMode {
			return u.updateAddMode(msg)
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
			u.enterAddMode()

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
			if u.addMode && u.addFormMode == "" {
				u.refreshAddCandidates()
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
	services, _ := st.LoadServices()
	groups, _ := st.ListGroups()

	seed, err := json.MarshalIndent(&storage.StorageData{Services: services, Groups: groups}, "", "  ")
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

	if u.addMode {
		if u.addFormMode != "" {
			return u.renderServiceForm()
		}
		return u.renderAddServiceOverlay()
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
		BorderForeground(colorAccentAlt).
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

func (u *UI) enterAddMode() {
	st := storage.NewStorage()
	allServices, err := st.ListServiceNames()
	if err != nil {
		return
	}

	u.addMode = true
	u.addFormMode = ""
	u.addFormErr = ""
	u.addCandidates = allServices
	u.addCursor = 0
	u.addSelected = make(map[string]bool)
}

func (u *UI) exitAddMode() {
	u.addMode = false
	u.addFormMode = ""
	u.addFormErr = ""
	u.addCandidates = nil
	u.addCursor = 0
	u.addSelected = nil
}

func (u *UI) runningNameSet() map[string]bool {
	set := make(map[string]bool, len(u.services))
	for i := range u.services {
		set[u.services[i].Name] = true
	}
	return set
}

func (u *UI) updateAddMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if u.addFormMode != "" {
		return u.updateAddForm(msg)
	}

	keyRaw := msg.String()
	key := keyRaw
	if keyRaw != "space" {
		key = stringutil.NormalizeToken(keyRaw)
	}
	switch key {
	case "esc":
		u.exitAddMode()
	case "up", "k":
		if u.addCursor > 0 {
			u.addCursor--
		}
	case "down", "j":
		if u.addCursor < len(u.addCandidates)-1 {
			u.addCursor++
		}
	case "space":
		if u.addCursor >= 0 && u.addCursor < len(u.addCandidates) {
			serviceName := u.addCandidates[u.addCursor]
			if !u.runningNameSet()[serviceName] {
				u.addSelected[serviceName] = !u.addSelected[serviceName]
			}
		}
	case "n":
		return u, u.openNewServiceForm()
	case "e":
		return u, u.openEditServiceForm()
	case "c":
		return u, u.launchEditor()
	case "enter":
		running := u.runningNameSet()
		for serviceName, selected := range u.addSelected {
			if selected && !running[serviceName] {
				_ = u.manager.StartStoredService(u.ctx, serviceName)
			}
		}
		u.exitAddMode()
	}
	return u, nil
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

func (u *UI) openEditServiceForm() tea.Cmd {
	if u.addCursor < 0 || u.addCursor >= len(u.addCandidates) {
		return nil
	}
	name := u.addCandidates[u.addCursor]
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

func (u *UI) updateAddForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyRaw := msg.String()
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
	u.refreshAddCandidates()
	u.focusCandidate(name)

	statusCmd := u.setStatus(status)
	if restartCmd != nil {
		return u, tea.Batch(restartCmd, statusCmd)
	}
	return u, statusCmd
}

func (u *UI) refreshAddCandidates() {
	names, err := storage.NewStorage().ListServiceNames()
	if err != nil {
		return
	}
	u.addCandidates = names
	if u.addSelected == nil {
		u.addSelected = make(map[string]bool)
	}
	if u.addCursor >= len(u.addCandidates) {
		u.addCursor = len(u.addCandidates) - 1
	}
	if u.addCursor < 0 {
		u.addCursor = 0
	}
}

func (u *UI) focusCandidate(name string) {
	for i, c := range u.addCandidates {
		if c == name {
			u.addCursor = i
			return
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

func maxVisibleServices(totalHeight int) int {
	if totalHeight <= 0 {
		return 8
	}
	cap := totalHeight / 3
	if cap < 3 {
		cap = 3
	}
	if cap > 15 {
		cap = 15
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

	viewportHeight := calculateViewportHeight(len(u.services), u.height)
	if u.viewport.Height() != viewportHeight {
		u.viewport.SetHeight(viewportHeight)
	}
	if u.viewport.Width() != u.width {
		u.viewport.SetWidth(u.width)
	}
}

func calculateViewportHeight(serviceCount, totalHeight int) int {
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
	overhead := tableLines + 2 + 3
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
		fixed := statusWidth + portWidth + 6
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
		fixed := statusWidth + uptimeWidth + portWidth + restartWidth + 10
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
	headerLine := headerPrefix + fmt.Sprintf(
		"%-*s  %-*s",
		maxNameLen, "SERVICE",
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
			statusColor = colorAccentAlt
			statusIcon = "●"
			statusText = "HEALTHY"
		case model.StatusConnecting:
			statusColor = colorWarn
			statusIcon = "◐"
			statusText = "CONNECTING"
		case model.StatusError:
			statusColor = colorError
			statusIcon = "✗"
			statusText = "ERROR"
		}

		uptime := formatUptime(svc.StartTime)

		displayName := svc.Name
		if len(displayName) > maxNameLen {
			displayName = displayName[:maxNameLen-3] + "..."
		}
		name := fmt.Sprintf("%-*s", maxNameLen, displayName)
		status := fmt.Sprintf("%s %-*s", statusIcon, statusWidth-2, statusText)
		uptimeStr := fmt.Sprintf("%-*s", uptimeWidth, uptime)
		portStr := fmt.Sprintf("%-*s", portWidth, svc.LocalPort)
		restarts := fmt.Sprintf("%-*d", restartWidth, svc.RestartCount)

		nameColor := colorText
		if selected {
			nameColor = colorAccent
		}
		styledName := lipgloss.NewStyle().
			Foreground(nameColor).
			Bold(true).
			Render(name)

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
		indicator := fmt.Sprintf("↑ %d above • ↓ %d below  (%d–%d of %d)", above, below, start+1, end, len(services))
		rows = append(rows, lipgloss.NewStyle().
			Foreground(colorMuted).
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

func (u *UI) renderAddServiceOverlay() string {
	width := u.width
	if width <= 0 {
		width = 120
	}
	if width < 60 {
		width = 60
	}

	running := u.runningNameSet()

	maxNameLen := 7
	for _, serviceName := range u.addCandidates {
		nameLen := len(serviceName)
		if nameLen > maxNameLen {
			maxNameLen = nameLen
		}
	}
	if maxNameLen > 30 {
		maxNameLen = 30
	}

	rows := make([]string, 0, len(u.addCandidates)+3)
	headerName := fmt.Sprintf("%-*s", maxNameLen, "SERVICE")
	header := lipgloss.NewStyle().
		Foreground(colorHeading).
		Bold(true).
		Render(headerName + "  SELECT")
	rows = append(rows, header)

	sepWidth := width - 6
	if sepWidth < 50 {
		sepWidth = 50
	}
	if sepWidth > 200 {
		sepWidth = 200
	}
	rows = append(rows, lipgloss.NewStyle().Foreground(colorBorder).Render(strings.Repeat("─", sepWidth)))

	if len(u.addCandidates) == 0 {
		rows = append(rows, lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true).
			Render("No services yet — press 'n' to create one"))
	}

	for i, serviceName := range u.addCandidates {
		selected := i == u.addCursor
		highlight := "  "
		if selected {
			highlight = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("► ")
		}

		displayName := serviceName
		if len(displayName) > maxNameLen {
			displayName = displayName[:maxNameLen-3] + "..."
		}
		name := fmt.Sprintf("%-*s", maxNameLen, displayName)
		nameColor := colorText
		if selected {
			nameColor = colorAccent
		}
		styledName := lipgloss.NewStyle().
			Foreground(nameColor).
			Bold(true).
			Render(name)

		var marker string
		if running[serviceName] {
			marker = lipgloss.NewStyle().
				Foreground(colorAccentAlt).
				Render(fmt.Sprintf("%-7s", "running"))
		} else {
			checkbox := "[ ]"
			if u.addSelected != nil && u.addSelected[serviceName] {
				checkbox = "[✓]"
			}
			marker = lipgloss.NewStyle().
				Foreground(colorMuted).
				Render(fmt.Sprintf("%-7s", checkbox))
		}

		rows = append(rows, highlight+styledName+"  "+marker)
	}

	table := lipgloss.JoinVertical(lipgloss.Left, rows...)
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(width - 2)

	overlayBox := style.Render(table)

	instructions := "↑↓:navigate • Space:select • Enter:run • n:new • e:edit • c:config in editor • Esc:cancel"
	instructionStyled := lipgloss.NewStyle().
		Foreground(colorMuted).
		Render(instructions)

	return lipgloss.JoinVertical(lipgloss.Left, overlayBox, instructionStyled)
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

	instructions := "Tab/↑↓:switch field • Enter:save • Esc:back"
	instructionStyled := lipgloss.NewStyle().
		Foreground(colorMuted).
		Render(instructions)

	return lipgloss.JoinVertical(lipgloss.Left, box, instructionStyled)
}

func renderHelp(width int, logScope string) string {
	if width < 60 {
		width = 60
	}

	keyStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(colorMuted)
	sep := descStyle.Render("  •  ")
	chip := func(k, d string) string {
		return keyStyle.Render(k) + descStyle.Render(" "+d)
	}

	var chips []string
	if width < 90 {
		chips = []string{
			chip("↑↓", "move"),
			chip("l", "logs="+logScope),
			chip("a", "add"),
			chip("c", "config"),
			chip("r", "restart"),
			chip("s", "stop"),
			chip("q", "quit"),
		}
	} else {
		chips = []string{
			chip("↑↓/j/k", "move"),
			chip("l", "logs="+logScope),
			chip("a", "add/edit"),
			chip("c", "config"),
			chip("r", "restart"),
			chip("^r", "restart all"),
			chip("s", "stop"),
			chip("q", "quit"),
		}
	}
	help := strings.Join(chips, sep)

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(width - 2)

	return style.Render(help)
}

func (u *UI) renderShutdownScreen() string {
	frame := spinnerFrames[u.spinnerFrame%len(spinnerFrames)]

	shutdownStyle := lipgloss.NewStyle().
		Foreground(colorAccentAlt).
		Bold(true)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentAlt).
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
