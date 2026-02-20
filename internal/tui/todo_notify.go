package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/internal/core/todo"
)

// sendTerminalNotification emits OSC 9 + OSC 777 escape sequences to trigger
// a desktop notification from the terminal emulator. Supports iTerm2/Kitty
// (OSC 9) and Ghostty/WezTerm/Foot (OSC 777). When running inside tmux,
// sequences are wrapped in DCS passthrough.
func sendTerminalNotification(item todo.Item) tea.Cmd {
	title := "Hive"
	body := fmt.Sprintf("New todo: %s", item.Title)

	// OSC 9 (iTerm2, Kitty)
	osc9 := fmt.Sprintf("\x1b]9;%s: %s\x07", title, body)
	// OSC 777 (Ghostty, WezTerm, Foot)
	osc777 := fmt.Sprintf("\x1b]777;notify;%s;%s\x07", title, body)

	seq := osc9 + osc777

	if os.Getenv("TMUX") != "" {
		seq = tmuxPassthrough(seq)
	}

	return tea.Raw(seq)
}

// tmuxPassthrough wraps escape sequences in DCS passthrough for tmux.
// Each ESC byte in the original sequence is doubled for tmux.
func tmuxPassthrough(seq string) string {
	// Double all ESC bytes for tmux passthrough
	doubled := strings.ReplaceAll(seq, "\x1b", "\x1b\x1b")
	return "\x1bPtmux;" + doubled + "\x1b\\"
}
