package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/model"
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

func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
