package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/agentws"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
)

func TestCreateChatStartsDetachedWithTheUnframedPromptAndPins(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	capture := filepath.Join(t.TempDir(), "prompt")
	agent := filepath.Join(t.TempDir(), "codex")
	require.NoError(t, os.WriteFile(agent, []byte("#!/bin/sh\nprintf '%s' \"$2\" > "+capture+"\nexec cat\n"), 0o755))
	writeAgentWorkspaceManifest(t, root, "orchestrator", agentWorkspaceYAML("Orchestrator", agent+agentws.PromptTail, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"codex": agent})
	updated := make(chan struct{}, 4)
	events.Subscribe(t.Context(), svc.events, "test", events.Buffer(4), func(context.Context, events.AgentChatsUpdated) { updated <- struct{}{} })

	view, err := svc.CreateChat(t.Context(), CreateChat{
		Workspace: "orchestrator", Name: " Manage sessions ", Prompt: "watch these sessions", PinToCode: true,
	})
	require.NoError(t, err)

	assert.Equal(t, "Manage sessions", view.Name)
	assert.Positive(t, view.PinnedAt)
	assert.Equal(t, 1, liveAgentSessionCount(t, svc))
	require.Eventually(t, func() bool {
		got, readErr := os.ReadFile(capture)
		return readErr == nil && string(got) == "watch these sessions"
	}, testExitWindow, 10*time.Millisecond, "the prompt reaches the agent as written, with no schedule framing")

	listed, err := svc.ListChats(t.Context(), "orchestrator")
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, view.PinnedAt, listed[0].PinnedAt)
	select {
	case <-updated:
	case <-time.After(testExitWindow):
		t.Fatal("a started chat publishes AgentChatsUpdated")
	}
}

func TestImportPinsKeepsOrderAndSkipsWhatItCannotPin(t *testing.T) {
	t.Parallel()
	svc := newManifestOnlyService(t, t.TempDir(), nil)
	create := func(name string) stores.AgentSession {
		t.Helper()
		rec, err := svc.sessions.Create(t.Context(), stores.AgentSessionCreate{Workspace: "demo", Name: name, Agent: "claude"})
		require.NoError(t, err)
		return rec
	}
	first, second, kept := create("first"), create("second"), create("kept")
	_, err := svc.sessions.SetPinnedAt(t.Context(), kept.ID, 1)
	require.NoError(t, err)

	require.NoError(t, svc.ImportPins(t.Context(), []string{second.ID, "gone", first.ID, kept.ID}))

	pinned := map[string]int64{}
	all, err := svc.sessions.ListAll(t.Context())
	require.NoError(t, err)
	for _, rec := range all {
		pinned[rec.Name] = rec.PinnedAt
	}
	assert.Less(t, pinned["second"], pinned["first"], "import order is pin order")
	assert.Equal(t, int64(1), pinned["kept"], "a chat already pinned keeps its place")
}

func TestPinChatIsIdempotent(t *testing.T) {
	t.Parallel()
	svc := newTestAgentWorkspacesService(t, t.TempDir(), nil)
	rec, err := svc.sessions.Create(t.Context(), stores.AgentSessionCreate{Workspace: "demo", Name: "c", Agent: "claude"})
	require.NoError(t, err)

	_, err = svc.PinChat(t.Context(), "nope", true)
	assert.Equal(t, KindNotFound, KindOf(err))

	first, err := svc.PinChat(t.Context(), rec.ID, true)
	require.NoError(t, err)
	again, err := svc.PinChat(t.Context(), rec.ID, true)
	require.NoError(t, err)
	assert.Equal(t, first.PinnedAt, again.PinnedAt)

	off, err := svc.PinChat(t.Context(), rec.ID, false)
	require.NoError(t, err)
	assert.Zero(t, off.PinnedAt)
}
