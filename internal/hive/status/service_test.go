package status

import (
	"context"
	"testing"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
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

	got := NewService(zerolog.Nop(), nil, 1).groupPaneStatuses(context.Background(), integration, "sess", infos)

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
	assert.Greater(t, statusRank(terminal.StatusApproval), statusRank(terminal.StatusQuestion))
	assert.Greater(t, statusRank(terminal.StatusQuestion), statusRank(terminal.StatusActive))
	assert.Greater(t, statusRank(terminal.StatusActive), statusRank(terminal.StatusMissing))
	assert.Greater(t, statusRank(terminal.StatusMissing), statusRank(terminal.StatusReady))
	assert.Zero(t, statusRank(terminal.Status("unknown")))
}

func TestAggregateStatus_FullOrdering(t *testing.T) {
	// Highest to lowest priority; aggregateStatus must pick the higher-ranked
	// status regardless of which side is "current" vs "next".
	order := []terminal.Status{
		terminal.StatusApproval,
		terminal.StatusQuestion,
		terminal.StatusActive,
		terminal.StatusMissing,
		terminal.StatusReady,
		terminal.Status(""),
	}

	for hi := 0; hi < len(order); hi++ {
		for lo := hi + 1; lo < len(order); lo++ {
			higher, lower := order[hi], order[lo]
			assert.Equal(t, higher, aggregateStatus(higher, lower), "%s should outrank %s as current", higher, lower)
			assert.Equal(t, higher, aggregateStatus(lower, higher), "%s should outrank %s as next", higher, lower)
		}
	}
}

func TestAgentPanes(t *testing.T) {
	integration := &fakeTerminalIntegration{
		statuses: map[string]terminal.Status{"%1": terminal.StatusReady, "%4": terminal.StatusApproval},
		session:  &terminal.SessionInfo{PaneID: "%1"},
		panes: []*terminal.SessionInfo{
			{WindowName: "claude", PaneID: "%1", DetectedTool: "claude"},
			{WindowName: "codex", PaneID: "%4", DetectedTool: "codex"},
		},
	}
	term := terminal.NewManager([]string{"fake"})
	term.Register(integration)

	got := NewService(zerolog.Nop(), term, 1).AgentPanes(context.Background(), []*session.Session{{ID: "s1", Slug: "sess"}})

	assert.Equal(t, map[string][]AgentPane{"s1": {
		{PaneID: "%1", WindowName: "claude", Status: terminal.StatusReady, Tool: "claude"},
		{PaneID: "%4", WindowName: "codex", Status: terminal.StatusApproval, Tool: "codex"},
	}}, got)
}

func TestAgentPanesNoTerminal(t *testing.T) {
	term := terminal.NewManager([]string{"fake"})
	term.Register(&fakeTerminalIntegration{})

	got := NewService(zerolog.Nop(), term, 1).AgentPanes(context.Background(), []*session.Session{{ID: "s1", Slug: "sess"}})

	assert.Empty(t, got["s1"])
}

type fakeTerminalIntegration struct {
	statuses map[string]terminal.Status
	session  *terminal.SessionInfo
	panes    []*terminal.SessionInfo
}

func (f *fakeTerminalIntegration) DiscoverAllPanes(context.Context, string, map[string]string) ([]*terminal.SessionInfo, error) {
	return f.panes, nil
}

func (f *fakeTerminalIntegration) Name() string                 { return "fake" }
func (f *fakeTerminalIntegration) Available() bool              { return true }
func (f *fakeTerminalIntegration) RefreshCache(context.Context) {}
func (f *fakeTerminalIntegration) DiscoverSession(context.Context, string, map[string]string) (*terminal.SessionInfo, error) {
	return f.session, nil
}

func (f *fakeTerminalIntegration) GetStatus(_ context.Context, info *terminal.SessionInfo) (terminal.Status, error) {
	return f.statuses[info.PaneID], nil
}
