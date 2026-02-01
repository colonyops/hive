package initcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ScriptPath returns the default path for the hive.sh helper script.
func ScriptPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin", "hive.sh")
}

// InstallHiveScript writes the embedded hive.sh script to ~/.local/bin/hive.sh.
// Returns the installed path or an error.
func InstallHiveScript() (string, error) {
	path := ScriptPath()

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(path, []byte(HiveScript), 0o755); err != nil {
		return "", fmt.Errorf("write script: %w", err)
	}

	return path, nil
}

// ScriptInstalled checks if hive.sh is already installed and executable.
func ScriptInstalled() bool {
	path := ScriptPath()
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	// Check if executable (user execute bit)
	return info.Mode()&0o100 != 0
}

// SetupShellAlias appends the hv alias to the user's shell rc file.
// Returns an error if the shell is unsupported or the file can't be modified.
func SetupShellAlias(shell ShellInfo) error {
	if shell.RCFile == "" {
		return fmt.Errorf("no rc file for shell %s", shell.Name)
	}

	exists, err := AliasExists(shell.RCFile)
	if err != nil {
		return err
	}
	if exists {
		return nil // Already configured
	}

	aliasLine := shell.Name.AliasLine()
	content := fmt.Sprintf("\n# Hive alias (added by hive init)\n%s\n", aliasLine)

	f, err := os.OpenFile(shell.RCFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", shell.RCFile, err)
	}

	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		return fmt.Errorf("write alias: %w", err)
	}

	return f.Close()
}

// TmuxConfigPath returns the path to the user's tmux config file.
func TmuxConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".tmux.conf")
}

// TmuxBindingsExist checks if hive keybindings are already in tmux.conf.
func TmuxBindingsExist() (bool, error) {
	path := TmuxConfigPath()
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return strings.Contains(string(content), "hive init"), nil
}

// SetupTmuxConfig appends hive keybindings to ~/.tmux.conf.
func SetupTmuxConfig() error {
	exists, err := TmuxBindingsExist()
	if err != nil {
		return err
	}
	if exists {
		return nil // Already configured
	}

	bindings := `
# Hive keybindings (added by hive init)
bind-key h display-popup -E -w 80% -h 80% "hive"
bind-key H run-shell "hive"
`

	path := TmuxConfigPath()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	if _, err := f.WriteString(bindings); err != nil {
		_ = f.Close()
		return fmt.Errorf("write bindings: %w", err)
	}

	return f.Close()
}
