package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

// stateResetPath restores a mock-mode server instance to its post-startup
// baseline so a Playwright retry (fresh browser, surviving server) starts
// from the same state the first attempt saw. Unlike the read-only smoke
// routes it mutates state, so while it is available in every mock mode it
// still demands the Docker harness marker.
const stateResetPath = "/_e2e/reset"

// coreResetTables is the subset of hive.db tables the desktop's action path
// writes (launch-session creates sessions; publish-message creates messages,
// which readers acknowledge in message_reads). Other hivecore tables are
// never touched by the desktop app, so a reset leaves them alone.
var coreResetTables = []string{"message_reads", "messages", "sessions"}

// pristineFile is one mutable config file captured at harness construction
// (post-startup). A nil content means the file did not exist at boot and must
// be removed again on reset.
type pristineFile struct {
	path    string
	mode    fs.FileMode
	content []byte
}

// StateReset owns the post-startup baseline POST /_e2e/reset restores. Reset
// ordering: the pipeline database is wiped and reseeded in one transaction,
// the core database's action tables are wiped in a second transaction (a
// separate SQLite file cannot share the first), and the mutable config files
// (actions.yml, settings.yaml, flows/*) are rewritten last. The flows/actions
// watchers observe those writes and hot-reload exactly as they would for an
// external edit — including recording the same reload activity a hand edit
// would.
type StateReset struct {
	db       *queries.DB
	stores   *stores.Stores
	core     *sql.DB
	logger   zerolog.Logger
	mock     string
	flowsDir string
	files    []pristineFile
}

// NewStateResetHarness captures the reset baseline, or returns nil when the
// route must stay unmounted (live mode, or no valid harness marker). It must
// run after startup seeding — the mock inbox rows and actions.yml defaults —
// so the captured baseline is the post-boot state a fresh server would show.
//
// core is the raw connection to the vendored Hive action database (sessions,
// messages) — the caller passes app.App.HiveConn() rather than the vendored
// *coredb.DB itself.
func NewStateResetHarness(db *queries.DB, st *stores.Stores, core *sql.DB, logger zerolog.Logger) *StateReset {
	b, _ := settings.LoadBootstrap()
	mock := settings.MockMode()
	return NewStateResetHarnessForInstance(db, st, core, mock, settings.ResolvePaths(b, settings.ResolveOptions{MockMode: mock}), logger)
}

// NewStateResetHarnessForInstance uses the composition-root runtime snapshot.
func NewStateResetHarnessForInstance(db *queries.DB, st *stores.Stores, core *sql.DB, mock string, paths settings.Paths, logger zerolog.Logger) *StateReset {
	if mock == "" || !e2eHarnessMarkerValid() {
		return nil
	}
	r := &StateReset{db: db, stores: st, core: core, logger: logger, mock: mock, flowsDir: paths.FlowsDir}
	r.capture(paths.ActionsPath)
	r.capture(paths.SettingsPath)
	// SaveFlow/SaveLayout/SaveSidebar write per-flow files, so every file
	// under the flows directory is baseline state, layout siblings included.
	_ = filepath.WalkDir(r.flowsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		r.capture(path)
		return nil
	})
	return r
}

// capture records path's current content and permissions, or its absence.
func (r *StateReset) capture(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			r.logger.Warn().Err(err).Str("path", path).Msg("e2e reset baseline capture failed; treating file as absent")
		}
		r.files = append(r.files, pristineFile{path: path})
		return
	}
	mode := fs.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	r.files = append(r.files, pristineFile{path: path, mode: mode, content: content})
}

// Reset restores the post-startup baseline in-process, without closing either
// SQLite connection. The pipeline wipe and reseed share one transaction so a
// concurrent frontend read sees either the old state or the baseline, never
// an empty store; see the type comment for the full ordering.
func (r *StateReset) Reset(ctx context.Context) error {
	var reseed queries.Seeder
	switch r.mock {
	case "feed", "action-smoke":
		// The same deterministic fixture path main.go seeds at startup.
		reseed = mockSeeder{db: r.db, logs: r.stores.EventLog}
	}
	if err := r.db.ResetAllState(ctx, reseed); err != nil {
		return fmt.Errorf("reset pipeline database: %w", err)
	}
	if err := r.resetCoreTables(ctx); err != nil {
		return fmt.Errorf("reset core database: %w", err)
	}
	if err := r.restoreConfigFiles(); err != nil {
		return fmt.Errorf("restore config baseline: %w", err)
	}
	return nil
}

// resetCoreTables clears the desktop-action-owned hive.db tables in one
// transaction. The e2e servers boot with a private, empty hive.db, so the
// post-startup baseline for these tables is emptiness.
func (r *StateReset) resetCoreTables(ctx context.Context) error {
	if r.core == nil {
		return nil
	}
	tx, err := r.core.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	for _, table := range coreResetTables {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("clearing table %s: %w (rollback also failed: %w)", table, err, rbErr)
			}
			return fmt.Errorf("clearing table %s: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// restoreConfigFiles puts every captured config file back and removes files a
// test created after boot.
func (r *StateReset) restoreConfigFiles() error {
	captured := make(map[string]bool, len(r.files))
	for _, f := range r.files {
		captured[f.path] = true
	}
	// SaveFlow can mint new flows/<id>.yaml files (plus layout siblings) that
	// no captured entry covers; sweep them before restoring content. A walk
	// error (for example the directory never existed) needs no cleanup.
	err := filepath.WalkDir(r.flowsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || captured[path] {
			return nil
		}
		return os.Remove(path)
	})
	if err != nil {
		return fmt.Errorf("removing post-boot flow files: %w", err)
	}
	for _, f := range r.files {
		if err := restorePristineFile(f); err != nil {
			return err
		}
	}
	return nil
}

// restorePristineFile puts one captured file back: absent-at-boot files are
// removed, changed files rewritten. Unchanged files are left alone so a reset
// does not trigger needless watcher reloads.
func restorePristineFile(f pristineFile) error {
	if f.content == nil {
		if err := os.Remove(f.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("removing post-boot file %s: %w", f.path, err)
		}
		return nil
	}
	current, err := os.ReadFile(f.path)
	if err == nil && bytes.Equal(current, f.content) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return fmt.Errorf("recreating directory for %s: %w", f.path, err)
	}
	if err := os.WriteFile(f.path, f.content, f.mode); err != nil {
		return fmt.Errorf("restoring %s: %w", f.path, err)
	}
	return nil
}

// stateResetMiddleware mounts POST /_e2e/reset when the harness exists. A nil
// harness (live mode, or a missing/invalid marker) leaves the asset handler
// untouched, exactly like the smoke middlewares.
func stateResetMiddleware(reset *StateReset) application.Middleware {
	return func(next http.Handler) http.Handler {
		if reset == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Path != stateResetPath {
				next.ServeHTTP(w, req)
				return
			}
			if req.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err := reset.Reset(req.Context()); err != nil {
				reset.logger.Debug().Err(err).Msg("e2e state reset failed")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			reset.logger.Debug().Msg("e2e state reset restored post-startup baseline")
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// e2eHarnessMarkerValid reports whether HIVE_DESKTOP_E2E_HARNESS carries the
// 256-bit hex marker desktop/e2e/scripts/run-docker.sh mints. It keeps mock
// mode alone from enabling test-only routes.
func e2eHarnessMarkerValid() bool {
	marker := strings.TrimSpace(os.Getenv(settings.EnvE2EHarness))
	if len(marker) != 64 {
		return false
	}
	_, err := hex.DecodeString(marker)
	return err == nil
}
