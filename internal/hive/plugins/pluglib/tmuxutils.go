package pluglib

import (
	"fmt"

	"github.com/colonyops/hive/internal/core/config"
)

// TmuxPopup creates a UserCommand that runs cmd in a tmux popup with less for scrolling.
// The cmd should use single quotes for any internal quoting to avoid escaping issues.
func TmuxPopup(cmd string, help string) config.UserCommand {
	// Use single quotes around the command to preserve template syntax
	tmuxCmd := fmt.Sprintf("tmux popup -E -w 80%% -h 80%% -- sh -c '%s | less -R'", cmd)

	return config.UserCommand{
		Sh:     tmuxCmd,
		Help:   help,
		Silent: true,
	}
}
