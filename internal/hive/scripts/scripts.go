// Package scripts embeds and extracts bundled helper scripts (hive-tmux, agent-send).
package scripts

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed bin/*
var binFS embed.FS

// scriptNames lists all bundled scripts.
var scriptNames = []string{"hive-tmux", "agent-send"}

// BinDir returns the path to the extracted scripts directory.
func BinDir(dataDir string) string {
	return filepath.Join(dataDir, "bin")
}

// ScriptPaths returns a map of script name -> absolute path for template functions.
func ScriptPaths(dataDir string) map[string]string {
	dir := BinDir(dataDir)
	paths := make(map[string]string, len(scriptNames))
	for _, name := range scriptNames {
		paths[name] = filepath.Join(dir, name)
	}
	return paths
}

// EnsureExtracted writes bundled scripts to $dataDir/bin/ when the version changes.
// A .version marker file tracks the last extracted version.
func EnsureExtracted(dataDir, version string) error {
	dir := BinDir(dataDir)
	marker := filepath.Join(dir, ".version")

	// Check if already extracted for this version
	if data, err := os.ReadFile(marker); err == nil && string(data) == version {
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}

	entries, err := fs.ReadDir(binFS, "bin")
	if err != nil {
		return fmt.Errorf("read embedded bin: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		content, err := binFS.ReadFile("bin/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", entry.Name(), err)
		}

		dest := filepath.Join(dir, entry.Name())
		if err := os.WriteFile(dest, content, 0o755); err != nil {
			return fmt.Errorf("write %s: %w", entry.Name(), err)
		}
	}

	// Write version marker
	if err := os.WriteFile(marker, []byte(version), 0o644); err != nil {
		return fmt.Errorf("write version marker: %w", err)
	}

	return nil
}
