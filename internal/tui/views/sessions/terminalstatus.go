package sessions

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/hive"
)

// TerminalStatusBatchCompleteMsg is sent when all terminal status fetches complete.
type TerminalStatusBatchCompleteMsg struct {
	Results map[string]hive.TerminalStatus // sessionID -> status
}

// TerminalPollTickMsg triggers a terminal status poll cycle.
type TerminalPollTickMsg struct{}

// FetchTerminalStatusBatch returns a command that fetches terminal status for multiple sessions.
func FetchTerminalStatusBatch(status *hive.StatusService, sessions []*session.Session) tea.Cmd {
	if len(sessions) == 0 || !status.Available() {
		return nil
	}

	return func() tea.Msg {
		return TerminalStatusBatchCompleteMsg{Results: status.FetchBatch(context.Background(), sessions)}
	}
}

// StartTerminalPollTicker returns a command that starts the terminal status poll ticker.
func StartTerminalPollTicker(interval time.Duration) tea.Cmd {
	if interval <= 0 {
		return nil
	}
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return TerminalPollTickMsg{}
	})
}
