package agentupdate

import (
	"fmt"
	"os"
)

// StageAndSwap atomically (as much as the OS allows) replaces the binary at
// currentPath with newData: write to a sibling temp file, rename the
// currently-running binary aside as a backup, then rename the staged file
// into its place. Renaming a file out from under a running process is safe
// on both Linux (the open inode stays valid under its old name) and
// Windows (the loader opens the image with FILE_SHARE_DELETE, so renaming —
// unlike deleting or overwriting in place — is permitted); this is the same
// pattern used by most self-updating native applications.
func StageAndSwap(currentPath string, newData []byte) (backupPath string, err error) {
	staged := currentPath + ".new"
	if err := os.WriteFile(staged, newData, 0o755); err != nil {
		return "", fmt.Errorf("agentupdate: write staged binary: %w", err)
	}
	backupPath = currentPath + ".old"
	_ = os.Remove(backupPath) // drop any stale backup left by a prior update
	if err := os.Rename(currentPath, backupPath); err != nil {
		_ = os.Remove(staged)
		return "", fmt.Errorf("agentupdate: back up current binary: %w", err)
	}
	if err := os.Rename(staged, currentPath); err != nil {
		_ = os.Rename(backupPath, currentPath) // best-effort: don't leave the install with no binary at all
		return "", fmt.Errorf("agentupdate: move staged binary into place: %w", err)
	}
	return backupPath, nil
}

// RestoreBackup reverses StageAndSwap after a detected boot failure
// (docs/AGENT.md §15 step 11, "Rollback bei Fehler").
func RestoreBackup(currentPath, backupPath string) error {
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("agentupdate: backup not found: %w", err)
	}
	_ = os.Remove(currentPath)
	if err := os.Rename(backupPath, currentPath); err != nil {
		return fmt.Errorf("agentupdate: restore backup: %w", err)
	}
	return nil
}
