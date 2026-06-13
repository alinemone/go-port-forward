//go:build !windows

package updater

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// replaceBinary atomically swaps exePath with newPath.
//
// A plain rename is atomic, but only within a single filesystem. The
// downloaded binary lives in a temp dir (often /tmp on a separate tmpfs or
// partition), so renaming it straight onto exePath fails with EXDEV
// ("invalid cross-device link"). To stay atomic we copy the new binary into a
// sibling temp file *in the destination directory* and rename that into place,
// so the rename never crosses a filesystem boundary.
//
// On Unix the running process keeps its open inode, so replacing the on-disk
// file is safe; the next exec() picks up the new binary.
func replaceBinary(newPath, exePath string) error {
	// Fast path: same filesystem — a direct rename is atomic.
	if err := os.Rename(newPath, exePath); err == nil {
		return nil
	}

	// Cross-device fallback: stage a copy next to exePath, then rename.
	dir := filepath.Dir(exePath)
	staged, err := os.CreateTemp(dir, ".pf-update-*")
	if err != nil {
		return fmt.Errorf("create staging file in %s: %w", dir, err)
	}
	stagedPath := staged.Name()
	staged.Close()

	if err := copyFile(newPath, stagedPath); err != nil {
		_ = os.Remove(stagedPath)
		return fmt.Errorf("copy new binary into %s: %w", dir, err)
	}
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		_ = os.Remove(stagedPath)
		return fmt.Errorf("chmod staged binary: %w", err)
	}
	if err := os.Rename(stagedPath, exePath); err != nil {
		_ = os.Remove(stagedPath)
		return fmt.Errorf("move %s → %s: %w", stagedPath, exePath, err)
	}
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

// CleanupStaleArtifacts is a no-op on Unix (no .old file produced).
func CleanupStaleArtifacts() {}
