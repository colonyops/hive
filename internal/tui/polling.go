package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// sessionRefreshTickMsg is sent to trigger session list refresh.
type sessionRefreshTickMsg struct{}

// scheduleSessionRefresh returns a command that schedules the next session refresh.
func (m Model) scheduleSessionRefresh() tea.Cmd {
	interval := m.cfg.TUI.RefreshInterval
	if interval == 0 {
		return nil // Disabled
	}
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return sessionRefreshTickMsg{}
	})
}

const kvPollInterval = 10 * time.Second

// kvPollTickMsg is sent to trigger KV store refresh.
type kvPollTickMsg struct{}

// scheduleKVPollTick returns a command that schedules the next KV poll tick.
func scheduleKVPollTick() tea.Cmd {
	return tea.Tick(kvPollInterval, func(time.Time) tea.Msg {
		return kvPollTickMsg{}
	})
}

// Animation constants.
const animationTickInterval = 100 * time.Millisecond

// animationTickMsg is sent to advance the status animation.
type animationTickMsg struct{}

// scheduleAnimationTick returns a command that schedules the next animation frame.
func scheduleAnimationTick() tea.Cmd {
	return tea.Tick(animationTickInterval, func(time.Time) tea.Msg {
		return animationTickMsg{}
	})
}
