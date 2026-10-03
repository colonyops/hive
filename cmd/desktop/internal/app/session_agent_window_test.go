package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
	tmuxcc "github.com/colonyops/hive/internal/platform/tmux/control"
)

type fakeAgentWindows struct {
	calls [][4]string
	err   error
}

func (f *fakeAgentWindows) NewCommandWindow(_ context.Context, slug, dir, name, command string) (string, error) {
	f.calls = append(f.calls, [4]string{slug, dir, name, command})
	return "@42", f.err
}

func TestNewAgentWindowSharesCheckoutAndReadsCurrentProfile(t *testing.T) {
	h := newHiveHarness(t, engineOptions{})
	detail := reviewSession()
	detail.Path = "/work/shared checkout"
	h.save(t, detail)
	windows := &fakeAgentWindows{}
	commands := map[string]string{"codex": "my-wrapper codex --model 'custom model'"}
	svc := newSessionsService(SessionsDeps{
		Hive: h.engine, AgentWindows: windows,
		AgentCommands: func() map[string]string { return commands },
	})

	id, err := svc.NewAgentWindow(t.Context(), detail.Slug, "codex")
	require.NoError(t, err)
	require.Equal(t, "@42", id)
	require.Equal(t, [][4]string{{detail.Slug, detail.Path, "codex", commands["codex"]}}, windows.calls)
	require.Empty(t, h.mux.opened)

	commands = map[string]string{"codex": "new-wrapper codex"}
	_, err = svc.NewAgentWindow(t.Context(), detail.Slug, "codex")
	require.NoError(t, err)
	require.Equal(t, "new-wrapper codex", windows.calls[1][3])
}

func TestNewAgentWindowRejectsInvalidTargetsBeforeSpawning(t *testing.T) {
	for _, tc := range []struct {
		name, slug, agent string
		state             session.State
		kind              Kind
	}{
		{name: "missing profile", slug: "review-81", kind: KindInvalid},
		{name: "unknown profile", slug: "review-81", agent: "sh -c bad", kind: KindInvalid},
		{name: "missing slug", agent: "codex", kind: KindInvalid},
		{name: "unknown session", slug: "missing", agent: "codex", kind: KindNotFound},
		{name: "scratch", slug: ScratchSlug, agent: "codex", kind: KindNotFound},
		{name: "chat", slug: "agentws-123", agent: "codex", kind: KindNotFound},
		{name: "recycled", slug: "review-81", agent: "codex", state: session.StateRecycled, kind: KindConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHiveHarness(t, engineOptions{})
			detail := reviewSession()
			if tc.state != "" {
				detail.State = tc.state
			}
			h.save(t, detail)
			windows := &fakeAgentWindows{}
			svc := newSessionsService(SessionsDeps{
				Hive: h.engine, AgentWindows: windows,
				AgentCommands: func() map[string]string { return map[string]string{"codex": "codex"} },
			})
			_, err := svc.NewAgentWindow(t.Context(), tc.slug, tc.agent)
			require.Error(t, err)
			require.Equal(t, tc.kind, KindOf(err))
			require.Empty(t, windows.calls)
		})
	}
}

func TestNewAgentWindowReportsAStoppedTerminal(t *testing.T) {
	h := activeHarness(t)
	detail := reviewSession()
	svc := newSessionsService(SessionsDeps{
		Hive: h.engine, AgentWindows: &fakeAgentWindows{err: tmuxcc.ErrNotAttached},
		AgentCommands: func() map[string]string { return map[string]string{"codex": "codex"} },
	})
	_, err := svc.NewAgentWindow(t.Context(), detail.Slug, "codex")
	require.Equal(t, KindNotFound, KindOf(err))
	require.Empty(t, h.mux.opened)
}
