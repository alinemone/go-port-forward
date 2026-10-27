package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/alinemone/go-port-forward/internal/model"
	"github.com/alinemone/go-port-forward/internal/storage"
)

// idleController stands in for the forward manager when the discovery wizard
// runs on its own (`pf discover`): nothing is running and nothing starts.
type idleController struct{}

func (idleController) ListServiceStates() []model.Service               { return nil }
func (idleController) StartStoredService(context.Context, string) error { return nil }
func (idleController) StopService(string)                               {}
func (idleController) StopAllServices()                                 {}
func (idleController) RestartService(context.Context, string) error     { return nil }
func (idleController) RestartAllServices(context.Context)               {}
func (idleController) ClearLogs(string)                                 {}

// RunDiscover runs the discovery wizard full-screen and returns the names of
// the services it saved (nil when cancelled). A namespace skips the first step.
func RunDiscover(store *storage.Storage, namespace string) ([]string, error) {
	u := NewUI(context.Background(), idleController{}, store)
	start := u.openDiscover(namespace, true)
	model, err := tea.NewProgram(&discoverProgram{UI: u, start: start}).Run()
	if err != nil {
		return nil, err
	}
	return model.(*discoverProgram).discover.added, nil
}

// discoverProgram is the UI with the wizard's first commands run at start.
type discoverProgram struct {
	*UI
	start tea.Cmd
}

func (p *discoverProgram) Init() tea.Cmd {
	return tea.Batch(p.UI.Init(), p.start)
}

func (p *discoverProgram) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := p.UI.Update(msg)
	return p, cmd
}
