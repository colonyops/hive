package sessions

import (
	"os/exec"
	"strings"
)

// DetectCurrentTmuxSession returns the current tmux session name, or empty if not in tmux.
func DetectCurrentTmuxSession() string {
	cmd := exec.Command("tmux", "display-message", "-p", "#S")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
