package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// helpSection prints a blank line then a bold, uppercased section header marked
// with an accent "▸". An optional hint (e.g. "pf group <sub>") is shown dim so
// the reader knows the prefix the rows below share. Used by the per-command
// helps (pf group, pf cert, pf update, pf completion).
func helpSection(title, hint string) {
	lipgloss.Println()
	line := "  " + cliArrow.Render("▸") + " " + cliHeading.Render(strings.ToUpper(title))
	if hint != "" {
		line += "  " + cliMuted.Render(hint)
	}
	lipgloss.Println(line)
}

// helpRow prints one aligned "name  description" row so descriptions line up.
func helpRow(name, desc string) {
	lipgloss.Printf("    %s  %s\n", cliName.Render(fmt.Sprintf("%-28s", name)), cliDetail.Render(desc))
}

// helpExample prints "pf <cmd>" with an optional trailing comment.
func helpExample(cmd, note string) {
	line := "    " + cliName.Render("pf ") + cliDetail.Render(cmd)
	if note != "" {
		line += "  " + cliMuted.Render("# "+note)
	}
	lipgloss.Println(line)
}

// helpNote prints a dim bullet line for tips/notes.
func helpNote(text string) {
	lipgloss.Println("    " + cliMuted.Render("• "+text))
}

// ── Main `pf help` screen ───────────────────────────────────────────────────
//
// showUsage is intentionally plain: only fmt with ASCII text — no colors,
// Unicode symbols, or box drawing — so it renders identically on every terminal,
// including legacy Windows consoles where styled output showed up as raw escape
// codes or empty boxes. It is split into clear sections (services, groups,
// certificate, kubectl); each command shows its shortcut inline (e.g. "a, add")
// and every section carries a short example.

// uHead prints a section header preceded by a blank line.
func uHead(title string) {
	fmt.Println()
	fmt.Println(title)
}

// uRow prints a "  command   description" line, padding the command to width w
// so the descriptions in a section line up.
func uRow(w int, cmd, desc string) {
	fmt.Printf("  %-*s  %s\n", w, cmd, desc)
}

// uExample prints an indented "Example:" block of "pf ..." lines.
func uExample(lines ...string) {
	fmt.Println("  Example:")
	for _, l := range lines {
		fmt.Println("    pf " + l)
	}
}

func showUsage() {
	fmt.Println()
	fmt.Println("pf - Port Forward Manager")
	fmt.Println("Forward remote ports (kubectl / ssh) to your machine and keep them alive.")

	uHead("USAGE:")
	fmt.Println("  pf <command> [arguments]")
	uRow(26, "pf <name>", "Shortcut for: pf run <name>  (service or group)")

	uHead("SERVICES:")
	uRow(27, `a, add <name> "<command>"`, "Add a new service")
	uRow(27, "l, list", "List all saved services")
	uRow(27, "r, run <names>", "Run one or more services in the live view (comma-separated)")
	uRow(27, "ra, run all", "Run every saved service")
	uRow(27, "d, delete <name>", "Delete a service")
	uRow(27, "rename <old> <new>", "Rename a service")
	uExample(`add db "kubectl port-forward service/postgres 5432:5432"`, "run db,redis")

	uHead("GROUPS:")
	uRow(39, "g, group add <name> <svcs>", "Create a group from services")
	uRow(39, "g, group add-service <name> <svcs>", "Add services to a group")
	uRow(39, "g, group remove-service <name> <svcs>", "Remove services from a group")
	uRow(39, "g, group list", "List all groups and their members")
	uRow(39, "g, group rename <old> <new>", "Rename a group")
	uRow(39, "g, group delete <name>", "Delete a group (services are kept)")
	uExample("group add backend api,db,redis", "run backend")

	uHead("CERTIFICATE:")
	uRow(22, "cert add <p12-file>", "Add a client certificate (used for all kubectl)")
	uRow(22, "cert list", "Show the configured certificate")
	uRow(22, "cert remove", "Remove the certificate")
	uExample("cert add company-vpn.p12")

	uHead("KUBECTL:")
	uRow(22, "k, kubectl <args...>", "Run kubectl with the configured certificate")
	uExample("k get pods -n production", "k logs deploy/api -f")

	uHead("OTHER:")
	uRow(26, "c, cleanup [--all]", "Free configured ports (--all kills all kubectl/ssh)")
	uRow(26, "edit", "Edit all services and groups as JSON")
	uRow(26, "theme [name|list]", "Change the color theme")
	uRow(26, "icon [on|off|status]", "Toggle service icons")
	uRow(26, "completion install", "Install shell tab-completion")
	uRow(26, "u, update [--yes|--force]", "Update pf to the latest release")
	uRow(26, "v, version", "Show the installed version")
	uRow(26, "h, help", "Show this help")

	fmt.Println()
	fmt.Println("https://github.com/alinemone/go-port-forward")
	fmt.Println()
}
