package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"

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

type Controller interface {
	ListServiceStates() []model.Service
	StartStoredService(ctx context.Context, name string) error
	StopService(name string)
	StopAllServices()
	RestartService(ctx context.Context, name string) error
	RestartAllServices(ctx context.Context)
	ClearLogs(serviceName string)
}

type UI struct {
	manager     Controller
	store       *storage.Storage
	services    []model.Service
	cursorIndex int
	quitting    bool
	width       int
	height      int
	viewport    viewport.Model
	ready       bool
	ctx         context.Context

	serviceForm serviceFormState // add/edit service form, launched from the manage overlay
	groupForm   groupFormState   // add/edit group form, launched from the manage overlay
	manage      manageState      // unified manage overlay (groups + services in one list)

	editStatus        string
	editStatusSeq     int
	logFilterSelected bool
	spinnerFrame      int
	tableOffset       int
	aliasEnabled      bool // cluster-host aliasing is on → the copy (y) action is available
	lastLogContent    string
	lastLogVersion    string
}

const uiTickInterval = 500 * time.Millisecond

func NewUI(ctx context.Context, mgr Controller, store *storage.Storage) *UI {
	aliasEnabled, _ := store.HostAliasEnabled()
	return &UI{
		manager:      mgr,
		store:        store,
		services:     []model.Service{},
		ctx:          ctx,
		aliasEnabled: aliasEnabled,
	}
}

// normalizeKeyToken maps a key message to the canonical token used by the
// keymap switches. "space" is kept verbatim so it stays distinguishable from a
// literal space rune typed into search or text inputs.
func normalizeKeyToken(msg tea.KeyMsg) string {
	raw := msg.String()
	if raw == "space" {
		return raw
	}
	return stringutil.NormalizeToken(raw)
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

		if u.manage.active && u.serviceForm.mode != "" {
			inputWidth := u.formInputWidth()
			u.serviceForm.nameInput.SetWidth(inputWidth)
			u.serviceForm.commandInput.SetWidth(inputWidth)
		}
		if u.manage.active && u.groupForm.mode != "" {
			u.groupForm.nameInput.SetWidth(u.formInputWidth())
		}

	case tea.MouseWheelMsg:
		switch {
		case u.manage.active:
			if u.serviceForm.mode == "" && u.groupForm.mode == "" {
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
		key := normalizeKeyToken(msg)
		if u.manage.active {
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

		case "x":
			scope := ""
			label := "all services"
			if u.logFilterSelected && u.cursorIndex >= 0 && u.cursorIndex < len(u.services) {
				scope = u.services[u.cursorIndex].Name
				label = scope
			}
			u.manager.ClearLogs(scope)
			u.services = u.manager.ListServiceStates()
			u.refreshViewportContent()
			return u, u.setStatus("✓ Logs cleared: " + label)

		case "y":
			if u.aliasEnabled && u.cursorIndex < len(u.services) && len(u.services) > 0 {
				return u, u.copyAlias(u.services[u.cursorIndex])
			}

		default:
			u.viewport, cmd = u.viewport.Update(msg)
		}

	case editResultMsg:
		var status string
		switch {
		case msg.ok:
			status = fmt.Sprintf("✓ Config saved: %d service(s), %d group(s) — affects future runs", msg.services, msg.groups)
			if u.manage.active && u.serviceForm.mode == "" && u.groupForm.mode == "" {
				u.reloadManageRowsFromStorage()
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
		if u.manage.active {
			return u.forwardOverlayInput(msg)
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

// copyAlias yanks the selected service's cluster-host alias to the system
// clipboard, reporting the result in the status line.
func (u *UI) copyAlias(svc model.Service) tea.Cmd {
	if svc.Alias == "" {
		return u.setStatus("○ No cluster alias for " + svc.Name + " — enable it with 'pf alias on'")
	}
	if err := clipboard.WriteAll(svc.Alias); err != nil {
		return u.setStatus("✗ Copy failed: " + err.Error())
	}
	return u.setStatus("✓ Copied " + svc.Alias)
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

	if u.manage.active {
		if u.serviceForm.mode != "" {
			return u.renderServiceForm()
		}
		if u.groupForm.mode != "" {
			return u.renderGroupForm()
		}
		return u.renderManageOverlay()
	}

	u.ensureViewportSize()

	sections := make([]string, 0, 3)
	if len(u.services) == 0 {
		sections = append(sections, renderEmptyState())
	} else {
		maxVis := maxVisibleServices(len(u.services), u.height, u.chromeBelowLog())
		u.ensureCursorVisible(maxVis)
		sections = append(sections, renderServiceTable(u.services, u.cursorIndex, u.tableOffset, maxVis, u.width))
	}

	logBoxWidth := u.width
	if logBoxWidth < 1 {
		logBoxWidth = 1
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

	sections = append(sections, renderHelp(u.width, u.logScopeLabel(), u.aliasEnabled))
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
