package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const dataFile = "services.json"

// رنگ‌های ANSI
const (
	colorReset   = "\033[0m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorGray    = "\033[90m"
	colorBold    = "\033[1m"
)

type ServiceStatus struct {
	Name   string
	Status string
	Local  string
	Remote string
}

var mu sync.Mutex
var statuses = make(map[string]ServiceStatus)

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func printBanner() {
	fmt.Printf("%s%s╔═══════════════════════════════════════╗%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s%s║     Port Forward Manager v1.0         ║%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s%s╚═══════════════════════════════════════╝%s\n", colorBold, colorCyan, colorReset)
}

func printHelp() {
	printBanner()
	fmt.Printf("\n%s%sUsage:%s\n", colorBold, colorYellow, colorReset)
	fmt.Printf("  %smypf%s %s<command>%s [arguments]\n\n", colorCyan, colorReset, colorGray, colorReset)

	fmt.Printf("%s%sCommands:%s\n", colorBold, colorYellow, colorReset)
	fmt.Printf("  %sa%s, %sadd%s <name> <command> [port]  %sAdd new service%s\n", colorGreen, colorReset, colorGreen, colorReset, colorGray, colorReset)
	fmt.Printf("  %sl%s, %slist%s                        %sList all services%s\n", colorGreen, colorReset, colorGreen, colorReset, colorGray, colorReset)
	fmt.Printf("  %sr%s, %srun%s <name1,name2,...>      %sRun services%s\n", colorGreen, colorReset, colorGreen, colorReset, colorGray, colorReset)
	fmt.Printf("  %sd%s, %sdelete%s <name>              %sDelete service%s\n", colorGreen, colorReset, colorGreen, colorReset, colorGray, colorReset)
	fmt.Printf("  %sh%s, %shelp%s                       %sShow this help%s\n\n", colorGreen, colorReset, colorGreen, colorReset, colorGray, colorReset)

	fmt.Printf("%s%sExamples:%s\n", colorBold, colorYellow, colorReset)
	fmt.Printf("  %smypf a db \"ssh -L\" 5432:5432%s\n", colorGray, colorReset)
	fmt.Printf("  %smypf r db,web%s\n", colorGray, colorReset)
	fmt.Printf("  %smypf d db%s\n\n", colorGray, colorReset)
}

func getStatusColor(status string) string {
	switch status {
	case "ONLINE":
		return colorGreen
	case "CONNECTING":
		return colorYellow
	case "RECONNECTING":
		return colorRed
	default:
		return colorGray
	}
}

func getStatusIcon(status string) string {
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

func loadServices() map[string]string {
	services := make(map[string]string)
	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		return services
	}
	data, _ := os.ReadFile(dataFile)
	json.Unmarshal(data, &services)
	return services
}

func saveServices(services map[string]string) {
	data, _ := json.MarshalIndent(services, "", "  ")
	os.WriteFile(dataFile, data, 0644)
}

func runLoop(name, command, localPort, remotePort string) {
	for {
		mu.Lock()
		statuses[name] = ServiceStatus{
			Name:   name,
			Status: "CONNECTING",
			Local:  localPort,
			Remote: remotePort,
		}
		mu.Unlock()

		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/C", command)
		} else {
			cmd = exec.Command("bash", "-c", command)
		}
		// حذف output برای نمایش تمیزتر
		// cmd.Stdout = os.Stdout
		// cmd.Stderr = os.Stderr

		mu.Lock()
		statuses[name] = ServiceStatus{
			Name:   name,
			Status: "ONLINE",
			Local:  localPort,
			Remote: remotePort,
		}
		mu.Unlock()

		cmd.Run()

		mu.Lock()
		statuses[name] = ServiceStatus{
			Name:   name,
			Status: "RECONNECTING",
			Local:  localPort,
			Remote: remotePort,
		}
		mu.Unlock()

		time.Sleep(2 * time.Second)
	}
}

func displayStatusLoop() {
	for {
		clearScreen()
		printBanner()
		fmt.Printf("\n%s%s Status Monitor %s\n\n", colorBold, colorCyan, colorReset)

		mu.Lock()
		// ترتیب کردن سرویس‌ها بر اساس نام
		names := make([]string, 0, len(statuses))
		for name := range statuses {
			names = append(names, name)
		}
		sort.Strings(names)

		if len(names) == 0 {
			fmt.Printf("  %sNo services running...%s\n", colorGray, colorReset)
		} else {
			fmt.Printf("  ┌──────────────┬───────────────┬──────────────────┐\n")
			fmt.Printf("  │ %sService%s      │ %sStatus%s        │ %sPorts%s            │\n", colorBold, colorReset, colorBold, colorReset, colorBold, colorReset)
			fmt.Printf("  ├──────────────┼───────────────┼──────────────────┤\n")

			for _, name := range names {
				s := statuses[name]
				color := getStatusColor(s.Status)
				icon := getStatusIcon(s.Status)

				// نام سرویس (حداکثر 12 کاراکتر)
				displayName := s.Name
				if len(displayName) > 12 {
					displayName = displayName[:12]
				}

				// استاتوس با ایکون (حداکثر 13 کاراکتر)
				statusText := fmt.Sprintf("%s %s", icon, s.Status)

				// پورت‌ها (حداکثر 16 کاراکتر)
				ports := fmt.Sprintf("%s → %s", s.Remote, s.Local)

				fmt.Printf("  │ %-12s │ %s%-13s%s │ %-16s │\n",
					displayName,
					color, statusText, colorReset,
					ports)
			}
			fmt.Printf("  └──────────────┴───────────────┴──────────────────┘\n")
		}
		mu.Unlock()

		fmt.Printf("\n  %sPress Ctrl+C to stop%s\n", colorGray, colorReset)
		time.Sleep(1 * time.Second)
	}
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	services := loadServices()
	cmd := os.Args[1]

	switch cmd {
	case "a", "add":
		if len(os.Args) < 4 {
			fmt.Printf("%sError:%s Usage: mypf %sa%s <name> <command> [port]\n", colorRed, colorReset, colorCyan, colorReset)
			fmt.Printf("%sExample:%s mypf a db \"ssh -L\" 5432:5432\n", colorYellow, colorReset)
			return
		}
		name := os.Args[2]
		command := os.Args[3]

		if len(os.Args) >= 5 {
			command = command + " " + os.Args[4]
		}

		services[name] = command
		saveServices(services)
		fmt.Printf("%s✓%s Service %s%s%s added successfully!\n", colorGreen, colorReset, colorCyan, name, colorReset)

	case "l", "list":
		if len(services) == 0 {
			fmt.Printf("%sNo services found.%s\n", colorGray, colorReset)
			return
		}

		printBanner()
		fmt.Printf("\n%s%s Saved Services %s\n\n", colorBold, colorCyan, colorReset)

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
			fmt.Printf("  %s%d.%s %s%s%s\n", colorGray, i+1, colorReset, colorCyan, name, colorReset)
			fmt.Printf("     %s→ %s%s\n", colorGray, cmd, colorReset)
		}
		fmt.Println()

	case "r", "run":
		if len(os.Args) < 3 {
			fmt.Printf("%sError:%s Usage: mypf %sr%s <name1,name2,...>\n", colorRed, colorReset, colorCyan, colorReset)
			return
		}

		names := strings.Split(os.Args[2], ",")
		validServices := 0

		for _, name := range names {
			name = strings.TrimSpace(name)
			if command, ok := services[name]; ok {
				parts := strings.Split(command, " ")
				ports := parts[len(parts)-1]
				prt := strings.Split(ports, ":")
				if len(prt) != 2 {
					fmt.Printf("%s✗%s Invalid port format for service %s%s%s\n", colorRed, colorReset, colorCyan, name, colorReset)
					continue
				}
				remote := prt[0]
				local := prt[1]
				go runLoop(name, command, local, remote)
				validServices++
			} else {
				fmt.Printf("%s✗%s Service %s%s%s not found\n", colorRed, colorReset, colorCyan, name, colorReset)
			}
		}

		if validServices > 0 {
			go displayStatusLoop()
			select {}
		}

	case "d", "delete", "rm":
		if len(os.Args) < 3 {
			fmt.Printf("%sError:%s Usage: mypf %sd%s <name>\n", colorRed, colorReset, colorCyan, colorReset)
			return
		}

		name := os.Args[2]
		if _, ok := services[name]; ok {
			delete(services, name)
			saveServices(services)
			fmt.Printf("%s✓%s Service %s%s%s deleted successfully!\n", colorGreen, colorReset, colorCyan, name, colorReset)
		} else {
			fmt.Printf("%s✗%s Service %s%s%s not found\n", colorRed, colorReset, colorCyan, name, colorReset)
		}

	case "h", "help", "--help", "-h":
		printHelp()

	default:
		fmt.Printf("%s✗%s Unknown command: %s%s%s\n", colorRed, colorReset, colorRed, cmd, colorReset)
		fmt.Printf("Run %smypf help%s for usage information.\n", colorCyan, colorReset)
	}
}
