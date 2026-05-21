package sessions

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupPaneStatuses(t *testing.T) {
	integration := &fakeTerminalIntegration{statuses: map[string]terminal.Status{
		"%1": terminal.StatusReady,
		"%2": terminal.StatusApproval,
		"%3": terminal.StatusActive,
	}}
	infos := []*terminal.SessionInfo{
		{WindowIndex: "0", WindowName: "main", PaneID: "%1", DetectedTool: "claude", PaneContent: "ready"},
		{WindowIndex: "0", WindowName: "main", PaneID: "%2", DetectedTool: "codex", PaneContent: "approval"},
		{WindowIndex: "1", WindowName: "main", PaneID: "%3", DetectedTool: "aider", PaneContent: "active"},
	}

	got := groupPaneStatuses(context.Background(), integration, "sess", infos)

	require.Len(t, got, 2)
	assert.Equal(t, "0", got[0].WindowIndex)
	assert.Equal(t, terminal.StatusApproval, got[0].Status)
	assert.Len(t, got[0].Panes, 2)
	assert.Equal(t, "1", got[1].WindowIndex)
	assert.Equal(t, terminal.StatusActive, got[1].Status)
}

func TestAggregateStatus(t *testing.T) {
	tests := []struct {
		name    string
		current terminal.Status
		next    terminal.Status
		want    terminal.Status
	}{
		{name: "approval wins", current: terminal.StatusActive, next: terminal.StatusApproval, want: terminal.StatusApproval},
		{name: "active beats ready", current: terminal.StatusReady, next: terminal.StatusActive, want: terminal.StatusActive},
		{name: "missing beats ready", current: terminal.StatusReady, next: terminal.StatusMissing, want: terminal.StatusMissing},
		{name: "current higher remains", current: terminal.StatusApproval, next: terminal.StatusActive, want: terminal.StatusApproval},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, aggregateStatus(tt.current, tt.next))
		})
	}
}

func TestStatusRank(t *testing.T) {
	assert.Greater(t, statusRank(terminal.StatusApproval), statusRank(terminal.StatusActive))
	assert.Greater(t, statusRank(terminal.StatusActive), statusRank(terminal.StatusMissing))
	assert.Greater(t, statusRank(terminal.StatusMissing), statusRank(terminal.StatusReady))
	assert.Zero(t, statusRank(terminal.Status("unknown")))
}

type fakeTerminalIntegration struct {
	statuses map[string]terminal.Status
}

func (f *fakeTerminalIntegration) Name() string    { return "fake" }
func (f *fakeTerminalIntegration) Available() bool { return true }
func (f *fakeTerminalIntegration) RefreshCache()   {}
func (f *fakeTerminalIntegration) DiscoverSession(context.Context, string, map[string]string) (*terminal.SessionInfo, error) {
	return nil, nil
}

func (f *fakeTerminalIntegration) GetStatus(_ context.Context, info *terminal.SessionInfo) (terminal.Status, error) {
	return f.statuses[info.PaneID], nil
}
