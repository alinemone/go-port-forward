package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/manager"
	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/stringutil"
)

func (u *UI) openNewGroupForm() tea.Cmd {
	names, err := storage.NewStorage().ListServiceNames()
	if err != nil {
		return nil
	}
	u.groupFormMode = "new"
	u.groupFormOrig = ""
	u.groupFormErr = ""
	u.groupFormName = newServiceTextInput("e.g. backend", "", u.formInputWidth())
	u.groupFormServices = names
	u.groupFormSelected = make(map[string]bool)
	u.groupFormFocus = 0
	u.groupFormSvcCursor = 0
	return u.groupFormName.Focus()
}

func (u *UI) openEditGroupFormFor(name string) tea.Cmd {
	names, err := storage.NewStorage().ListServiceNames()
	if err != nil {
		return nil
	}
	u.groupFormMode = "edit"
	u.groupFormOrig = name
	u.groupFormErr = ""
	u.groupFormName = newServiceTextInput("group name", name, u.formInputWidth())
	u.groupFormServices = names
	u.groupFormSelected = make(map[string]bool)
	for _, svc := range u.manageGroups[name] {
		u.groupFormSelected[svc] = true
	}
	u.groupFormFocus = 0
	u.groupFormSvcCursor = 0
	return u.groupFormName.Focus()
}

func (u *UI) closeGroupForm() {
	u.groupFormMode = ""
	u.groupFormErr = ""
	u.groupFormName.Blur()
}

func (u *UI) toggleGroupFormFocus() tea.Cmd {
	if u.groupFormFocus == 0 {
		u.groupFormFocus = 1
		u.groupFormName.Blur()
		return nil
	}
	u.groupFormFocus = 0
	return u.groupFormName.Focus()
}

func (u *UI) updateGroupForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if paste, ok := msg.(tea.PasteMsg); ok {
		return u.updateGroupNameInput(paste)
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return u.updateGroupNameInput(msg)
	}

	keyRaw := keyMsg.String()
	key := keyRaw
	if keyRaw != "space" {
		key = stringutil.NormalizeToken(keyRaw)
	}

	switch key {
	case "esc":
		u.closeGroupForm()
		return u, nil
	case "tab", "shift+tab":
		return u, u.toggleGroupFormFocus()
	case "enter":
		return u.submitGroupForm()
	}

	if u.groupFormFocus == 1 {
		switch key {
		case "up", "k":
			if u.groupFormSvcCursor > 0 {
				u.groupFormSvcCursor--
			}
		case "down", "j":
			if u.groupFormSvcCursor < len(u.groupFormServices)-1 {
				u.groupFormSvcCursor++
			}
		case "space":
			if u.groupFormSvcCursor >= 0 && u.groupFormSvcCursor < len(u.groupFormServices) {
				svc := u.groupFormServices[u.groupFormSvcCursor]
				u.groupFormSelected[svc] = !u.groupFormSelected[svc]
			}
		}
		return u, nil
	}

	return u.updateGroupNameInput(msg)
}

func (u *UI) updateGroupNameInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	u.groupFormName, cmd = u.groupFormName.Update(msg)
	return u, cmd
}

func (u *UI) submitGroupForm() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(u.groupFormName.Value())
	if err := manager.ValidateServiceName(name); err != nil {
		u.groupFormErr = err.Error()
		return u, nil
	}

	selected := make([]string, 0, len(u.groupFormSelected))
	for _, svc := range u.groupFormServices {
		if u.groupFormSelected[svc] {
			selected = append(selected, svc)
		}
	}

	st := storage.NewStorage()
	var status string

	switch u.groupFormMode {
	case "new":
		if _, exists := u.manageGroups[name]; exists {
			u.groupFormErr = fmt.Sprintf("a group named '%s' already exists", name)
			return u, nil
		}
		if err := st.AddGroup(name, selected); err != nil {
			u.groupFormErr = err.Error()
			return u, nil
		}
		status = fmt.Sprintf("✓ Group '%s' created with %d service(s)", name, len(selected))

	case "edit":
		orig := u.groupFormOrig
		if name != orig {
			if err := st.RenameGroup(orig, name); err != nil {
				u.groupFormErr = err.Error()
				return u, nil
			}
		}
		// AddGroup overwrites the membership of an existing group.
		if err := st.AddGroup(name, selected); err != nil {
			u.groupFormErr = err.Error()
			return u, nil
		}
		status = fmt.Sprintf("✓ Group '%s' updated (%d service(s))", name, len(selected))
	}

	u.closeGroupForm()
	u.manageSearch = "" // ensure the saved group is visible regardless of any active filter
	u.buildManageRows()
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
	if u.groupFormMode == "edit" {
		title = fmt.Sprintf("Edit group: %s", u.groupFormOrig)
	}
	titleStyled := lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Render(title)

	labelStyle := lipgloss.NewStyle().Foreground(colorMuted)
	activeLabel := lipgloss.NewStyle().Foreground(colorAccentAlt).Bold(true)

	nameLabel := labelStyle.Render("  Name:")
	servicesLabel := labelStyle.Render("  Services:")
	if u.groupFormFocus == 0 {
		nameLabel = activeLabel.Render("► Name:")
	} else {
		servicesLabel = activeLabel.Render("► Services:")
	}

	rows := []string{
		titleStyled,
		"",
		nameLabel,
		"  " + u.groupFormName.View(),
		"",
		servicesLabel,
	}

	if len(u.groupFormServices) == 0 {
		rows = append(rows, lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true).
			Render("  No services available — create services first"))
	} else {
		const maxVisible = 20
		start := 0
		if u.groupFormSvcCursor >= maxVisible {
			start = u.groupFormSvcCursor - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(u.groupFormServices) {
			end = len(u.groupFormServices)
		}

		for i := start; i < end; i++ {
			svc := u.groupFormServices[i]
			onCursor := u.groupFormFocus == 1 && i == u.groupFormSvcCursor
			marker := "  "
			if onCursor {
				marker = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("► ")
			}
			checkbox := "[ ]"
			if u.groupFormSelected[svc] {
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

		if len(u.groupFormServices) > maxVisible {
			rows = append(rows, lipgloss.NewStyle().
				Foreground(colorMuted).
				Render(fmt.Sprintf("  (%d–%d of %d)", start+1, end, len(u.groupFormServices))))
		}
	}

	if u.groupFormErr != "" {
		rows = append(rows, "", lipgloss.NewStyle().Foreground(colorError).Render("✗ "+u.groupFormErr))
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
