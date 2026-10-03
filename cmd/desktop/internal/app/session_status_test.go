package app

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	statussvc "github.com/colonyops/hive/internal/hive/status"
	tmuxcc "github.com/colonyops/hive/internal/platform/tmux/control"
)

func TestProjectSessionStatusesMapsWindowsToStableIDs(t *testing.T) {
	active := []*session.Session{
		{ID: "s1", Slug: "one", State: session.StateActive, Metadata: map[string]string{session.MetaTmuxSession: "actual-one"}},
		{ID: "s2", Slug: "two", State: session.StateActive},
		{ID: "s4", Slug: "four", State: session.StateActive},
		{ID: "s5", Slug: "five", State: session.StateActive},
	}
	statuses := map[string]statussvc.TerminalStatus{
		"s1": {Windows: []statussvc.WindowStatus{
			{WindowIndex: "0", Status: terminal.StatusApproval, Tool: "claude"},
			{WindowIndex: "2", Status: terminal.StatusActive, Tool: "pi"},
			{WindowIndex: "9", WindowName: "departed", Status: terminal.StatusActive, Tool: "codex"},
		}},
		"s2": {WindowName: "codex", Status: terminal.StatusQuestion, Tool: "codex"},
		"s4": {Status: terminal.StatusMissing, Error: errors.New("session disappeared")},
	}
	windows := map[string][]tmuxcc.IndexedWindow{
		"actual-one": {{ID: "@1", Index: "0", Name: "claude"}, {ID: "@2", Index: "2", Name: "pi"}},
		"two":        {{ID: "@3", Index: "1", Name: "codex"}},
	}

	assert.Equal(t, []SessionStatus{
		{SessionID: "s1", Running: true, Windows: []SessionWindowStatus{
			{WindowID: "@1", Status: terminal.StatusApproval, Tool: "claude"},
			{WindowID: "@2", Status: terminal.StatusActive, Tool: "pi"},
		}},
		{SessionID: "s2", Running: true, Windows: []SessionWindowStatus{{WindowID: "@3", Status: terminal.StatusApproval, Tool: "codex"}}},
		{SessionID: "s4", Windows: []SessionWindowStatus{}},
	}, projectSessionStatuses(active, statuses, windows, true), "a session hive has no status for is left out")
}

func TestProjectSessionStatusesFallsBackToHiveLivenessWithoutWindows(t *testing.T) {
	active := []*session.Session{
		{ID: "s1", Slug: "one", State: session.StateActive},
		{ID: "s2", Slug: "two", State: session.StateActive},
	}
	statuses := map[string]statussvc.TerminalStatus{
		"s1": {Status: terminal.StatusReady},
		"s2": {Status: terminal.StatusMissing},
	}

	got := projectSessionStatuses(active, statuses, nil, false)
	require.Len(t, got, 2)
	assert.True(t, got[0].Running)
	assert.False(t, got[1].Running)
}

func TestStableWindowIDFallsBackToNameWhenAnIndexWasReused(t *testing.T) {
	t.Parallel()

	refs := []tmuxcc.IndexedWindow{
		{ID: "@shell", Index: "0", Name: "shell"},
		{ID: "@agent", Index: "2", Name: "agent"},
	}
	assert.Equal(t, "@agent", stableWindowID("0", "agent", refs))
}

func TestStableWindowIDDoesNotMapADepartedIndexedWindowToTheSoleSurvivor(t *testing.T) {
	t.Parallel()

	refs := []tmuxcc.IndexedWindow{{ID: "@shell", Index: "0", Name: "shell"}}
	assert.Empty(t, stableWindowID("2", "agent", refs))
}

// Status reads only active sessions and asks tmux about each one's persisted
// target, not its slug.
func TestSessionsService_SessionStatusesProbesActiveSessionsByTheirTmuxTarget(t *testing.T) {
	h := newHiveHarness(t, engineOptions{panes: stubPanes{}})
	renamed := session.Session{ID: "s1", Name: "one", Slug: "one", State: session.StateActive}
	renamed.SetMeta(session.MetaTmuxSession, "actual-one")
	h.save(t,
		renamed,
		session.Session{ID: "s2", Name: "two", Slug: "two", State: session.StateActive},
		session.Session{ID: "s3", Name: "three", Slug: "three", State: session.StateRecycled},
	)
	windows := &fakeWindowSource{results: map[string][]tmuxcc.IndexedWindow{"actual-one": {{ID: "@1", Index: "0"}}}}
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Windows: windows})

	got, err := svc.SessionStatuses(t.Context())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"actual-one", "two"}, windows.seen)
	running := map[string]bool{}
	for _, item := range got.Items {
		running[item.SessionID] = item.Running
	}
	assert.Equal(t, map[string]bool{"s1": true, "s2": false}, running)
}

func TestSessionsService_SessionStatusesIsEmptyWithoutStatus(t *testing.T) {
	h := newHiveHarness(t, engineOptions{cfg: func(cfg *config.Config) { cfg.Tmux.PollInterval = 1500 * time.Millisecond }})
	h.save(t, reviewSession())
	windows := &fakeWindowSource{}
	svc := newSessionsService(SessionsDeps{Hive: h.engine, Windows: windows})

	got, err := svc.SessionStatuses(t.Context())
	require.NoError(t, err)
	assert.Empty(t, got.Items)
	assert.NotNil(t, got.Items)
	assert.Equal(t, 1500*time.Millisecond, got.PollInterval)
	assert.Empty(t, windows.seen)
}

// The poll interval is hive config, so a reload changes the next answer
// without rebuilding the service.
func TestSessionsService_SessionStatusesReportTheReloadedPollInterval(t *testing.T) {
	h := newHiveHarness(t, engineOptions{cfg: func(cfg *config.Config) { cfg.Tmux.PollInterval = 5 * time.Second }})
	svc := newSessionsService(SessionsDeps{Hive: h.engine})

	before, err := svc.SessionStatuses(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, before.PollInterval)

	reloaded := *h.cfg
	reloaded.Tmux.PollInterval = 9 * time.Second
	require.NoError(t, h.engine.Reload(&reloaded))

	after, err := svc.SessionStatuses(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 9*time.Second, after.PollInterval)
}
