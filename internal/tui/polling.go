package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

const kvPollInterval = 10 * time.Second

// kvPollTickMsg is sent to trigger KV store refresh.
type kvPollTickMsg struct{}

// scheduleKVPollTick returns a command that schedules the next KV poll tick.
func scheduleKVPollTick() tea.Cmd {
	return tea.Tick(kvPollInterval, func(time.Time) tea.Msg {
		return kvPollTickMsg{}
	})
}
