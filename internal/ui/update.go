package ui

import (
	"sort"
	"time"

	"github.com/alinemone/go-port-forward/internal/service"
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles messages and updates the model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		// Regular tick - update services and check errors
		return m, tea.Batch(
			m.tickCmd(),
			m.updateServicesCmd(),
			m.checkErrorClearCmd(),
		)

	case servicesUpdateMsg:
		m.updateServiceStates([]service.State(msg))
		return m, nil

	case errorClearMsg:
		m.clearError(msg.serviceName)
		return m, nil
	}

	return m, nil
}

func (m *Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		m.manager.StopAll()
		return m, tea.Quit

	case "r":
		// Manual refresh
		return m, m.updateServicesCmd()
	}

	return m, nil
}

func (m *Model) updateServiceStates(states []service.State) {
	// Sort by name
	sort.Slice(states, func(i, j int) bool {
		return states[i].Name < states[j].Name
	})

	// Update services
	m.services = states

	// Update errors
	now := time.Now()
	for _, state := range states {
		// Check if this service has a new error
		if state.LastError != "" && !state.ErrorTime.IsZero() {
			// Check if error already exists
			exists := false
			for i, err := range m.errors {
				if err.Service == state.Name {
					exists = true
					// Update existing error
					if err.Timestamp != state.ErrorTime {
						m.errors[i] = ErrorEntry{
							Service:   state.Name,
							Message:   state.LastError,
							Timestamp: state.ErrorTime,
							ClearAt:   time.Time{}, // Don't auto-clear yet
						}
					}
					break
				}
			}

			if !exists {
				// Add new error
				m.errors = append(m.errors, ErrorEntry{
					Service:   state.Name,
					Message:   state.LastError,
					Timestamp: state.ErrorTime,
					ClearAt:   time.Time{},
				})
			}
		}

		// If service is now online and has an error, schedule it for clearing
		if state.Status == service.StatusOnline {
			for i, err := range m.errors {
				if err.Service == state.Name && err.ClearAt.IsZero() {
					// Schedule for clearing
					m.errors[i].ClearAt = now.Add(m.config.ErrorAutoClearDelay)
				}
			}
		}
	}

	// Remove old errors (older than 30 seconds with no clear scheduled)
	filtered := make([]ErrorEntry, 0, len(m.errors))
	for _, err := range m.errors {
		age := now.Sub(err.Timestamp)
		if age < 30*time.Second || !err.ClearAt.IsZero() {
			filtered = append(filtered, err)
		}
	}
	m.errors = filtered
}

func (m *Model) clearError(serviceName string) {
	filtered := make([]ErrorEntry, 0, len(m.errors))
	for _, err := range m.errors {
		if err.Service != serviceName {
			filtered = append(filtered, err)
		}
	}
	m.errors = filtered
}
