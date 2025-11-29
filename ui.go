package main

import (
	"fmt"
	"sort"
	"time"
)

func ClearScreen() {
	fmt.Print("\033[H\033[2J")
}

func PrintBanner() {
	fmt.Printf("%s%s╔═══════════════════════════════════════╗%s\n", ColorBold, ColorCyan, ColorReset)
	fmt.Printf("%s%s║     Port Forward Manager v1.0         ║%s\n", ColorBold, ColorCyan, ColorReset)
	fmt.Printf("%s%s╚═══════════════════════════════════════╝%s\n", ColorBold, ColorCyan, ColorReset)
}

func PrintHelp() {
	PrintBanner()
	fmt.Printf("\n%s%sUsage:%s\n", ColorBold, ColorYellow, ColorReset)
	fmt.Printf("  %spf%s %s<command>%s [arguments]\n\n", ColorCyan, ColorReset, ColorGray, ColorReset)

	fmt.Printf("%s%sCommands:%s\n", ColorBold, ColorYellow, ColorReset)
	fmt.Printf("  %sa%s, %sadd%s <name> <command> %sAdd new service%s\n", ColorGreen, ColorReset, ColorGreen, ColorReset, ColorGray, ColorReset)
	fmt.Printf("  %sl%s, %slist%s                        %sList all services%s\n", ColorGreen, ColorReset, ColorGreen, ColorReset, ColorGray, ColorReset)
	fmt.Printf("  %sr%s, %srun%s <name1,name2,...>      %sRun services%s\n", ColorGreen, ColorReset, ColorGreen, ColorReset, ColorGray, ColorReset)
	fmt.Printf("  %sd%s, %sdelete%s <name>              %sDelete service%s\n", ColorGreen, ColorReset, ColorGreen, ColorReset, ColorGray, ColorReset)
	fmt.Printf("  %sh%s, %shelp%s                       %sShow this help%s\n\n", ColorGreen, ColorReset, ColorGreen, ColorReset, ColorGray, ColorReset)

	fmt.Printf("%s%sExamples:%s\n", ColorBold, ColorYellow, ColorReset)
	fmt.Printf("  %spf a db \"kubectl -n prod-service-name port-forward service/prod-service-name-psql-postgresql-ha 1113:5432\" %s\n", ColorGray, ColorReset)
	fmt.Printf("  %spf r db,web%s\n", ColorGray, ColorReset)
	fmt.Printf("  %spf d db%s\n\n", ColorGray, ColorReset)
}

func GetStatusColor(status string) string {
	switch status {
	case "ONLINE":
		return ColorGreen
	case "CONNECTING":
		return ColorYellow
	case "RECONNECTING":
		return ColorRed
	default:
		return ColorGray
	}
}

func GetStatusIcon(status string) string {
	switch status {
	case "ONLINE":
		return "●"
	case "CONNECTING":
		return "◐"
	case "RECONNECTING":
		return "○"
	default:
		return "•"
	}
}

func DisplayStatusLoop() {
	for {
		ClearScreen()
		PrintBanner()
		fmt.Printf("\n%s%s Status Monitor %s\n\n", ColorBold, ColorCyan, ColorReset)

		mu.Lock()
		names := make([]string, 0, len(statuses))
		for name := range statuses {
			names = append(names, name)
		}
		sort.Strings(names)

		if len(names) == 0 {
			fmt.Printf("  %sNo services running...%s\n", ColorGray, ColorReset)
		} else {
			fmt.Printf("  ┌──────────────────────┬─────────────────┬──────────────────┐\n")
			fmt.Printf("  │ %sService%s              │ %sStatus%s          │ %sPorts%s            │\n", ColorBold, ColorReset, ColorBold, ColorReset, ColorBold, ColorReset)
			fmt.Printf("  ├──────────────────────┼─────────────────┼──────────────────┤\n")

			for _, name := range names {
				s := statuses[name]
				color := GetStatusColor(s.Status)
				icon := GetStatusIcon(s.Status)

				displayName := s.Name
				if len(displayName) > 20 {
					displayName = displayName[:20]
				}

				statusText := fmt.Sprintf("%s %s", icon, s.Status)

				ports := fmt.Sprintf("%s → %s", s.Local, s.Remote)

				fmt.Printf("  │ %-20s │ %s%-15s%s │ %-16s │\n",
					displayName,
					color, statusText, ColorReset,
					ports)
			}
			fmt.Printf("  └──────────────────────┴─────────────────┴──────────────────┘\n")
		}
		mu.Unlock()

		fmt.Printf("\n  %sPress Ctrl+C to stop%s\n", ColorGray, ColorReset)
		time.Sleep(3 * time.Second)
	}
}
