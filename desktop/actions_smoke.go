package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	coredb "github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/desktop"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
	"github.com/wailsapp/wails/v3/pkg/application"
	_ "modernc.org/sqlite"
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

// desktopSmokeMiddleware composes the two narrow test-only readers without
// changing the normal asset handler or exposing either route in production.
func desktopSmokeMiddleware(pipeline *pipelinedb.DB, core *coredb.DB) application.Middleware {
	return func(next http.Handler) http.Handler {
		return actionSmokeMiddleware(pipeline, core)(sourceToCommitSmokeMiddleware(pipeline)(next))
	}
}

func actionSmokeMiddleware(pipeline *pipelinedb.DB, core *coredb.DB) application.Middleware {
	return func(next http.Handler) http.Handler {
		if !actionSmokeHarnessEnabled() {
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
func readActionSmokeState(ctx context.Context, pipeline *pipelinedb.DB, core *coredb.DB) (actionSmokeState, error) {
	runID := desktopSmokeRunID()
	if runID == "" {
		return actionSmokeState{}, fmt.Errorf("action smoke run id is required")
	}
	live, err := readActionSmokeSnapshot(ctx, core.Conn(), pipeline.Conn(), runID)
	if err != nil {
		return actionSmokeState{}, err
	}

	reopenedCore, err := openSmokeReadOnly(filepath.Join(filepath.Dir(desktop.StateDir()), "hive.db"))
	if err != nil {
		return actionSmokeState{}, fmt.Errorf("reopen action smoke core database: %w", err)
	}
	defer func() { _ = reopenedCore.Close() }()
	reopenedPipeline, err := openSmokeReadOnly(filepath.Join(desktop.StateDir(), "desktop-pipeline.db"))
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
		ActionsPath:    desktop.ActionsPath(),
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
		SELECT id, action_id, key, status, COALESCE(last_error, ''),
		       COALESCE(stdout, ''), COALESCE(stderr, ''), COALESCE(result_json, 'null')
		FROM output_command
		WHERE action_id LIKE ? ESCAPE '\' AND key IN ('pr2841', 'iss1190')
		ORDER BY id`, likePrefix(prefix))
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

func actionSmokeHarnessEnabled() bool {
	if desktop.MockMode() != "action-smoke" || desktopSmokeRunID() == "" {
		return false
	}
	marker := strings.TrimSpace(os.Getenv(desktop.EnvE2EHarness))
	if len(marker) != 64 {
		return false
	}
	_, err := hex.DecodeString(marker)
	return err == nil
}

func desktopSmokeRunID() string {
	return strings.TrimSpace(os.Getenv("HIVE_DESKTOP_SMOKE_RUN_ID"))
}
