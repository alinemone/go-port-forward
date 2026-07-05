package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/manager"
)

// serviceFormState is the state of the add/edit-service form: which mode it is
// in, the two text inputs, keyboard focus, and the last validation error.
type serviceFormState struct {
	mode         string // "" = closed, "new", "edit"
	originalName string // name before editing (rename detection)
	nameInput    textinput.Model
	commandInput textinput.Model
	focusedField int // 0 = name, 1 = command
	errorMsg     string
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
	u.serviceForm.mode = "new"
	u.serviceForm.originalName = ""
	u.serviceForm.errorMsg = ""
	inputWidth := u.formInputWidth()
	u.serviceForm.nameInput = newServiceTextInput("e.g. db", "", inputWidth)
	u.serviceForm.commandInput = newServiceTextInput("e.g. kubectl port-forward service/postgres 5432:5432", "", inputWidth)
	u.serviceForm.focusedField = 0
	u.serviceForm.commandInput.Blur()
	return u.serviceForm.nameInput.Focus()
}

func (u *UI) openEditServiceFormFor(name string) tea.Cmd {
	command, err := u.store.GetService(name)
	if err != nil {
		return nil
	}
	u.serviceForm.mode = "edit"
	u.serviceForm.originalName = name
	u.serviceForm.errorMsg = ""
	inputWidth := u.formInputWidth()
	u.serviceForm.nameInput = newServiceTextInput("service name", name, inputWidth)
	u.serviceForm.commandInput = newServiceTextInput("command", command, inputWidth)
	u.serviceForm.focusedField = 0
	u.serviceForm.commandInput.Blur()
	return u.serviceForm.nameInput.Focus()
}

func (u *UI) closeServiceForm() {
	u.serviceForm.mode = ""
	u.serviceForm.errorMsg = ""
	u.serviceForm.nameInput.Blur()
	u.serviceForm.commandInput.Blur()
}

func (u *UI) toggleServiceFormFocus() tea.Cmd {
	if u.serviceForm.focusedField == 0 {
		u.serviceForm.focusedField = 1
		u.serviceForm.nameInput.Blur()
		return u.serviceForm.commandInput.Focus()
	}
	u.serviceForm.focusedField = 0
	u.serviceForm.commandInput.Blur()
	return u.serviceForm.nameInput.Focus()
}

func (u *UI) updateServiceForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if paste, ok := msg.(tea.PasteMsg); ok {
		return u.forwardServiceFormInput(paste)
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return u.forwardServiceFormInput(msg)
	}

	key := normalizeKeyToken(keyMsg)

	switch key {
	case "esc":
		u.closeServiceForm()
		return u, nil
	case "tab", "shift+tab", "up", "down":
		return u, u.toggleServiceFormFocus()
	case "enter":
		return u.submitServiceForm()
	}

	return u.forwardServiceFormInput(msg)
}

func (u *UI) forwardServiceFormInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if u.serviceForm.focusedField == 0 {
		u.serviceForm.nameInput, cmd = u.serviceForm.nameInput.Update(msg)
	} else {
		u.serviceForm.commandInput, cmd = u.serviceForm.commandInput.Update(msg)
	}
	return u, cmd
}

func (u *UI) submitServiceForm() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(u.serviceForm.nameInput.Value())
	command := strings.TrimSpace(u.serviceForm.commandInput.Value())

	if err := manager.ValidateServiceName(name); err != nil {
		u.serviceForm.errorMsg = err.Error()
		return u, nil
	}
	if err := manager.ValidateCommand(command); err != nil {
		u.serviceForm.errorMsg = err.Error()
		return u, nil
	}

	st := u.store
	var restartCmd tea.Cmd
	var status string

	switch u.serviceForm.mode {
	case "new":
		if _, err := st.GetService(name); err == nil {
			u.serviceForm.errorMsg = fmt.Sprintf("a service named '%s' already exists", name)
			return u, nil
		}
		if err := st.AddService(name, command); err != nil {
			u.serviceForm.errorMsg = err.Error()
			return u, nil
		}
		status = fmt.Sprintf("✓ Service '%s' created — select it and press Enter to run", name)

	case "edit":
		orig := u.serviceForm.originalName
		wasRunning := u.runningNameSet()[orig]

		if name != orig {
			if err := st.RenameService(orig, name); err != nil {
				u.serviceForm.errorMsg = err.Error()
				return u, nil
			}
		}
		if err := st.AddService(name, command); err != nil {
			u.serviceForm.errorMsg = err.Error()
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

	u.closeServiceForm()
	u.manage.searchQuery = "" // ensure the saved service is visible regardless of any active filter
	u.reloadManageRowsFromStorage()
	u.focusManage(rowService, name)

	statusCmd := u.setStatus(status)
	if restartCmd != nil {
		return u, tea.Batch(restartCmd, statusCmd)
	}
	return u, statusCmd
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
	if u.serviceForm.mode == "edit" {
		title = fmt.Sprintf("Edit service: %s", u.serviceForm.originalName)
	}
	titleStyled := lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Render(title)

	labelStyle := lipgloss.NewStyle().Foreground(colorMuted)
	activeLabel := lipgloss.NewStyle().Foreground(colorAccentAlt).Bold(true)

	nameLabel := labelStyle.Render("  Name:")
	cmdLabel := labelStyle.Render("  Command:")
	if u.serviceForm.focusedField == 0 {
		nameLabel = activeLabel.Render("► Name:")
	} else {
		cmdLabel = activeLabel.Render("► Command:")
	}

	rows := []string{
		titleStyled,
		"",
		nameLabel,
		"  " + u.serviceForm.nameInput.View(),
		"",
		cmdLabel,
		"  " + u.serviceForm.commandInput.View(),
	}

	if u.serviceForm.errorMsg != "" {
		rows = append(rows, "", lipgloss.NewStyle().Foreground(colorError).Render("✗ "+u.serviceForm.errorMsg))
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
