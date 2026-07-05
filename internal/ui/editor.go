package ui

import (
	"encoding/json"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/alinemone/go-port-forward/internal/configedit"
	"github.com/alinemone/go-port-forward/internal/storage"
)

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
