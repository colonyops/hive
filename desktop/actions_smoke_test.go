package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/colonyops/hive/internal/core/session"
	coredb "github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/desktop"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

func TestActionSmokeMiddlewareUnavailableOutsideDedicatedHarness(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mode   string
		runID  string
		marker string
	}{
		{name: "wrong mode", mode: "feed", runID: "unit", marker: smokeHarnessMarker},
		{name: "missing run ID", mode: "action-smoke", marker: smokeHarnessMarker},
		{name: "missing harness marker", mode: "action-smoke", runID: "unit"},
		{name: "invalid harness marker", mode: "action-smoke", runID: "unit", marker: "not-a-256-bit-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(desktop.EnvMockMode, tc.mode)
			t.Setenv("HIVE_DESKTOP_SMOKE_RUN_ID", tc.runID)
			t.Setenv(desktop.EnvE2EHarness, tc.marker)
			h := actionSmokeMiddleware(nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, actionSmokePath, nil))
			assert.Equal(t, http.StatusTeapot, r.Code)
		})
	}
}

const smokeHarnessMarker = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestActionSmokeMiddlewareGETOnly(t *testing.T) {
	pipeline, core := newActionSmokeDatabases(t)
	t.Setenv(desktop.EnvMockMode, "action-smoke")
	t.Setenv("HIVE_DESKTOP_SMOKE_RUN_ID", "unit")
	t.Setenv(desktop.EnvE2EHarness, smokeHarnessMarker)
	h := actionSmokeMiddleware(pipeline, core)(http.NotFoundHandler())
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, actionSmokePath, nil))
	assert.Equal(t, http.StatusMethodNotAllowed, r.Code)
	assert.Equal(t, http.MethodGet, r.Header().Get("Allow"))
}

func TestActionSmokeMiddlewareReadsOnlyCurrentRunWithoutMutation(t *testing.T) {
	pipeline, core := newActionSmokeDatabases(t)
	t.Setenv(desktop.EnvMockMode, "action-smoke")
	t.Setenv("HIVE_DESKTOP_SMOKE_RUN_ID", "unit")
	t.Setenv(desktop.EnvE2EHarness, smokeHarnessMarker)
	t.Setenv(desktop.EnvActionsPath, filepath.Join(t.TempDir(), "private-actions.yml"))
	ctx := context.Background()

	sessionStore := stores.NewSessionStore(core)
	now := time.Now()
	require.NoError(t, sessionStore.Save(ctx, session.Session{ID: "kept", Name: "smoke-unit-template", Slug: "smoke-unit-template", Remote: "file:///fixture", State: session.StateActive, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, sessionStore.Save(ctx, session.Session{ID: "hidden", Name: "smoke-other-template", Slug: "smoke-other-template", Remote: "file:///other", State: session.StateActive, CreatedAt: now, UpdatedAt: now}))
	_, err := stores.NewMessageStore(core, 0).Publish(ctx, messaging.Message{Payload: "rendered", Sender: "hive-desktop"}, []string{"smoke.unit"})
	require.NoError(t, err)
	_, err = stores.NewMessageStore(core, 0).Publish(ctx, messaging.Message{Payload: "hidden", Sender: "other"}, []string{"smoke.other"})
	require.NoError(t, err)

	kept, created, err := pipeline.ConfirmOutputCommand(ctx, "smoke-unit-shell", "pr2841", []byte(`{}`))
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, pipeline.MarkOutputCommandDone(ctx, kept.ID, `{"message":{"topic":"smoke.unit","sender":"hive-desktop"}}`, "out", "err"))
	other, created, err := pipeline.ConfirmOutputCommand(ctx, "smoke-other-shell", "pr2841", []byte(`{}`))
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, pipeline.MarkOutputCommandFailed(ctx, other.ID, "hidden failure"))

	h := actionSmokeMiddleware(pipeline, core)(http.NotFoundHandler())
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, actionSmokePath, nil))
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	var state actionSmokeState
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &state))
	assert.Equal(t, "unit", state.RunID)
	assert.Equal(t, desktop.ActionsPath(), state.ActionsPath)
	require.Len(t, state.Sessions, 1)
	assert.Equal(t, "kept", state.Sessions[0].ID)
	require.Len(t, state.Messages, 1)
	assert.Equal(t, "rendered", state.Messages[0].Payload)
	require.Len(t, state.OutputCommands, 1)
	assert.Equal(t, "smoke-unit-shell", state.OutputCommands[0].ActionID)
	assert.JSONEq(t, `{"message":{"topic":"smoke.unit","sender":"hive-desktop"}}`, string(state.OutputCommands[0].Result))

	// readActionSmokeState returns the rows queried through independent
	// mode=ro connections, not merely a successful reopen probe.
	reopened, err := readActionSmokeState(ctx, pipeline, core)
	require.NoError(t, err)
	require.Len(t, reopened.Sessions, 1)
	assert.Equal(t, actionSmokeSession{ID: "kept", Name: "smoke-unit-template", Remote: "file:///fixture"}, reopened.Sessions[0])
	require.Len(t, reopened.Messages, 1)
	assert.Equal(t, "smoke.unit", reopened.Messages[0].Topic)
	assert.Equal(t, "rendered", reopened.Messages[0].Payload)
	assert.Equal(t, "hive-desktop", reopened.Messages[0].Sender)
	require.Len(t, reopened.OutputCommands, 1)
	assert.Equal(t, "smoke-unit-shell", reopened.OutputCommands[0].ActionID)
	assert.Equal(t, "done", reopened.OutputCommands[0].Status)
	assert.JSONEq(t, `{"message":{"topic":"smoke.unit","sender":"hive-desktop"}}`, string(reopened.OutputCommands[0].Result))

	// The GET endpoint is observational: it must not append messages, sessions,
	// or commands while verifying its independent read-only reopen.
	assert.Equal(t, int64(2), countRows(t, core, "sessions"))
	assert.Equal(t, int64(2), countRows(t, core, "messages"))
	assert.Equal(t, int64(2), countRows(t, pipeline, "output_command"))
}

func newActionSmokeDatabases(t *testing.T) (*pipelinedb.DB, *coredb.DB) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HIVE_DATA_DIR", root)
	pipeline, err := pipelinedb.Open(desktop.StateDir(), pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	core, err := coredb.Open(root, coredb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pipeline.Close()); require.NoError(t, core.Close()) })
	return pipeline, core
}

func countRows(t *testing.T, database interface{ Conn() *sql.DB }, table string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, database.Conn().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&count))
	return count
}
