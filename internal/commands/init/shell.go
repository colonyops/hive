package initcmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Shell represents a detected shell type.
type Shell string

const (
	ShellZsh  Shell = "zsh"
	ShellBash Shell = "bash"
	ShellFish Shell = "fish"
)

// ShellInfo contains detected shell information.
type ShellInfo struct {
	Name   Shell
	RCFile string
}

// DetectShell returns the user's shell and rc file path.
func DetectShell() (ShellInfo, error) {
	shellPath := os.Getenv("SHELL")
	if shellPath == "" {
		return ShellInfo{}, errors.New("SHELL environment variable not set")
	}

	shell := Shell(filepath.Base(shellPath))
	home, err := os.UserHomeDir()
	if err != nil {
		return ShellInfo{}, err
	}

	var rcFile string
	switch shell {
	case ShellZsh:
		rcFile = filepath.Join(home, ".zshrc")
	case ShellBash:
		rcFile = filepath.Join(home, ".bashrc")
		if _, err := os.Stat(rcFile); os.IsNotExist(err) {
			rcFile = filepath.Join(home, ".bash_profile")
		}
	case ShellFish:
		rcFile = filepath.Join(home, ".config", "fish", "config.fish")
	default:
		return ShellInfo{Name: shell}, fmt.Errorf("unsupported shell: %s", shell)
	}

	return ShellInfo{Name: shell, RCFile: rcFile}, nil
}

// AliasLine returns the appropriate alias line for the shell.
func (s Shell) AliasLine() string {
	switch s {
	case ShellFish:
		return `alias hv 'tmux new-session -As hive hive'`
	default:
		return `alias hv="tmux new-session -As hive hive"`
	}
}

// AliasExists checks if the hv alias is already configured in the rc file.
func AliasExists(rcFile string) (bool, error) {
	content, err := os.ReadFile(rcFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return strings.Contains(string(content), "alias hv"), nil
}
