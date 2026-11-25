package commands

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/engineops"
	"github.com/colonyops/hive/internal/hive/hivetest"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/ops/catalog"
	"github.com/colonyops/hive/internal/ops/tasks"
	"github.com/colonyops/hive/internal/platform/desktopendpoint"
	"github.com/colonyops/hive/internal/store"
)

type engineFixture struct {
	handlers *engineops.Handlers
	harness  hivetest.Harness
	sess     session.Session
	opens    int
	closes   int
}

func newEngineFixture(t *testing.T) *engineFixture {
	t.Helper()
	harness := hivetest.NewEngine(t, hivetest.Options{})
	sess := session.Session{
		ID: "abc123", Name: "worker", Slug: "worker", Path: t.TempDir(), State: session.StateActive,
		Remote: "git@github.com:acme/widgets.git", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, store.NewSessionStore(harness.DB).Save(t.Context(), sess))
	return &engineFixture{handlers: engineops.New(zerolog.Nop(), harness.Engine), harness: harness, sess: sess}
}

func (f *engineFixture) open(context.Context) (*openedEngine, error) {
	f.opens++
	return &openedEngine{
		handlers: f.handlers,
		resolve: func(ctx context.Context, caller string) (sessionsvc.Caller, error) {
			return f.harness.Engine.Sessions().ResolveCaller(ctx, caller, "", "")
		},
		close: func() error { f.closes++; return nil },
	}, nil
}

func resultText(res *mcp.CallToolResult) string {
	return toolResultText(res, "")
}

func TestMCPServeAnswersTaskNextWithoutTheDesktop(t *testing.T) {
	f := newEngineFixture(t)
	created, err := f.handlers.TaskCreate(t.Context(), tasks.TaskCreateIn{
		Caller: f.sess.ID, Title: "Ship it", Type: "epic",
		Children: []map[string]any{{"title": "Write the code"}},
	})
	require.NoError(t, err)
	require.Len(t, created.Tasks, 2)
	epic, task := created.Tasks[0], created.Tasks[1]

	engine := newLocalEngine(t.Context(), f.open, "test", zerolog.Nop())
	cs := connectServeWith(t, t.TempDir(), engine, f.sess.ID, "tasks")

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "task_next",
		Arguments: map[string]any{"epicId": epic.ID, "assign": true},
	})
	require.NoError(t, err)
	require.False(t, res.IsError, resultText(res))
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var got hc.Item
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, task.ID, got.ID)
	assert.Equal(t, hc.StatusInProgress, got.Status)
	assert.Equal(t, f.sess.ID, got.SessionID)

	require.NoError(t, engine.Close())
	assert.Equal(t, 1, f.closes)
}

func TestMCPServeOpensTheEngineOnce(t *testing.T) {
	f := newEngineFixture(t)
	engine := newLocalEngine(t.Context(), f.open, "test", zerolog.Nop())
	cs := connectServeWith(t, t.TempDir(), engine, f.sess.ID, "tasks", "layout")

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "layout_get", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Equal(t, 0, f.opens, "a desktop tool needs no engine")

	for range 2 {
		res, err = cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "task_list", Arguments: map[string]any{}})
		require.NoError(t, err)
		require.False(t, res.IsError, resultText(res))
	}
	assert.Equal(t, 1, f.opens)
}

func TestMCPServeRefusesAnInvalidEngineCall(t *testing.T) {
	f := newEngineFixture(t)
	cs := connectServeWith(t, t.TempDir(), newLocalEngine(t.Context(), f.open, "test", zerolog.Nop()), f.sess.ID, "tasks")

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "task_next", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Contains(t, resultText(res), "invalid: ")
}

func TestMCPServeOpensTheEngineAgainAfterAFailure(t *testing.T) {
	f := newEngineFixture(t)
	fail := true
	open := func(ctx context.Context) (*openedEngine, error) {
		if fail {
			fail = false
			return nil, errors.New("database is locked")
		}
		return f.open(ctx)
	}
	cs := connectServeWith(t, t.TempDir(), newLocalEngine(t.Context(), open, "test", zerolog.Nop()), f.sess.ID, "tasks")

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "task_list", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Equal(t, "unavailable: open the hive engine: database is locked", resultText(res))

	res, err = cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "task_list", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.False(t, res.IsError, resultText(res))
}

func TestMCPServeForwardsAChatsDocumentsToTheDesktop(t *testing.T) {
	const chatID = "0b5f3c2e-7a41-4d8e-9c3a-2f6d1e8b4a90"
	f := newEngineFixture(t)
	fake := newFakeDesktop(t)
	dataDir := t.TempDir()
	require.NoError(t, desktopendpoint.Write(dataDir, desktopendpoint.Endpoint{URL: fake.srv.URL, PID: 1}))
	cs := connectServeWith(t, dataDir, newLocalEngine(t.Context(), f.open, "test", zerolog.Nop()), chatID, "drive")

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "document_put",
		Arguments: map[string]any{"path": "notes.md", "body": "# Notes"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError, resultText(res))

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.calls["/mcp/drive"], 1)
	assert.Equal(t, "document_put", fake.calls["/mcp/drive"][0].Tool)
	assert.Equal(t, chatID, fake.calls["/mcp/drive"][0].Args["caller"])
}

func TestMCPServeForwardsSessionControlToTheDesktop(t *testing.T) {
	cs := connectServe(t, t.TempDir(), "session-control")

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "session_delete", Arguments: map[string]any{"sessionId": "abc123"}})
	require.NoError(t, err)
	require.True(t, res.IsError)
	assert.Equal(t, "unavailable: Hive Desktop is not running", resultText(res))
}

func TestRunsInProcessIsEveryEngineOpButSessionControl(t *testing.T) {
	want := map[string]bool{
		"task_next": true, "message_create": true, "todo_list": true, "document_get": true, "session_list": true,
		"session_create": false, "session_delete": false, "layout_get": false, "applet_list": false,
	}
	for _, op := range catalog.All() {
		if inProcess, ok := want[op.Info().ToolName]; ok {
			assert.Equal(t, inProcess, runsInProcess(op.Info()), op.Info().ToolName)
			delete(want, op.Info().ToolName)
		}
	}
	assert.Empty(t, want, "tools missing from the catalog")
}
