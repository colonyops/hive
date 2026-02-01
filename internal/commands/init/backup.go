package initcmd

import (
	"fmt"
	"os"
)

// BackupConfig creates a backup of existing config before overwriting.
// Returns empty string if no backup was needed (file doesn't exist).
func BackupConfig(configPath string) (string, error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return "", nil
	}

	backupPath := configPath + ".bak"

	// Remove existing backup if present
	_ = os.Remove(backupPath)

	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read existing config: %w", err)
	}

	if err := os.WriteFile(backupPath, content, 0o644); err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	return backupPath, nil
}

// ConfigExists checks if a config file exists at the given path.
func ConfigExists(configPath string) bool {
	_, err := os.Stat(configPath)
	return err == nil
}
