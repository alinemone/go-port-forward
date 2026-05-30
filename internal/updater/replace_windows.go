//go:build windows

package updater

import (
	"fmt"
	"os"
)

// replaceBinary swaps exePath with newPath on Windows.
//
// Windows refuses to overwrite a running .exe, but DOES allow renaming it.
// Strategy:
//  1. Rename the running pf.exe → pf.exe.old  (allowed even while running)
//  2. Move the freshly downloaded binary into pf.exe's original location.
//
// The .old file stays locked until this process exits. It is cleaned up by
// CleanupStaleArtifacts() on the next launch.
func replaceBinary(newPath, exePath string) error {
	oldPath := exePath + ".old"

	// در صورت وجود .old قبلی، تلاش برای پاک کردن — اگر هنوز lock باشه نگران نباش
	_ = os.Remove(oldPath)

	if err := os.Rename(exePath, oldPath); err != nil {
		return fmt.Errorf("rename current binary to .old: %w", err)
	}

	if err := os.Rename(newPath, exePath); err != nil {
		// rollback — برگردوندن .old سرجاش
		_ = os.Rename(oldPath, exePath)
		return fmt.Errorf("move new binary into place: %w", err)
	}

	return nil
}

// CleanupStaleArtifacts removes the pf.exe.old left behind by a previous update.
// Safe to call on every startup; silent if nothing to do.
func CleanupStaleArtifacts() {
	exe, err := currentExePath()
	if err != nil {
		return
	}
	_ = os.Remove(exe + ".old")
}
