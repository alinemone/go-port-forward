//go:build !windows

package updater

import (
	"fmt"
	"os"
)

// replaceBinary atomically swaps exePath with newPath.
// On Unix this is a single rename: the running process keeps its inode,
// the next exec() sees the new file.
func replaceBinary(newPath, exePath string) error {
	if err := os.Rename(newPath, exePath); err != nil {
		return fmt.Errorf("rename %s → %s: %w", newPath, exePath, err)
	}
	return nil
}

// CleanupStaleArtifacts is a no-op on Unix (no .old file produced).
func CleanupStaleArtifacts() {}
