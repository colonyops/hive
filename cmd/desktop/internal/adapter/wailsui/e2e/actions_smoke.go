package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	_ "modernc.org/sqlite"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

// actionSmokePath is intentionally available only in the dedicated Docker e2e
// mode. It is read-only: the browser must invoke normal generated Wails
// methods/UI to create actions or execute commands; this endpoint merely
// verifies persisted records afterwards.
const actionSmokePath = "/_e2e/actions"

type actionSmokeSession struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Remote string `json:"remote"`
}

type actionSmokeMessage struct {
	ID        string `json:"id"`
	Topic     string `json:"topic"`
	Payload   string `json:"payload"`
	Sender    string `json:"sender"`
	SessionID string `json:"sessionId"`
}

type actionSmokeCommand struct {
	ID        int64           `json:"id"`
	ActionID  string          `json:"actionId"`
	Key       string          `json:"key"`
	Status    string          `json:"status"`
	LastError string          `json:"lastError"`
	Stdout    string          `json:"stdout"`
	Stderr    string          `json:"stderr"`
	Result    json.RawMessage `json:"result"`
}

type actionSmokeState struct {
	RunID          string               `json:"runId"`
	ActionsPath    string               `json:"actionsPath"`
	Sessions       []actionSmokeSession `json:"sessions"`
	Messages       []actionSmokeMessage `json:"messages"`
	OutputCommands []actionSmokeCommand `json:"outputCommands"`
}

// SmokeMiddleware composes the narrow test-only routes — the two
// smoke readers plus the /_e2e/reset harness — without changing the normal
// asset handler or exposing any of them in production.
//
// core is the raw connection to the shared Hive action database (sessions,
// messages) — the caller passes app.App.HiveConn() rather than the
// *coredb.DB itself.
func SmokeMiddleware(pipeline *queries.DB, st *stores.Stores, core *sql.DB, reset *StateReset, onAppended func(nextOffset int64)) application.Middleware {
	mock := ""
	if reset != nil {
		mock = reset.mock
	}
	return func(next http.Handler) http.Handler {
		return actionSmokeMiddleware(pipeline, core, mock)(sourceToCommitSmokeMiddleware(pipeline, st, mock, onAppended)(stateResetMiddleware(reset)(next)))
	}
}

func actionSmokeMiddleware(pipeline *queries.DB, core *sql.DB, mock string) application.Middleware {
	return func(next http.Handler) http.Handler {
		if !actionSmokeHarnessEnabled(mock) {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != actionSmokePath {
				next.ServeHTTP(w, r)
				return
			}
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", http.MethodGet)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			state, err := readActionSmokeState(r.Context(), pipeline, core)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(state); err != nil {
				return
			}
		})
	}
}

// readActionSmokeState reads only rows owned by this server's injected run ID.
// It compares the live snapshot to separately reopened, read-only SQLite
// connections and returns the reopened rows. Neither reopen can migrate,
// recover commands, or create a database file.
func readActionSmokeState(ctx context.Context, pipeline *queries.DB, core *sql.DB) (actionSmokeState, error) {
	runID := desktopSmokeRunID()
	if runID == "" {
		return actionSmokeState{}, fmt.Errorf("action smoke run id is required")
	}
	live, err := readActionSmokeSnapshot(ctx, core, pipeline.Conn(), runID)
	if err != nil {
		return actionSmokeState{}, err
	}

	reopenedCore, err := openSmokeReadOnly(filepath.Join(filepath.Dir(settings.StateDir()), "hive.db"))
	if err != nil {
		return actionSmokeState{}, fmt.Errorf("reopen action smoke core database: %w", err)
	}
	defer func() { _ = reopenedCore.Close() }()
	reopenedPipeline, err := openSmokeReadOnly(filepath.Join(settings.StateDir(), "desktop-pipeline.db"))
	if err != nil {
		return actionSmokeState{}, fmt.Errorf("reopen action smoke pipeline database: %w", err)
	}
	defer func() { _ = reopenedPipeline.Close() }()

	reopened, err := readActionSmokeSnapshot(ctx, reopenedCore, reopenedPipeline, runID)
	if err != nil {
		return actionSmokeState{}, fmt.Errorf("read reopened action smoke state: %w", err)
	}
	if !reflect.DeepEqual(live, reopened) {
		return actionSmokeState{}, fmt.Errorf("live and reopened action smoke snapshots differ")
	}
	return reopened, nil
}

// readActionSmokeSnapshot queries the filtered durable rows from the supplied
// core and pipeline connections. It is used for both live and reopened reads.
func readActionSmokeSnapshot(ctx context.Context, core, pipeline *sql.DB, runID string) (actionSmokeState, error) {
	prefix := "smoke-" + runID + "-"
	topic := "smoke." + runID
	state := actionSmokeState{
		RunID:          runID,
		ActionsPath:    settings.ActionsPath(),
		Sessions:       []actionSmokeSession{},
		Messages:       []actionSmokeMessage{},
		OutputCommands: []actionSmokeCommand{},
	}

	sessionRows, err := core.QueryContext(ctx, `
		SELECT id, name, remote FROM sessions
		WHERE name LIKE ? ESCAPE '\'
		ORDER BY created_at, id`, likePrefix(prefix))
	if err != nil {
		return actionSmokeState{}, fmt.Errorf("read action smoke sessions: %w", err)
	}
	defer func() { _ = sessionRows.Close() }()
	for sessionRows.Next() {
		var row actionSmokeSession
		if err := sessionRows.Scan(&row.ID, &row.Name, &row.Remote); err != nil {
			return actionSmokeState{}, fmt.Errorf("scan action smoke session: %w", err)
		}
		state.Sessions = append(state.Sessions, row)
	}
	if err := sessionRows.Err(); err != nil {
		return actionSmokeState{}, fmt.Errorf("iterate action smoke sessions: %w", err)
	}

	messageRows, err := core.QueryContext(ctx, `
		SELECT id, topic, payload, COALESCE(sender, ''), COALESCE(session_id, '')
		FROM messages WHERE topic = ? ORDER BY created_at, id`, topic)
	if err != nil {
		return actionSmokeState{}, fmt.Errorf("read action smoke messages: %w", err)
	}
	defer func() { _ = messageRows.Close() }()
	for messageRows.Next() {
		var row actionSmokeMessage
		if err := messageRows.Scan(&row.ID, &row.Topic, &row.Payload, &row.Sender, &row.SessionID); err != nil {
			return actionSmokeState{}, fmt.Errorf("scan action smoke message: %w", err)
		}
		state.Messages = append(state.Messages, row)
	}
	if err := messageRows.Err(); err != nil {
		return actionSmokeState{}, fmt.Errorf("iterate action smoke messages: %w", err)
	}

	commandRows, err := pipeline.QueryContext(ctx, `
		SELECT oc.id, oc.action_id, oc.key, oc.status, COALESCE(oc.last_error, ''),
		       COALESCE((SELECT group_concat(text, char(10)) FROM (
		           SELECT text FROM action_run_log WHERE command_id = oc.id AND stream = 'stdout' ORDER BY id)), ''),
		       COALESCE((SELECT group_concat(text, char(10)) FROM (
		           SELECT text FROM action_run_log WHERE command_id = oc.id AND stream = 'stderr' ORDER BY id)), ''),
		       COALESCE(oc.result_json, 'null')
		FROM output_command oc
		WHERE oc.action_id LIKE ? ESCAPE '\' AND oc.key IN ('pr2841', 'iss1190')
		ORDER BY oc.id`, likePrefix(prefix))
	if err != nil {
		return actionSmokeState{}, fmt.Errorf("read action smoke commands: %w", err)
	}
	defer func() { _ = commandRows.Close() }()
	for commandRows.Next() {
		var row actionSmokeCommand
		var result string
		if err := commandRows.Scan(&row.ID, &row.ActionID, &row.Key, &row.Status, &row.LastError, &row.Stdout, &row.Stderr, &result); err != nil {
			return actionSmokeState{}, fmt.Errorf("scan action smoke command: %w", err)
		}
		if !json.Valid([]byte(result)) {
			return actionSmokeState{}, fmt.Errorf("decode action smoke command %d result", row.ID)
		}
		row.Result = json.RawMessage(result)
		state.OutputCommands = append(state.OutputCommands, row)
	}
	if err := commandRows.Err(); err != nil {
		return actionSmokeState{}, fmt.Errorf("iterate action smoke commands: %w", err)
	}
	return state, nil
}

func openSmokeReadOnly(path string) (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
}

func likePrefix(prefix string) string {
	return strings.NewReplacer(`\\`, `\\\\`, `%`, `\\%`, `_`, `\\_`).Replace(prefix) + "%"
}

func actionSmokeHarnessEnabled(mock string) bool {
	return mock == "action-smoke" && desktopSmokeRunID() != "" && e2eHarnessMarkerValid()
}

func desktopSmokeRunID() string {
	return strings.TrimSpace(os.Getenv("HIVE_DESKTOP_SMOKE_RUN_ID"))
}
