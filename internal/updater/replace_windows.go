//go:build windows

package updater

import (
	"fmt"
	"io"
	"os"
)

// replaceBinary swaps exePath with newPath on Windows.
//
// Windows refuses to overwrite a running .exe, but DOES allow renaming it.
// Strategy:
//  1. Rename the running pf.exe → pf.exe.old  (allowed even while running)
//  2. Move the freshly downloaded binary into pf.exe's original location.
//
// os.Rename uses MoveFileEx which fails across drive letters
// ("cannot move the file to a different disk drive"). When newPath and
// exePath live on different volumes (e.g. Temp on C:, exe on F:), we fall
// back to a copy+remove.
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

	if err := moveOrCopy(newPath, exePath); err != nil {
		// rollback — برگردوندن .old سرجاش
		_ = os.Rename(oldPath, exePath)
		return fmt.Errorf("move new binary into place: %w", err)
	}

	return nil
}

// moveOrCopy tries os.Rename first; on cross-drive failure falls back to
// copying the file contents and removing the source.
func moveOrCopy(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	_ = os.Remove(src)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
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
