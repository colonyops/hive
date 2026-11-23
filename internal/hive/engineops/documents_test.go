package engineops_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	drivesvc "github.com/colonyops/hive/internal/hive/drive"
	"github.com/colonyops/hive/internal/hive/engineops"
	"github.com/colonyops/hive/internal/hive/hivetest"
	"github.com/colonyops/hive/internal/ops"
	driveops "github.com/colonyops/hive/internal/ops/drive"
	"github.com/colonyops/hive/pkg/executil/executiltest"
)

type fakeChats map[string]drivesvc.Chat

func (f fakeChats) Chat(_ context.Context, id string) (drivesvc.Chat, error) {
	chat, ok := f[id]
	if !ok {
		return drivesvc.Chat{}, ops.Errorf(ops.KindNotFound, "session %q not found", id)
	}
	return chat, nil
}

func put(t *testing.T, h *engineops.Handlers, caller, path string) (driveops.DocumentEntry, error) {
	t.Helper()
	return h.DocumentPut(t.Context(), driveops.DocumentPutIn{Caller: caller, Path: path, Body: "x"})
}

func contextFile(t *testing.T, f fixture, parts ...string) string {
	t.Helper()
	return filepath.Join(append([]string{f.harness.Engine.Config().ContextDir()}, parts...)...)
}

func TestDocumentsFileUnderTheCallersSpace(t *testing.T) {
	f := newFixture(t)

	sub := filepath.Join(f.sess.Path, "cmd")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	_, err := put(t, f.h, sub, "plans/a.md")
	require.NoError(t, err)
	assert.FileExists(t, contextFile(t, f, "acme", "widgets", "plans", "a.md"), "a path inside a session is its repository")

	_, err = put(t, f.h, f.sess.ID, "plans/b.md")
	require.NoError(t, err)
	assert.FileExists(t, contextFile(t, f, "acme", "widgets", "plans", "b.md"), "a Code session's HIVE_AGENT_SESSION is its id")

	_, err = put(t, f.h, " global ", "notes.md")
	require.NoError(t, err)
	assert.FileExists(t, contextFile(t, f, "shared", "notes.md"))

	listed, err := f.h.DocumentList(t.Context(), driveops.DocumentListIn{Caller: f.sess.ID, Path: "plans"})
	require.NoError(t, err)
	assert.Equal(t, "acme/widgets", listed.Space)
	assert.Equal(t, driveops.DetailChildren, listed.Detail)
	assert.Len(t, listed.JSONLines(), 2)

	got, err := f.h.DocumentGet(t.Context(), driveops.DocumentGetIn{Caller: f.sess.ID, Path: "plans/a.md"})
	require.NoError(t, err)
	assert.Equal(t, "x", got.Body)

	moved, err := f.h.DocumentMove(t.Context(), driveops.DocumentMoveIn{Caller: f.sess.ID, From: "plans/a.md", To: "plans/c.md"})
	require.NoError(t, err)
	assert.Equal(t, "plans/c.md", moved.Path)

	deleted, err := f.h.DocumentDelete(t.Context(), driveops.DocumentDeleteIn{Caller: f.sess.ID, Path: "plans/c.md"})
	require.NoError(t, err)
	assert.True(t, deleted.Deleted)
}

func TestChatDocumentsNeedTheChatPort(t *testing.T) {
	f := newFixture(t)
	_, err := put(t, f.h, chatID, "a.md")
	assert.Equal(t, ops.KindUnavailable, kindOf(t, err), "the CLI has no chat records")

	harness := hivetest.NewEngine(t, hivetest.Options{Chats: fakeChats{
		chatID: {ID: chatID, Workspace: "ws"},
		"42":   {ID: chatID, Workspace: "ws"},
	}})
	h := engineops.New(zerolog.Nop(), harness.Engine)
	_, err = put(t, h, chatID, "a.md")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(harness.Engine.Config().ContextDir(), "workspaces", "ws", "a.md"))

	_, err = put(t, h, "42", "b.md")
	require.NoError(t, err, "a migrated chat's all-digit alias still resolves")

	_, err = put(t, h, "0b5f3c2e-7a41-4d8e-9c3a-000000000000", "a.md")
	assert.Equal(t, ops.KindNotFound, kindOf(t, err))
}

func TestADirectoryInsideNoSessionIsItsRemotesRepository(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()

	f.harness.Exec.Responses = append(make([]executiltest.Response, len(f.harness.Exec.Calls())),
		executiltest.Response{Out: []byte("https://github.com/acme/widgets.git\n")})
	_, err := put(t, f.h, dir, "from-clone.md")
	require.NoError(t, err)
	assert.FileExists(t, contextFile(t, f, "acme", "widgets", "from-clone.md"), "the same space the repository's sessions use")

	_, err = put(t, f.h, t.TempDir(), "nowhere.md")
	assert.Equal(t, ops.KindNotFound, kindOf(t, err), "a directory with no remote names no space")
}

func TestOversizedBodyIsRefusedBeforeTheCaller(t *testing.T) {
	f := newFixture(t)
	_, err := f.h.DocumentPut(t.Context(), driveops.DocumentPutIn{
		Caller: "global", Path: "big.md", Body: strings.Repeat("x", drivesvc.MaxDocumentBytes+1),
	})
	assert.Equal(t, "invalid: document is too large (2097152 bytes max)", err.Error())
	_, statErr := os.Stat(contextFile(t, f, "shared", "big.md"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}
