package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"charm.land/lipgloss/v2"
)

// newCompletionCmd replaces Cobra's default `completion` command so we can add
// an `install` helper. The hidden `__complete` command that actually powers
// Tab-completion stays registered by Cobra.
func newCompletionCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "completion",
		Short: "Generate or install shell completion scripts",
		Args:  cobra.ArbitraryArgs,
		Run:   func(*cobra.Command, []string) { showCompletionUsage() },
	}
	c.SetHelpFunc(func(*cobra.Command, []string) { showCompletionUsage() })

	gen := func(use, short string, fn func(root *cobra.Command, w io.Writer) error) *cobra.Command {
		return &cobra.Command{
			Use: use, Short: short,
			Run: func(cmd *cobra.Command, _ []string) {
				if err := fn(cmd.Root(), os.Stdout); err != nil {
					fmt.Printf("Error: %v\n", err)
					os.Exit(1)
				}
			},
		}
	}

	c.AddCommand(
		gen("bash", "Print the bash completion script",
			func(r *cobra.Command, w io.Writer) error { return r.GenBashCompletionV2(w, true) }),
		gen("zsh", "Print the zsh completion script",
			func(r *cobra.Command, w io.Writer) error { return r.GenZshCompletion(w) }),
		gen("fish", "Print the fish completion script",
			func(r *cobra.Command, w io.Writer) error { return r.GenFishCompletion(w, true) }),
		gen("powershell", "Print the PowerShell completion script",
			func(r *cobra.Command, w io.Writer) error { return r.GenPowerShellCompletionWithDesc(w) }),
		newCompletionInstallCmd(),
	)
	return c
}

func showCompletionUsage() {
	helpSection("Completion", "pf completion <shell> | install [shell]")
	helpRow("bash | zsh | fish | powershell", "Print the completion script for that shell")
	helpRow("install [shell]", "Auto-install into your shell config (detects shell if omitted)")

	helpSection("Examples", "")
	helpExample("completion install", "set up completion for your current shell")
	helpExample("completion install powershell", "")
	helpExample("completion bash > ~/.local/share/bash-completion/completions/pf", "")

	lipgloss.Println()
}

func newCompletionInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "install [shell]",
		Short:             "Install completion into your shell config",
		Args:              cobra.MaximumNArgs(1),
		ValidArgs:         []string{"bash", "zsh", "fish", "powershell"},
		ValidArgsFunction: completeShells,
		Run: func(cmd *cobra.Command, args []string) {
			shell := ""
			if len(args) > 0 {
				shell = strings.ToLower(args[0])
			} else {
				shell = detectShell()
				fmt.Printf("Detected shell: %s\n", shell)
			}
			runCompletionInstall(cmd.Root(), shell)
		},
	}
}

func completeShells(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return []string{"bash", "zsh", "fish", "powershell"}, cobra.ShellCompDirectiveNoFileComp
}

func detectShell() string {
	if sh := strings.ToLower(os.Getenv("SHELL")); sh != "" {
		switch {
		case strings.Contains(sh, "fish"):
			return "fish"
		case strings.Contains(sh, "zsh"):
			return "zsh"
		case strings.Contains(sh, "bash"):
			return "bash"
		}
	}
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "bash"
}

func runCompletionInstall(root *cobra.Command, shell string) {
	var err error
	switch shell {
	case "bash":
		err = installPosix(root, "bash", ".bashrc",
			func(w io.Writer) error { return root.GenBashCompletionV2(w, true) })
	case "zsh":
		err = installPosix(root, "zsh", ".zshrc",
			func(w io.Writer) error { return root.GenZshCompletion(w) })
	case "fish":
		err = installFish(root)
	case "powershell", "pwsh":
		err = installPowerShell(root)
	default:
		fmt.Printf("Unknown shell: %s (use bash|zsh|fish|powershell)\n", shell)
		os.Exit(1)
	}
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

// --- install helpers -------------------------------------------------------

const (
	beginMarker = "# >>> pf completion >>>"
	endMarker   = "# <<< pf completion <<<"
)

func completionDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".pf", "completion")
	return dir, os.MkdirAll(dir, 0o755)
}

func writeScriptFile(name string, gen func(io.Writer) error) (string, error) {
	dir, err := completionDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := gen(f); err != nil {
		return "", err
	}
	return path, nil
}

// injectBlock idempotently writes a begin/end-marked block into a config file,
// replacing any previous pf block (creating the file/dir if needed).
func injectBlock(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	existing, _ := os.ReadFile(path)
	content := strings.TrimRight(stripBlock(string(existing)), "\n")
	if content != "" {
		content += "\n"
	}
	content += beginMarker + "\n" + body + "\n" + endMarker + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func stripBlock(s string) string {
	start := strings.Index(s, beginMarker)
	if start < 0 {
		return s
	}
	end := strings.Index(s, endMarker)
	if end < 0 || end < start {
		return s
	}
	pre := strings.TrimRight(s[:start], "\n")
	post := strings.TrimPrefix(s[end+len(endMarker):], "\n")
	if pre != "" {
		pre += "\n"
	}
	return pre + post
}

func installPosix(root *cobra.Command, shell, rcName string, gen func(io.Writer) error) error {
	path, err := writeScriptFile("pf."+shell, gen)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	rc := filepath.Join(home, rcName)
	body := fmt.Sprintf("[ -f %q ] && source %q", path, path)
	if shell == "zsh" {
		body = "autoload -U compinit && compinit\n" + body
	}
	if err := injectBlock(rc, body); err != nil {
		return err
	}
	fmt.Printf("✓ Installed %s completion\n", shell)
	fmt.Printf("  script:      %s\n", path)
	fmt.Printf("  loaded from: %s\n", rc)
	fmt.Printf("  reload now:  source %s\n", rc)
	return nil
}

func installFish(root *cobra.Command) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".config", "fish", "completions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "pf.fish")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := root.GenFishCompletion(f, true); err != nil {
		return err
	}
	fmt.Printf("✓ Installed fish completion → %s (auto-loaded)\n", path)
	return nil
}

func installPowerShell(root *cobra.Command) error {
	path, err := writeScriptFile("pf.ps1", func(w io.Writer) error {
		return root.GenPowerShellCompletionWithDesc(w)
	})
	if err != nil {
		return err
	}
	profile := powershellProfilePath()
	if profile == "" {
		fmt.Printf("✓ Wrote PowerShell completion → %s\n", path)
		fmt.Println("  Couldn't locate $PROFILE automatically. Add this line to your profile:")
		fmt.Printf("    . %q\n", path)
		return nil
	}
	if err := injectBlock(profile, fmt.Sprintf(". %q", path)); err != nil {
		return err
	}
	fmt.Printf("✓ Installed PowerShell completion\n")
	fmt.Printf("  script:  %s\n", path)
	fmt.Printf("  profile: %s\n", profile)
	fmt.Println("  reload now: . $PROFILE   (or open a new PowerShell)")
	return nil
}

// powershellProfilePath asks PowerShell itself for $PROFILE so OneDrive / PS7
// redirection is handled correctly.
func powershellProfilePath() string {
	for _, exe := range []string{"pwsh", "powershell"} {
		out, err := exec.Command(exe, "-NoProfile", "-Command", "$PROFILE.CurrentUserAllHosts").Output()
		if err == nil {
			if p := strings.TrimSpace(string(out)); p != "" {
				return p
			}
		}
	}
	return ""
}
