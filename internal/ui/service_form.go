package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/manager"
	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/stringutil"
)

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
