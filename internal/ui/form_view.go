package ui

import (
	"fmt"
	"strings"

	"github.com/alinemone/go-port-forward/internal/kube"
)

// The Configure screens: the service and group forms. Rendering only — keys
// and state live in service_form.go and group_form.go.

// formLabel is a field label: bright with a ▸ when focused, with a short hint.
func formLabel(label string, focused bool, hint string) string {
	if focused {
		return fg(colorAccent).Bold(true).Render("▸ "+label) + fg(colorMuted).Render("   "+hint)
	}
	return fg(colorMuted).Render("  "+label) + fg(colorBorder).Render("   "+hint)
}

// formInput frames a text input with a rail that lights up on focus.
func formInput(view string, focused bool) string {
	rail := fg(colorBorder).Render("  ┃ ")
	if focused {
		rail = fg(colorAccent).Render("  ┃ ")
	}
	return rail + view
}

// formScreen lays a Configure screen out: header, the form frame (topped up
// with starfield so it always fills the screen), and the key bar.
func (u *UI) formScreen(page, headerRight string, body []string, keys [][2]string) string {
	width := max(40, u.width)
	height := u.height
	if height <= 0 {
		height = 24
	}
	inner := max(1, width-2*len(margin))
	keyLines := keyBar(keys, nil, inner, "")
	frameH := max(4, height-2-len(keyLines))

	if room := frameH - 2 - len(body) - 1; room > 0 {
		body = append(append(body, ""), starfield(width-4, room, 11, u.spinnerFrame, "")...)
	}

	lines := []string{u.hudHeader(page, headerRight), ""}
	lines = append(lines, hudFrame(hudTitle("CONFIGURE", ""), "", body, width, frameH, toneNormal)...)
	lines = append(lines, keyLines...)
	return screen(lines, width, height)
}

func formError(msg string) []string {
	if msg == "" {
		return nil
	}
	return []string{"", fg(colorError).Bold(true).Render("✗ " + msg)}
}

// ── service form ────────────────────────────────────────────────────────────

func (u *UI) renderServiceForm() string {
	page, right := "NEW SERVICE", ""
	if u.serviceForm.mode == "edit" {
		page = "EDIT SERVICE"
		right = fg(colorMuted).Render(u.serviceForm.originalName)
	}
	nameFocused := u.serviceForm.focusedField == 0

	body := []string{
		"",
		formLabel("NAME", nameFocused, "letters, digits, - and _"),
		formInput(u.serviceForm.nameInput.View(), nameFocused),
		"",
		formLabel("COMMAND", !nameFocused, "a kubectl port-forward command"),
		formInput(u.serviceForm.commandInput.View(), !nameFocused),
		"",
		commandPreview(u.serviceForm.commandInput.Value()),
	}
	body = append(body, formError(u.serviceForm.errorMsg)...)

	return u.formScreen(page, right, body, [][2]string{
		{"Tab/↑↓", "switch field"},
		{"Enter", "save"},
		{"Esc", "back"},
	})
}

// commandPreview reads the command back as you type: where it will forward, or
// what is still wrong — so a typo shows up before saving, not after launch.
func commandPreview(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return fg(colorBorder).Render("  ⋯ ") + fg(colorMuted).Italic(true).Render("e.g. kubectl -n prod port-forward svc/postgres 5432:5432")
	}
	spec, err := kube.ParseCommand(command)
	if err != nil {
		return fg(colorWarn).Render("  ! " + err.Error())
	}

	where := spec.Target()
	if spec.Namespace != "" {
		where += fg(colorMuted).Render(" in ") + spec.Namespace
	}
	if spec.Context != "" {
		where += fg(colorMuted).Render(" @ ") + spec.Context
	}
	ports := make([]string, 0, len(spec.Ports))
	for _, p := range spec.Ports {
		ports = append(ports, fmt.Sprintf("localhost:%d ⇢ %s", p.Local, p.Remote))
	}
	return fg(statusHealthyColor).Render("  ✓ ") + fg(colorText).Render(where) +
		fg(colorBorder).Render("   ──▸   ") + fg(colorText).Render(strings.Join(ports, ", "))
}

// ── group form ──────────────────────────────────────────────────────────────

func (u *UI) renderGroupForm() string {
	page, right := "NEW GROUP", ""
	if u.groupForm.mode == "edit" {
		page = "EDIT GROUP"
		right = fg(colorMuted).Render(u.groupForm.originalName)
	}
	nameFocused := u.groupForm.focusedField == 0

	selected := 0
	for _, on := range u.groupForm.selected {
		if on {
			selected++
		}
	}

	body := []string{
		"",
		formLabel("NAME", nameFocused, "letters, digits, - and _"),
		formInput(u.groupForm.nameInput.View(), nameFocused),
		"",
		formLabel("SERVICES", !nameFocused, fmt.Sprintf("%d selected", selected)),
	}

	names := u.groupForm.serviceNames
	if len(names) == 0 {
		body = append(body, "  "+fg(colorMuted).Italic(true).Render("no services yet — create services first"))
	} else {
		// The list gets whatever height the screen leaves.
		height := u.height
		if height <= 0 {
			height = 24
		}
		listRows := max(3, height-2-1-2-len(body)-len(formError(u.groupForm.errorMsg))-1)
		if len(names) > listRows {
			listRows-- // range line
		}
		start := 0
		if u.groupForm.serviceCursor >= listRows {
			start = u.groupForm.serviceCursor - listRows + 1
		}
		end := min(len(names), start+listRows)

		for i := start; i < end; i++ {
			svc := names[i]
			onCursor := !nameFocused && i == u.groupForm.serviceCursor
			mark := fg(colorMuted).Render("○")
			if u.groupForm.selected[svc] {
				mark = fg(colorAccentAlt).Bold(true).Render("◉")
			}
			nameStyle := fg(colorText)
			if onCursor {
				nameStyle = nameStyle.Foreground(colorAccent).Bold(true)
			}
			body = append(body, u.cursorBar(onCursor)+mark+" "+nameStyle.Render(svc))
		}
		if len(names) > listRows {
			body = append(body, "  "+fg(colorMuted).Render(fmt.Sprintf("%d–%d of %d", start+1, end, len(names))))
		}
	}
	body = append(body, formError(u.groupForm.errorMsg)...)

	return u.formScreen(page, right, body, [][2]string{
		{"Tab", "switch field"},
		{"↑↓", "navigate"},
		{"Space", "toggle"},
		{"Enter", "save"},
		{"Esc", "back"},
	})
}
