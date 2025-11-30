package ui

import (
	"time"

	"github.com/alinemone/go-port-forward/internal/config"
	"github.com/alinemone/go-port-forward/internal/service"
	tea "github.com/charmbracelet/bubbletea"
)

// tickMsg is sent on every tick to update the UI.
type tickMsg time.Time

// servicesUpdateMsg contains updated service states.
type servicesUpdateMsg []service.State

// errorClearMsg indicates an error should be cleared.
type errorClearMsg struct {
	serviceName string
}

// ErrorEntry represents an error to display.
type ErrorEntry struct {
	Service   string
	Message   string
	Timestamp time.Time
	ClearAt   time.Time // When to auto-clear
}

// Model is the Bubbletea model for the UI.
type Model struct {
	manager *service.Manager
	config  *config.Config
	styles  *Styles

	services []service.State
	errors   []ErrorEntry

	quitting bool
	width    int
	height   int
}

// New creates a new UI model.
func New(manager *service.Manager, cfg *config.Config) *Model {
	return &Model{
		manager:  manager,
		config:   cfg,
		styles:   NewStyles(),
		services: []service.State{},
		errors:   []ErrorEntry{},
	}
}

// Init initializes the model.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.tickCmd(),
		tea.EnterAltScreen,
	)
}

// tickCmd returns a command that sends tick messages.
func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(m.config.UIRefreshRate, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// updateServicesCmd returns a command that fetches service states.
func (m *Model) updateServicesCmd() tea.Cmd {
	return func() tea.Msg {
		states := m.manager.GetStates()
		return servicesUpdateMsg(states)
	}
}

// checkErrorClearCmd checks if any errors should be cleared.
func (m *Model) checkErrorClearCmd() tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		for i := len(m.errors) - 1; i >= 0; i-- {
			if !m.errors[i].ClearAt.IsZero() && now.After(m.errors[i].ClearAt) {
				return errorClearMsg{serviceName: m.errors[i].Service}
			}
		}
		return nil
	}
}
