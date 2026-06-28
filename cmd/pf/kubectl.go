package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/alinemone/go-port-forward/internal/cert"
)

func runKubectlCommand(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: pf kubectl <kubectl-args...>")
		fmt.Println("Alias: pf k <kubectl-args...>")
		fmt.Println("Example: pf k get pods -n production")
		os.Exit(1)
	}

	finalArgs := append([]string{}, args...)

	certMgr, err := cert.NewManager()
	if err == nil {
		if certConfig, exists := certMgr.GetCertificate(); exists && !hasKubectlClientCertArgs(finalArgs) {
			certArgs := []string{
				"--client-certificate=" + certConfig.CertPath,
				"--client-key=" + certConfig.KeyPath,
			}
			finalArgs = append(certArgs, finalArgs...)
		}
	}

	cmd := exec.Command("kubectl", finalArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Printf("Error: failed to run kubectl: %v\n", err)
		os.Exit(1)
	}
}

func hasKubectlClientCertArgs(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--client-certificate") || strings.HasPrefix(arg, "--client-key") {
			return true
		}
	}
	return false
}
