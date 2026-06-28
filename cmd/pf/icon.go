package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/alinemone/go-port-forward/internal/storage"
)

func runIconCommand(args []string) {
	st := storage.NewStorage()

	action := ""
	if len(args) > 0 {
		action = strings.ToLower(strings.TrimSpace(args[0]))
	}

	switch action {
	case "", "status":
		enabled, err := st.IconEnabled()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		printIconStatus(enabled)
	case "on", "enable", "true":
		setIcons(st, true)
	case "off", "disable", "false":
		setIcons(st, false)
	default:
		fmt.Printf("Unknown option: %s\n", action)
		fmt.Println("Usage: pf icon [on|off|status]")
		os.Exit(1)
	}
}

func setIcons(st *storage.Storage, enabled bool) {
	if err := st.SetIconEnabled(enabled); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	printIconStatus(enabled)
	if enabled {
		fmt.Println("Note: icons need a Nerd Font (https://www.nerdfonts.com) — without")
		fmt.Println("one they render as blank boxes. Set your terminal to a Nerd Font.")
	}
}

func printIconStatus(enabled bool) {
	if enabled {
		fmt.Println("✓ Service icons: ON")
	} else {
		fmt.Println("○ Service icons: OFF")
	}
}
