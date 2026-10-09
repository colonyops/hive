package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/cmd/hive/internal/tui/views/sessions"
	engineconfig "github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/notify"
	tmuxexec "github.com/colonyops/hive/internal/platform/tmux/exec"
)

func newActionTestModel() Model {
	cfg := &config.Config{}
	cfg.Tmux = engineconfig.TmuxConfig{PollInterval: time.Second}
	return Model{
		cfg:              cfg,
		modals:           NewModalCoordinator(),
		notifyBuffer:     NewNotificationBuffer(),
		actionedSessions: make(map[string]time.Time),
	}
}

func notificationMessages(m Model) []string {
	var messages []string
	for _, n := range m.notifyBuffer.Drain() {
		messages = append(messages, n.Message)
	}
	return messages
}

func TestActionCompleteAnnouncesResultMessage(t *testing.T) {
	result, _ := newActionTestModel().handleActionComplete(actionCompleteMsg{message: "Session completed successfully."})
	m := result.(Model)
	assert.Equal(t, stateNormal, m.state)
	assert.Equal(t, []string{"Session completed successfully."}, notificationMessages(m))
}

func TestActionCompleteShowsMultilineReportInOutputModal(t *testing.T) {
	report := &tmuxexec.LaunchError{Session: "work", Err: &tmuxexec.CommandExitedError{Window: "agent", Command: "agent", Status: 1, Output: "boom"}}
	result, _ := newActionTestModel().handleActionComplete(actionCompleteMsg{err: report})
	m := result.(Model)
	assert.Equal(t, stateStreaming, m.state)
	assert.False(t, m.modals.Output.IsRunning())
	messages := notificationMessages(m)
	require.Len(t, messages, 1)
	assert.NotContains(t, messages[0], "\n")
	assert.Contains(t, messages[0], `tmux session "work" failed to start`)
}

func TestActionCompleteKeepsSingleLineErrorsInToast(t *testing.T) {
	result, _ := newActionTestModel().handleActionComplete(actionCompleteMsg{err: assert.AnError})
	m := result.(Model)
	assert.Equal(t, stateNormal, m.state)
	assert.Len(t, notificationMessages(m), 1)
}

func TestTerminalEndAfterUserActionIsNotAnnounced(t *testing.T) {
	result, _ := newActionTestModel().handleActionComplete(actionCompleteMsg{sessionID: "killed"})
	m := result.(Model)
	notificationMessages(m)
	result, _ = m.handleTerminalEnded(sessions.TerminalEndedMsg{Sessions: []sessions.EndedSession{
		{ID: "killed", Name: "killed"},
		{ID: "exited", Name: "exited"},
	}})
	m = result.(Model)
	assert.Equal(t, []string{`Terminal session "exited" ended.`}, notificationMessages(m))
}

func TestTerminalEndLongAfterUserActionIsAnnounced(t *testing.T) {
	m := newActionTestModel()
	m.actionedSessions["old"] = time.Now().Add(-time.Minute)
	result, _ := m.handleTerminalEnded(sessions.TerminalEndedMsg{Sessions: []sessions.EndedSession{{ID: "old", Name: "old"}}})
	m = result.(Model)
	assert.Equal(t, []notify.Level{notify.LevelInfo}, levels(m))
	assert.Empty(t, m.actionedSessions)
}

func levels(m Model) []notify.Level {
	var out []notify.Level
	for _, n := range m.notifyBuffer.Drain() {
		out = append(out, n.Level)
	}
	return out
}
