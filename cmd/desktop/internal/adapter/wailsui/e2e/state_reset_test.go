package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	appstores "github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/internal/domain/messaging"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/store"
	coredb "github.com/colonyops/hive/internal/store/db"
)

func TestStateResetHarnessUnavailableOutsideMockHarness(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mode   string
		marker string
	}{
		{name: "live mode", mode: "", marker: smokeHarnessMarker},
		{name: "missing harness marker", mode: "feed"},
		{name: "invalid harness marker", mode: "feed", marker: "not-a-256-bit-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(settings.EnvMockMode, tc.mode)
			t.Setenv(settings.EnvE2EHarness, tc.marker)
			harness := NewStateResetHarness(nil, nil, nil, zerolog.Nop())
			assert.Nil(t, harness)
			h := stateResetMiddleware(harness)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))
			r := httptest.NewRecorder()
			h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, stateResetPath, nil))
			assert.Equal(t, http.StatusTeapot, r.Code)
		})
	}
}

func TestStateResetPOSTOnly(t *testing.T) {
	setStateResetEnv(t, "feed")
	db := openStateResetPipelineDB(t)
	harness := NewStateResetHarness(db, appstores.New(db, appstores.Options{}), nil, zerolog.Nop())
	require.NotNil(t, harness)
	h := stateResetMiddleware(harness)(http.NotFoundHandler())
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, stateResetPath, nil))
	assert.Equal(t, http.StatusMethodNotAllowed, r.Code)
	assert.Equal(t, http.MethodPost, r.Header().Get("Allow"))
}

func TestStateResetRestoresFreshlySeededBaseline(t *testing.T) {
	root := setStateResetEnv(t, "feed")
	ctx := context.Background()

	// Config baseline as the e2e launcher lays it out: a private flow file and
	// actions.yml; settings.yaml deliberately absent, as on a fresh boot.
	flowsDir := settings.FlowsDir()
	require.NoError(t, os.MkdirAll(flowsDir, 0o755))
	flowPath := filepath.Join(flowsDir, "frontend-triage.yaml")
	pristineFlow := "id: frontend-triage\nname: Frontend Triage\n"
	require.NoError(t, os.WriteFile(flowPath, []byte(pristineFlow), 0o644))
	pristineActions := "version: 1\nactions: []\n"
	require.NoError(t, os.WriteFile(settings.ActionsPath(), []byte(pristineActions), 0o644))

	db := openStateResetPipelineDB(t)
	core, err := coredb.Open(root, coredb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, core.Close()) })
	st := appstores.New(db, appstores.Options{})
	require.NoError(t, seedMockInboxItems(t.Context(), db, st.EventLog)) // feed mode's startup seeding

	harness := NewStateResetHarness(db, appstores.New(db, appstores.Options{}), core.Conn(), zerolog.Nop())
	require.NotNil(t, harness)
	h := SmokeMiddleware(db, appstores.New(db, appstores.Options{}), core.Conn(), harness, nil)(http.NotFoundHandler())

	var itemID, revision int64
	require.NoError(t, db.Conn().QueryRowContext(ctx,
		`SELECT id, revision FROM inbox_item WHERE external_id = 'pr2841'`).Scan(&itemID, &revision))
	_, err = st.InboxItems.SetUnread(ctx, itemID, revision, false)
	require.NoError(t, err)
	_, err = st.EventLog.Append(ctx, "source:"+MockFlowID+"/"+MockSourceNodeID, "extra", []byte(`{"mutated":true}`))
	require.NoError(t, err)
	require.NoError(t, db.CommitConsumerOffset(ctx, queries.CommitConsumerOffsetParams{Consumer: "frontend", Offset: 5}))
	require.NoError(t, db.UpsertSourceHead(ctx, queries.UpsertSourceHeadParams{Topic: "source:x", Key: "k", Payload: []byte(`{}`)}))
	command, created, err := st.OutputCommands.Confirm(ctx, "smoke-shell", "pr2841", []byte(`{}`), models.ItemRef{})
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, st.OutputCommands.MarkDone(ctx, command.ID, `{"ok":true}`, "out", "err"))
	_, err = st.ActivityEvents.Append(ctx, appstores.ActivityEventCreate{Category: "action", Severity: "info", Title: "mutated"})
	require.NoError(t, err)
	_, err = st.Jobs.Insert(ctx, appstores.JobCreate{Status: "done", Label: "mutated"})
	require.NoError(t, err)
	require.NoError(t, db.InsertNodeRun(ctx, queries.InsertNodeRunParams{FlowID: MockFlowID, NodeID: MockSourceNodeID, Ok: 1, EndedAt: time.Now().UnixMilli()}))

	// Mutate the core action tables the way a launch-session/publish-message
	// action does.
	now := time.Now()
	require.NoError(t, store.NewSessionStore(core).Save(ctx, session.Session{
		ID: "s1", Name: "smoke-session", Slug: "smoke-session", Remote: "file:///fixture",
		State: session.StateActive, CreatedAt: now, UpdatedAt: now,
	}))
	_, err = store.NewMessageStore(core, 0).Publish(ctx, messaging.Message{Payload: "mutated", Sender: "test"}, []string{"smoke.reset"})
	require.NoError(t, err)

	// Mutate config: rewrite tracked files, mint an untracked flow, create
	// settings.yaml.
	require.NoError(t, os.WriteFile(flowPath, []byte("id: frontend-triage\nname: Mutated\n"), 0o644))
	extraFlow := filepath.Join(flowsDir, "minted-by-test.yaml")
	require.NoError(t, os.WriteFile(extraFlow, []byte("id: minted-by-test\n"), 0o644))
	require.NoError(t, os.WriteFile(settings.ActionsPath(), []byte("version: 1\nactions:\n  - id: mutated\n"), 0o644))
	cfg := settings.DefaultSettings()
	cfg.Polling.Interval = settings.Duration(2 * time.Minute)
	require.NoError(t, settings.SaveSettings(cfg))

	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, stateResetPath, nil))
	require.Equal(t, http.StatusNoContent, r.Code, r.Body.String())

	// The pipeline database now equals a freshly seeded instance — including
	// restarted AUTOINCREMENT ids and event offsets.
	fresh, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, fresh.Close()) })
	require.NoError(t, seedMockInboxItems(t.Context(), fresh, appstores.New(fresh, appstores.Options{}).EventLog))
	assert.Equal(t, dumpStableState(t, fresh), dumpStableState(t, db))

	// The core action tables are empty again, as after a fresh boot.
	assert.Zero(t, countRows(t, core, "sessions"))
	assert.Zero(t, countRows(t, core, "messages"))
	assert.Zero(t, countRows(t, core, "message_reads"))

	// Config files are back to the captured baseline.
	restoredFlow, err := os.ReadFile(flowPath)
	require.NoError(t, err)
	assert.Equal(t, pristineFlow, string(restoredFlow))
	restoredActions, err := os.ReadFile(settings.ActionsPath())
	require.NoError(t, err)
	assert.Equal(t, pristineActions, string(restoredActions))
	assert.NoFileExists(t, extraFlow)
	assert.NoFileExists(t, settings.SettingsPath())
}

func TestStateResetPipelineModeWipesWithoutReseeding(t *testing.T) {
	setStateResetEnv(t, "pipeline")
	ctx := context.Background()
	db := openStateResetPipelineDB(t)

	// The pipeline smoke fixture's own server-side append plus a command, the
	// state a source-to-commit run leaves behind.
	require.NoError(t, appendSourceToCommitSmokeItems(ctx, appstores.New(db, appstores.Options{}), "", nil))
	_, created, err := appstores.New(db, appstores.Options{}).OutputCommands.Confirm(ctx, "launch", "smoke-pr", []byte(`{}`), models.ItemRef{})
	require.NoError(t, err)
	require.True(t, created)

	harness := NewStateResetHarness(db, appstores.New(db, appstores.Options{}), nil, zerolog.Nop())
	require.NotNil(t, harness)
	h := SmokeMiddleware(db, appstores.New(db, appstores.Options{}), nil, harness, nil)(http.NotFoundHandler())
	r := httptest.NewRecorder()
	h.ServeHTTP(r, httptest.NewRequest(http.MethodPost, stateResetPath, nil))
	require.Equal(t, http.StatusNoContent, r.Code, r.Body.String())

	// Pipeline mode seeds nothing at startup, so its baseline is emptiness.
	for _, table := range []string{"event_log", "inbox_item", "inbox_event", "feed_membership_claim", "output_command", "consumer_offset"} {
		assert.Zero(t, countRows(t, db, table), table)
	}
	tail, err := appstores.New(db, appstores.Options{}).EventLog.TailOffset(ctx)
	require.NoError(t, err)
	assert.Zero(t, tail, "sequence reset must restart event offsets from 1")
}

// setStateResetEnv points every mutable path at a private temp root, mirroring
// how cmd/desktop/e2e/scripts/serve.sh isolates each server instance.
func setStateResetEnv(t *testing.T, mode string) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv(settings.EnvMockMode, mode)
	t.Setenv(settings.EnvE2EHarness, smokeHarnessMarker)
	t.Setenv(settings.EnvDataDir, root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv(settings.EnvFlowsDir, filepath.Join(root, "flows"))
	t.Setenv(settings.EnvActionsPath, filepath.Join(root, "actions.yml"))
	return root
}

func openStateResetPipelineDB(t *testing.T) *queries.DB {
	t.Helper()
	db, err := queries.Open(t.Context(), settings.StateDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

// dumpStableState reads every reset-scoped table's deterministic columns:
// identity and content, but not wall-clock timestamps, which necessarily
// differ between two seeding runs. Including ids, event offsets, and
// sqlite_sequence proves the reset restarts AUTOINCREMENT counters exactly
// like a fresh database.
func dumpStableState(t *testing.T, db *queries.DB) map[string][][]string {
	t.Helper()
	queries := map[string]string{
		"event_log":             `SELECT "offset", topic, key, snapshot, source_kind, source_scope, payload FROM event_log ORDER BY "offset"`,
		"consumer_offset":       `SELECT consumer, "offset" FROM consumer_offset ORDER BY consumer`,
		"source_head":           `SELECT topic, key, payload FROM source_head ORDER BY topic, key`,
		"output_command":        `SELECT id, action_id, key, status, attempts, is_rerun FROM output_command ORDER BY id`,
		"node_run":              `SELECT flow_id, node_id, ok, in_count, out_count, drop_count FROM node_run ORDER BY flow_id, node_id`,
		"activity_event":        `SELECT id, category, severity, title, body, source FROM activity_event ORDER BY id`,
		"job":                   `SELECT id, status, label, step, action_id, target, error FROM job ORDER BY id`,
		"inbox_item":            `SELECT id, profile_id, source_kind, source_scope, external_id, title, url, revision, unread, lifecycle, archived_at IS NULL, ignored_at IS NULL FROM inbox_item ORDER BY id`,
		"inbox_event":           `SELECT id, item_id, kind, transition, attention, summary FROM inbox_event ORDER BY id`,
		"feed_membership_claim": `SELECT profile_id, feed_id, item_id, source_id FROM feed_membership_claim ORDER BY profile_id, feed_id, item_id, source_id`,
		"sqlite_sequence":       `SELECT name, seq FROM sqlite_sequence ORDER BY name`,
	}
	out := make(map[string][][]string, len(queries))
	for table, query := range queries {
		out[table] = dumpRows(t, db, query)
	}
	return out
}

func dumpRows(t *testing.T, db *queries.DB, query string) [][]string {
	t.Helper()
	rows, err := db.Conn().QueryContext(context.Background(), query)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	columns, err := rows.Columns()
	require.NoError(t, err)
	out := [][]string{}
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		require.NoError(t, rows.Scan(pointers...))
		row := make([]string, len(columns))
		for i, value := range values {
			if b, ok := value.([]byte); ok {
				row[i] = string(b)
				continue
			}
			row[i] = fmt.Sprint(value)
		}
		out = append(out, row)
	}
	require.NoError(t, rows.Err())
	return out
}
