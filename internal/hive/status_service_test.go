package hive

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/core/session"
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
		{WindowID: "@1", WindowIndex: "0", WindowName: "main", PaneID: "%1", DetectedTool: "claude", PaneContent: "ready"},
		{WindowID: "@1", WindowIndex: "0", WindowName: "main", PaneID: "%2", DetectedTool: "codex", PaneContent: "approval"},
		{WindowID: "@2", WindowIndex: "1", WindowName: "main", PaneID: "%3", DetectedTool: "aider", PaneContent: "active"},
	}

	got := groupPaneStatuses(context.Background(), integration, "sess", infos)

	require.Len(t, got, 2)
	assert.Equal(t, "@1", got[0].WindowID)
	assert.Equal(t, "0", got[0].WindowIndex)
	assert.Equal(t, terminal.StatusApproval, got[0].Status)
	assert.Equal(t, "codex", got[0].Tool)
	assert.Equal(t, "approval", got[0].PaneContent)
	assert.Len(t, got[0].Panes, 2)
	assert.Equal(t, "@2", got[1].WindowID)
	assert.Equal(t, "1", got[1].WindowIndex)
	assert.Equal(t, terminal.StatusActive, got[1].Status)
}

func TestGroupPaneStatusesFallsBackWhenWindowIDIsUnavailable(t *testing.T) {
	integration := &fakeTerminalIntegration{statuses: map[string]terminal.Status{
		"%1": terminal.StatusReady,
		"%2": terminal.StatusApproval,
		"%3": terminal.StatusActive,
	}}
	infos := []*terminal.SessionInfo{
		{WindowIndex: "0", WindowName: "main", PaneID: "%1"},
		{WindowIndex: "0", WindowName: "main", PaneID: "%2"},
		{WindowIndex: "1", WindowName: "main", PaneID: "%3"},
	}

	got := groupPaneStatuses(t.Context(), integration, "sess", infos)

	require.Len(t, got, 2)
	assert.Len(t, got[0].Panes, 2)
	assert.Equal(t, "0", got[0].WindowIndex)
	assert.Len(t, got[1].Panes, 1)
	assert.Equal(t, "1", got[1].WindowIndex)
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

func TestFetchSessionReportsLivenessAndWindowIdentity(t *testing.T) {
	integration := &fakeTerminalIntegration{
		info:     &terminal.SessionInfo{Name: "sess", WindowID: "@7", WindowIndex: "2", WindowName: "agent", PaneID: "%1", DetectedTool: "claude"},
		statuses: map[string]terminal.Status{"%1": terminal.StatusReady},
	}
	manager := terminal.NewManager([]string{"fake"})
	manager.Register(integration)

	got := NewStatusService(manager, 1).FetchSession(t.Context(), &session.Session{Slug: "sess"})

	assert.True(t, got.Running)
	assert.Equal(t, terminal.StatusReady, got.Status)
	assert.Equal(t, "@7", got.WindowID)
	assert.Equal(t, "agent", got.WindowName)
	assert.Equal(t, "claude", got.Tool)
}

func TestFetchSessionReportsRunningWithoutAnAgentPane(t *testing.T) {
	integration := &fakeTerminalIntegration{
		info:     &terminal.SessionInfo{Name: "sess"},
		statuses: map[string]terminal.Status{"": terminal.StatusMissing},
	}
	manager := terminal.NewManager([]string{"fake"})
	manager.Register(integration)

	got := NewStatusService(manager, 1).FetchSession(t.Context(), &session.Session{Slug: "sess"})

	assert.True(t, got.Running)
	assert.Equal(t, terminal.StatusMissing, got.Status)
	assert.Empty(t, got.WindowID)
}

type fakeTerminalIntegration struct {
	info     *terminal.SessionInfo
	statuses map[string]terminal.Status
}

func (f *fakeTerminalIntegration) Name() string    { return "fake" }
func (f *fakeTerminalIntegration) Available() bool { return true }
func (f *fakeTerminalIntegration) RefreshCache()   {}
func (f *fakeTerminalIntegration) DiscoverSession(context.Context, string, map[string]string) (*terminal.SessionInfo, error) {
	return f.info, nil
}

func (f *fakeTerminalIntegration) GetStatus(_ context.Context, info *terminal.SessionInfo) (terminal.Status, error) {
	return f.statuses[info.PaneID], nil
}
