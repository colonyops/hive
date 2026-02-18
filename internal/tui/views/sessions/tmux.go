package sessions

import (
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
)

// DetectCurrentTmuxSession returns the current tmux session name, or empty if not in tmux.
func DetectCurrentTmuxSession() string {
	cmd := exec.Command("tmux", "display-message", "-p", "#S")
	output, err := cmd.Output()
	if err != nil {
		log.Debug().Err(err).Msg("tmux session detection failed")
		return ""
	}
	return strings.TrimSpace(string(output))
}
