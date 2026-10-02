package dispatch

import (
	"fmt"

	"github.com/colonyops/hive/pkg/tmpl"
)

// MaxPromptBytes bounds the shell-quoted prompt a session launch hands to an
// agent. The window command quotes the prompt into one tmux command, and the
// tmux client refuses a command over 16 KiB (MAX_IMSGSIZE) with "command too
// long". 12 KiB leaves room for the agent command, the checkout path, and the
// workspace environment that share that budget.
const MaxPromptBytes = 12 << 10

// ValidatePromptSize rejects a prompt whose shell-quoted form is longer than
// MaxPromptBytes, with an error that names the size and the limit so the
// failure points at the input rather than at the spawn.
func ValidatePromptSize(prompt string) error {
	quoted := len(tmpl.ShellQuote(prompt))
	if quoted <= MaxPromptBytes {
		return nil
	}
	return fmt.Errorf("prompt is %d bytes after shell quoting, over the %d-byte limit", quoted, MaxPromptBytes)
}
