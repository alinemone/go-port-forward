package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/manager"
)

// groupFormState is the state of the add/edit-group form: the group name
// input, the selectable list of member services, keyboard focus, and the last
// validation error.
type groupFormState struct {
	mode          string // "" = closed, "new", "edit"
	originalName  string // name before editing (rename detection)
	nameInput     textinput.Model
	errorMsg      string
	focusedField  int      // 0 = name, 1 = services list
	serviceNames  []string // all services offered for membership
	selected      map[string]bool
	serviceCursor int
}

func (u *UI) openNewGroupForm() tea.Cmd {
	names, err := u.store.ListServiceNames()
	if err != nil {
		return nil
	}
	u.groupForm.mode = "new"
	u.groupForm.originalName = ""
	u.groupForm.errorMsg = ""
	u.groupForm.nameInput = newServiceTextInput("e.g. backend", "", u.formInputWidth())
	u.groupForm.serviceNames = names
	u.groupForm.selected = make(map[string]bool)
	u.groupForm.focusedField = 0
	u.groupForm.serviceCursor = 0
	return u.groupForm.nameInput.Focus()
}

func (u *UI) openEditGroupFormFor(name string) tea.Cmd {
	names, err := u.store.ListServiceNames()
	if err != nil {
		return nil
	}
	u.groupForm.mode = "edit"
	u.groupForm.originalName = name
	u.groupForm.errorMsg = ""
	u.groupForm.nameInput = newServiceTextInput("group name", name, u.formInputWidth())
	u.groupForm.serviceNames = names
	u.groupForm.selected = make(map[string]bool)
	for _, svc := range u.manage.groups[name] {
		u.groupForm.selected[svc] = true
	}
	u.groupForm.focusedField = 0
	u.groupForm.serviceCursor = 0
	return u.groupForm.nameInput.Focus()
}

func (u *UI) closeGroupForm() {
	u.groupForm.mode = ""
	u.groupForm.errorMsg = ""
	u.groupForm.nameInput.Blur()
}

func (u *UI) toggleGroupFormFocus() tea.Cmd {
	if u.groupForm.focusedField == 0 {
		u.groupForm.focusedField = 1
		u.groupForm.nameInput.Blur()
		return nil
	}
	u.groupForm.focusedField = 0
	return u.groupForm.nameInput.Focus()
}

func (u *UI) updateGroupForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if paste, ok := msg.(tea.PasteMsg); ok {
		return u.forwardGroupFormInput(paste)
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return u.forwardGroupFormInput(msg)
	}

	key := normalizeKeyToken(keyMsg)

	switch key {
	case "esc":
		u.closeGroupForm()
		return u, nil
	case "tab", "shift+tab":
		return u, u.toggleGroupFormFocus()
	case "enter":
		return u.submitGroupForm()
	}

	if u.groupForm.focusedField == 1 {
		switch key {
		case "up", "k":
			if u.groupForm.serviceCursor > 0 {
				u.groupForm.serviceCursor--
			}
		case "down", "j":
			if u.groupForm.serviceCursor < len(u.groupForm.serviceNames)-1 {
				u.groupForm.serviceCursor++
			}
		case "space":
			if u.groupForm.serviceCursor >= 0 && u.groupForm.serviceCursor < len(u.groupForm.serviceNames) {
				svc := u.groupForm.serviceNames[u.groupForm.serviceCursor]
				u.groupForm.selected[svc] = !u.groupForm.selected[svc]
			}
		}
		return u, nil
	}

	return u.forwardGroupFormInput(msg)
}

func (u *UI) forwardGroupFormInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	u.groupForm.nameInput, cmd = u.groupForm.nameInput.Update(msg)
	return u, cmd
}

func (u *UI) submitGroupForm() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(u.groupForm.nameInput.Value())
	if err := manager.ValidateServiceName(name); err != nil {
		u.groupForm.errorMsg = err.Error()
		return u, nil
	}

	selected := make([]string, 0, len(u.groupForm.selected))
	for _, svc := range u.groupForm.serviceNames {
		if u.groupForm.selected[svc] {
			selected = append(selected, svc)
		}
	}

	st := u.store
	var status string

	switch u.groupForm.mode {
	case "new":
		if _, exists := u.manage.groups[name]; exists {
			u.groupForm.errorMsg = fmt.Sprintf("a group named '%s' already exists", name)
			return u, nil
		}
		if err := st.AddGroup(name, selected); err != nil {
			u.groupForm.errorMsg = err.Error()
			return u, nil
		}
		status = fmt.Sprintf("✓ Group '%s' created with %d service(s)", name, len(selected))

	case "edit":
		orig := u.groupForm.originalName
		if name != orig {
			if err := st.RenameGroup(orig, name); err != nil {
				u.groupForm.errorMsg = err.Error()
				return u, nil
			}
		}
		// AddGroup overwrites the membership of an existing group.
		if err := st.AddGroup(name, selected); err != nil {
			u.groupForm.errorMsg = err.Error()
			return u, nil
		}
		status = fmt.Sprintf("✓ Group '%s' updated (%d service(s))", name, len(selected))
	}

	u.closeGroupForm()
	u.manage.searchQuery = "" // ensure the saved group is visible regardless of any active filter
	u.reloadManageRowsFromStorage()
	u.focusManage(rowGroup, name)
	return u, u.setStatus(status)
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
	if u.groupForm.mode == "edit" {
		title = fmt.Sprintf("Edit group: %s", u.groupForm.originalName)
	}
	titleStyled := lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Render(title)

	labelStyle := lipgloss.NewStyle().Foreground(colorMuted)
	activeLabel := lipgloss.NewStyle().Foreground(colorAccentAlt).Bold(true)

	nameLabel := labelStyle.Render("  Name:")
	servicesLabel := labelStyle.Render("  Services:")
	if u.groupForm.focusedField == 0 {
		nameLabel = activeLabel.Render("► Name:")
	} else {
		servicesLabel = activeLabel.Render("► Services:")
	}

	rows := []string{
		titleStyled,
		"",
		nameLabel,
		"  " + u.groupForm.nameInput.View(),
		"",
		servicesLabel,
	}

	if len(u.groupForm.serviceNames) == 0 {
		rows = append(rows, lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true).
			Render("  No services available — create services first"))
	} else {
		const maxVisible = 20
		start := 0
		if u.groupForm.serviceCursor >= maxVisible {
			start = u.groupForm.serviceCursor - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(u.groupForm.serviceNames) {
			end = len(u.groupForm.serviceNames)
		}

		for i := start; i < end; i++ {
			svc := u.groupForm.serviceNames[i]
			onCursor := u.groupForm.focusedField == 1 && i == u.groupForm.serviceCursor
			marker := "  "
			if onCursor {
				marker = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("► ")
			}
			checkbox := "[ ]"
			if u.groupForm.selected[svc] {
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

		if len(u.groupForm.serviceNames) > maxVisible {
			rows = append(rows, lipgloss.NewStyle().
				Foreground(colorMuted).
				Render(fmt.Sprintf("  (%d–%d of %d)", start+1, end, len(u.groupForm.serviceNames))))
		}
	}

	if u.groupForm.errorMsg != "" {
		rows = append(rows, "", lipgloss.NewStyle().Foreground(colorError).Render("✗ "+u.groupForm.errorMsg))
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
