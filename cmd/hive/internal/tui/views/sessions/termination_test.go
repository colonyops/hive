package sessions

import (
	"testing"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	statussvc "github.com/colonyops/hive/internal/hive/status"
	"github.com/stretchr/testify/require"
)

func TestTerminationNoticeRequiresConfirmedAbsence(t *testing.T) {
	v := newViewWithTerminalMgr([]session.Session{{ID: "one", Name: "agent", State: session.StateActive}})
	v.terminalStatuses.Set("one", statussvc.TerminalStatus{SessionPresent: true, PresenceKnown: true, Status: terminal.StatusReady})
	unknown := TerminalStatusBatchCompleteMsg{Results: map[string]statussvc.TerminalStatus{"one": {Status: terminal.StatusMissing}}}
	require.Nil(t, v.handleTerminalStatusComplete(unknown))
	cached, ok := v.terminalStatuses.Get("one")
	require.True(t, ok)
	require.True(t, cached.SessionPresent)
	gone := TerminalStatusBatchCompleteMsg{Results: map[string]statussvc.TerminalStatus{"one": {PresenceKnown: true, Status: terminal.StatusMissing}}}
	cmd := v.handleTerminalStatusComplete(gone)
	require.NotNil(t, cmd)
	require.Equal(t, TerminalEndedMsg{Sessions: []EndedSession{{ID: "one", Name: "agent"}}}, cmd())
	require.Nil(t, v.handleTerminalStatusComplete(gone))
}

func TestMissingAgentDoesNotMeanTerminatedSession(t *testing.T) {
	v := newViewWithTerminalMgr([]session.Session{{ID: "one", Name: "agent", State: session.StateActive}})
	v.terminalStatuses.Set("one", statussvc.TerminalStatus{SessionPresent: true, PresenceKnown: true, Status: terminal.StatusReady})
	missingAgent := TerminalStatusBatchCompleteMsg{Results: map[string]statussvc.TerminalStatus{"one": {SessionPresent: true, PresenceKnown: true, Status: terminal.StatusMissing}}}
	require.Nil(t, v.handleTerminalStatusComplete(missingAgent))
}

func TestNeverStartedSessionDoesNotEmitTermination(t *testing.T) {
	v := newViewWithTerminalMgr([]session.Session{{ID: "one", Name: "agent", State: session.StateActive}})
	gone := TerminalStatusBatchCompleteMsg{Results: map[string]statussvc.TerminalStatus{"one": {PresenceKnown: true, Status: terminal.StatusMissing}}}
	require.Nil(t, v.handleTerminalStatusComplete(gone))
	require.Nil(t, v.handleTerminalStatusComplete(gone))
}
