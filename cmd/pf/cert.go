package main

import (
	"fmt"
	"os"

	"github.com/alinemone/go-port-forward/internal/cert"

	"charm.land/lipgloss/v2"
)

func runCertAddCommand(certMgr *cert.Manager, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: pf cert add <p12-file>")
		fmt.Println("Example: pf cert add company-vpn.p12")
		os.Exit(1)
	}

	p12Path := args[0]
	if _, err := os.Stat(p12Path); os.IsNotExist(err) {
		fmt.Printf("Error: P12 file not found: %s\n", p12Path)
		os.Exit(1)
	}

	var password string
	fmt.Print("🔐 P12 password (press Enter if none): ")
	fmt.Scanln(&password)

	if err := certMgr.AddCertificate(p12Path, password); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Certificate added successfully")
	fmt.Println("  This certificate will be used for all kubectl services")
}

func runCertListCommand(certMgr *cert.Manager) {
	config, exists := certMgr.GetCertificate()
	if !exists {
		lipgloss.Println(cliMuted.Render("No certificate configured"))
		lipgloss.Println(cliMuted.Render("Use 'pf cert add <p12-file>' to add a certificate"))
		return
	}

	lipgloss.Println()
	lipgloss.Println(cliHeading.Render("📜 Configured Certificate"))
	lipgloss.Println()
	for _, kv := range [][2]string{
		{"P12", config.P12Path},
		{"Cert", config.CertPath},
		{"Key", config.KeyPath},
	} {
		lipgloss.Printf("  %s %s\n", cliName.Render(fmt.Sprintf("%-5s", kv[0])), cliDetail.Render(kv[1]))
	}
	lipgloss.Println()
}

func runCertRemoveCommand(certMgr *cert.Manager) {
	if err := certMgr.RemoveCertificate(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Certificate removed successfully")
}

func showCertUsage() {
	uHead("CERTIFICATE:")
	uRow(22, "cert add <p12-file>", "Add a certificate for all kubectl services")
	uRow(22, "cert list", "Show the configured certificate")
	uRow(22, "cert remove", "Remove the certificate")
	uExample("cert add company-vpn.p12", "cert list", "cert remove")

	uHead("NOTES:")
	fmt.Println("  The certificate is auto-injected into every 'pf k' / 'pf kubectl' call.")
	fmt.Println()
}
