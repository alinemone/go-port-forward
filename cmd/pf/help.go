package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/alinemone/go-port-forward/internal/theme"
)

// helpSection prints a blank line then a bold, uppercased section header marked
// with an accent "▸". An optional hint (e.g. "pf group <sub>") is shown dim so
// the reader knows the prefix the rows below share.
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

// helpEg prints an indented "e.g. pf <cmd>" hint under a command row.
func helpEg(cmd string) {
	lipgloss.Println("        " + cliMuted.Render("e.g. ") + cliName.Render("pf ") + cliDetail.Render(cmd))
}

// helpStep prints a numbered quick-start step: "N. pf <cmd>   → <note>".
func helpStep(n int, cmd, note string) {
	num := cliArrow.Render(fmt.Sprintf("%d.", n))
	body := cliName.Render("pf ") + cliDetail.Render(cmd)
	pad := 54 - lipgloss.Width("pf "+cmd)
	if pad < 2 {
		pad = 2
	}
	lipgloss.Println("    " + num + " " + body + strings.Repeat(" ", pad) + cliMuted.Render("→ "+note))
}

// helpText prints an indented prose line in the normal text color.
func helpText(s string) { lipgloss.Println("    " + cliDetail.Render(s)) }

// helpNote prints a dim bullet line for tips/notes.
func helpNote(text string) {
	lipgloss.Println("    " + cliMuted.Render("• "+text))
}

// helpTitle renders the bordered banner shown at the top of the help.
func helpTitle() string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Active.Border)).
		Padding(0, 2).
		Render(cliTitle.Render("⚡ pf") + cliMuted.Render("  ·  Port Forward Manager"))
}

func showUsage() {
	lipgloss.Println()
	lipgloss.Println(helpTitle())
	lipgloss.Println("  " + cliMuted.Render("Forward remote ports (kubectl / ssh) to your computer and keep them alive."))

	helpSection("What it does", "")
	helpText("A SERVICE is a port-forward command you save under a short name.")
	helpText("Save it once, then start it by name — pf keeps it running and shows")
	helpText("live status + logs, and auto-reconnects if it drops.")
	helpText("A GROUP bundles several services so you can start them all at once.")

	helpSection("Quick start", "your first 4 commands")
	helpStep(1, `add db "kubectl port-forward service/postgres 5432:5432"`, "save a service named db")
	helpStep(2, "list", "see everything you've saved")
	helpStep(3, "db", "run it — opens the live view; press q to quit")
	helpStep(4, "theme ocean", "optional: pick a color theme you like")

	helpSection("Run services", "open the live view (TUI)")
	helpRow("<name>", "Run one service by its name (shortcut for `run`)")
	helpRow("<name1,name2,...>", "Run several services at once (comma-separated)")
	helpRow("<group>", "Run every service in a group")
	helpRow("r, run <targets>", "Same thing, written explicitly")
	helpRow("ra, run all", "Run every service you've saved")
	helpEg("db,redis")
	helpEg("backend")

	helpSection("Manage services", "")
	helpRow(`a, add <name> "<command>"`, "Save a new service (wrap the command in quotes)")
	helpRow("l, list", "List saved services and their commands")
	helpRow("d, delete <name>", "Delete a saved service")
	helpRow("rename <old> <new>", "Rename a service (or a group)")
	helpEg(`add api "kubectl port-forward svc/api 8080:80"`)

	helpSection("Groups", "bundle services · pf group <sub>")
	helpRow("add <name> <svcs>", "Create a group from comma-separated services")
	helpRow("add-service <name> <svcs>", "Add services into an existing group")
	helpRow("remove-service <name> <svcs>", "Remove services from a group")
	helpRow("list", "List groups and their members")
	helpRow("delete <name>", "Delete the group (the services are kept)")
	helpRow("rename <old> <new>", "Rename a group")
	helpEg("group add backend api,db,redis")

	helpSection("Certificate & kubectl", "for secure clusters · pf cert <sub>")
	helpRow("cert add <p12-file>", "Load a client cert; pf adds it to every kubectl call")
	helpRow("cert list", "Show the loaded certificate")
	helpRow("cert remove", "Forget the certificate")
	helpRow("k, kubectl <args>", "Run any kubectl command with that cert applied")
	helpEg("k get pods -n production")

	helpSection("Appearance", "")
	helpRow("theme [name|list]", "Change colors: default · ocean · sunset")
	helpRow("icon [on|off|status]", "Small app icons by services (needs a Nerd Font)")
	helpEg("theme ocean")

	helpSection("Maintenance", "")
	helpRow("c, cleanup [--all]", "Free stuck ports (--all kills every kubectl/ssh)")
	helpRow("edit", "Advanced: edit everything as JSON in your $EDITOR")
	helpRow("u, update [--yes|--force]", "Update pf to the newest version")
	helpRow("v, version", "Show the installed version")
	helpRow("h, help", "Show this help")

	helpSection("Shortcuts", "type less")
	helpText("pf <name>  =  pf run <name>   (just type the service or group name)")
	helpText("a=add   l=list   d=delete   r=run   ra=run all   g=group")
	helpText("k=kubectl   c=cleanup   u=update   v=version   h=help")

	helpSection("In the live view (TUI)", "keys")
	helpRow("↑ ↓  (or j k)", "Move between services")
	helpRow("a", "Add / edit services & groups")
	helpRow("l", "Show logs for the selected service")
	helpRow("r  ·  ^r", "Restart selected  ·  restart all")
	helpRow("s", "Stop the selected service")
	helpRow("q", "Quit (stops the running services)")

	helpSection("Good to know", "")
	helpNote("Ports are LOCAL:REMOTE — `5432:5432` maps the remote port to localhost:5432.")
	helpNote("Run `pf group`, `pf cert` or `pf update` with no arguments for focused help.")
	helpNote("Icons are off by default (need a Nerd Font) — turn on with `pf icon on`.")
	helpNote("Your settings live in ~/.pf/services.json — `pf edit` opens it safely.")

	lipgloss.Println()
	lipgloss.Println("  " + cliBorder.Render("https://github.com/alinemone/go-port-forward"))
	lipgloss.Println()
}
