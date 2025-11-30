package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

func extractPorts(command string) (local, remote string, ok bool) {
	portRegex := regexp.MustCompile(`(\d+):(\d+)`)
	matches := portRegex.FindStringSubmatch(command)
	if len(matches) == 3 {
		return matches[2], matches[1], true
	}
	return "", "", false
}

func main() {
	if len(os.Args) < 2 {
		PrintHelp()
		return
	}

	services := LoadServices()
	cmd := os.Args[1]

	switch cmd {
	case "a", "add":
		if len(os.Args) < 4 {
			fmt.Printf("%sError:%s Usage: pf %sa%s <name> <command> \n", ColorRed, ColorReset, ColorCyan, ColorReset)
			fmt.Printf("%sExample:%s pf a db \"ssh -L\" 5432:5432\n", ColorYellow, ColorReset)
			return
		}
		name := os.Args[2]
		command := os.Args[3]

		if len(os.Args) >= 5 {
			command = command + " " + os.Args[4]
		}

		services[name] = command
		SaveServices(services)
		fmt.Printf("%s✓%s Service %s%s%s added successfully!\n", ColorGreen, ColorReset, ColorCyan, name, ColorReset)

	case "l", "list":
		if len(services) == 0 {
			fmt.Printf("%sNo services found.%s\n", ColorGray, ColorReset)
			return
		}

		PrintBanner()
		fmt.Printf("\n%s%s Saved Services %s\n\n", ColorBold, ColorCyan, ColorReset)

		names := make([]string, 0, len(services))
		for name := range services {
			names = append(names, name)
		}
		sort.Strings(names)

		for i, name := range names {
			cmd := services[name]
			if len(cmd) > 50 {
				cmd = cmd[:47] + "..."
			}
			fmt.Printf("  %s%d.%s %s%s%s\n", ColorGray, i+1, ColorReset, ColorCyan, name, ColorReset)
			fmt.Printf("     %s→ %s%s\n", ColorGray, cmd, ColorReset)
		}
		fmt.Println()

	case "r", "run":
		if len(os.Args) < 3 {
			fmt.Printf("%sError:%s Usage: pf %sr%s <name1,name2,...>\n", ColorRed, ColorReset, ColorCyan, ColorReset)
			return
		}

		names := strings.Split(os.Args[2], ",")
		validServices := 0

		for _, name := range names {
			name = strings.TrimSpace(name)
			if command, ok := services[name]; ok {
				local, remote, portOk := extractPorts(command)
				if !portOk {
					fmt.Printf("%s✗%s Invalid port format for service %s%s%s\n", ColorRed, ColorReset, ColorCyan, name, ColorReset)
					continue
				}
				go RunLoop(name, command, local, remote)
				validServices++
			} else {
				fmt.Printf("%s✗%s Service %s%s%s not found\n", ColorRed, ColorReset, ColorCyan, name, ColorReset)
			}
		}

		if validServices > 0 {
			go DisplayStatusLoop()
			select {}
		}

	case "d", "delete", "rm":
		if len(os.Args) < 3 {
			fmt.Printf("%sError:%s Usage: pf %sd%s <name>\n", ColorRed, ColorReset, ColorCyan, ColorReset)
			return
		}

		name := os.Args[2]
		if _, ok := services[name]; ok {
			delete(services, name)
			SaveServices(services)
			fmt.Printf("%s✓%s Service %s%s%s deleted successfully!\n", ColorGreen, ColorReset, ColorCyan, name, ColorReset)
		} else {
			fmt.Printf("%s✗%s Service %s%s%s not found\n", ColorRed, ColorReset, ColorCyan, name, ColorReset)
		}

	case "h", "help", "--help", "-h":
		PrintHelp()

	default:
		fmt.Printf("%s✗%s Unknown command: %s%s%s\n", ColorRed, ColorReset, ColorRed, cmd, ColorReset)
		fmt.Printf("Run %spf help%s for usage information.\n", ColorCyan, ColorReset)
	}
}
