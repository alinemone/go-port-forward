package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/alinemone/go-port-forward/internal/configedit"
	"github.com/alinemone/go-port-forward/internal/storage"
	"github.com/alinemone/go-port-forward/internal/updater"
	"github.com/alinemone/go-port-forward/internal/version"
)

func runEditCommand() {
	st := storage.NewStorage()

	data, err := st.LoadData()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	seed, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	tmp, err := os.CreateTemp("", "pf-config-*.json")
	if err != nil {
		fmt.Printf("Error: failed to create temp file: %v\n", err)
		os.Exit(1)
	}
	tmpPath := tmp.Name()
	tmp.Write(seed)
	tmp.Close()

	for {
		cmd, err := configedit.EditorCommand(tmpPath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Remove(tmpPath)
			os.Exit(1)
		}

		if err := cmd.Run(); err != nil {
			fmt.Printf("Error: editor exited with error: %v\n", err)
			os.Remove(tmpPath)
			os.Exit(1)
		}

		edited, err := os.ReadFile(tmpPath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Remove(tmpPath)
			os.Exit(1)
		}

		validated, err := configedit.Validate(edited)
		if err == nil {
			if err := st.SaveData(validated); err != nil {
				fmt.Printf("Error: failed to save config: %v\n", err)
				os.Remove(tmpPath)
				os.Exit(1)
			}
			fmt.Printf("✓ Config saved: %d service(s), %d group(s)\n", len(validated.Services), len(validated.Groups))
			os.Remove(tmpPath)
			return
		}

		fmt.Printf("\n✗ Invalid config: %v\n", err)
		fmt.Print("Reopen to fix? [Y/n]: ")
		var answer string
		fmt.Scanln(&answer)
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer == "n" || answer == "no" {
			fmt.Printf("Aborted. Your edits are preserved at: %s\n", tmpPath)
			return
		}
	}
}

func runVersionCommand() {
	fmt.Printf("pf %s\n", version.Version)
	fmt.Printf("commit: %s\n", version.Commit)
	fmt.Printf("built: %s\n", version.BuildDate)
}

func runUpdateCommand(args []string) {
	opts := updater.Options{CurrentVersion: version.Version}
	for _, a := range args {
		switch strings.ToLower(strings.TrimSpace(a)) {
		case "-y", "--yes":
			opts.AssumeYes = true
		case "-f", "--force":
			opts.Force = true
		case "-h", "--help":
			showUpdateUsage()
			return
		default:
			fmt.Printf("Unknown flag for update: %s\n", a)
			showUpdateUsage()
			os.Exit(1)
		}
	}

	if err := updater.Run(opts); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func showUpdateUsage() {
	uHead("UPDATE:")
	uRow(16, "u, update", "Download the latest release and replace this binary")
	uRow(16, "--yes", "Skip the confirmation prompt (for scripts)")
	uRow(16, "--force", "Re-install even if already up to date")
	uExample("update", "update --yes", "update --force")

	uHead("NOTES:")
	fmt.Println("  The binary path is auto-detected (os.Executable), so it works anywhere on PATH.")
	fmt.Println("  On Windows the running pf.exe is renamed to pf.exe.old and cleaned up next launch.")
	fmt.Println("  If the install dir isn't writable, pf re-launches with sudo / a UAC prompt.")
	fmt.Println()
}
