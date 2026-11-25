//go:build darwin

package loopback

import "os/exec"

const needsSetup = true

// Configure adds each IP as an alias on lo0. Needs root. IPs outside the pool
// are ignored, so a tampered input can't touch other interfaces' addresses.
func Configure(ips []string) {
	for _, ip := range ips {
		if InPool(ip) {
			_ = exec.Command("ifconfig", "lo0", "alias", ip, "up").Run()
		}
	}
}

// Unconfigure removes the lo0 aliases Configure added. Needs root.
func Unconfigure(ips []string) {
	for _, ip := range ips {
		if InPool(ip) {
			_ = exec.Command("ifconfig", "lo0", "-alias", ip).Run()
		}
	}
}
