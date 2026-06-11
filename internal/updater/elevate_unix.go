//go:build !windows

package updater

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// tryElevate re-launches the current command via sudo, then exits on success.
// Returns an error only if sudo is unavailable or fails to start.
func tryElevate() error {
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return errors.New("sudo not found in PATH; please re-run as root")
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate self for re-exec: %w", err)
	}

	args := append([]string{exe}, elevatedArgs()...)

	cmd := exec.Command(sudo, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo re-exec failed: %w", err)
	}

	os.Exit(0)
	return nil
}
