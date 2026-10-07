package usageanalytics

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	domain "github.com/colonyops/hive/internal/domain/usageanalytics"
	"github.com/colonyops/hive/internal/platform/sqlite"
	"github.com/colonyops/hive/internal/store/migrate"
	"github.com/colonyops/hive/internal/store/usageanalytics/queries"
	"github.com/google/uuid"
)

const (
	Filename      = "usage-analytics.db"
	RetentionDays = 90
)

//go:embed migrations/*.up.sql
var migrations embed.FS

type Store struct{ db *sql.DB }

type Summary struct {
	CLICommands    int64 `json:"cliCommands"`
	HiveSessions   int64 `json:"hiveSessions"`
	TerminalStarts int64 `json:"terminalStarts"`
}

func Open(ctx context.Context, dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	db, err := sqlite.Open(ctx, filepath.Join(dataDir, Filename), options(false))
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = db.Close()
		}
	}()
	migrationFS, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return nil, err
	}
	if err := migrate.Up(ctx, db, migrationFS); err != nil {
		return nil, err
	}
	if err := queries.New(db).InitializeMetadata(ctx, uuid.NewString()); err != nil {
		return nil, err
	}
	ok = true
	return &Store{db: db}, nil
}

func options(readOnly bool) sqlite.Options {
	return sqlite.Options{ReadOnly: readOnly, MaxOpenConns: 2, MaxIdleConns: 2, BusyTimeout: 5 * time.Second}
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) WriteBatch(ctx context.Context, batch []domain.Captured) error {
	return s.transaction(ctx, func(q *queries.Queries) error {
		meta, err := q.Metadata(ctx)
		if err != nil {
			return err
		}
		for _, e := range batch {
			if err := e.Validate(); err != nil {
				return err
			}
			if e.OccurredAt().UnixNano() <= meta.ClearCutoffNs {
				continue
			}
			if err := q.InsertEvent(ctx, queries.InsertEventParams{
				EventID: e.ID, OccurredAtMs: e.OccurredAt().UnixMilli(), RecordedAtMs: time.Now().UnixMilli(),
				Name: e.Name(), SchemaVersion: 1, InstallationID: meta.InstallationID, RunID: e.RunID,
				Surface: e.Surface, AppVersion: e.AppVersion, ReleaseChannel: e.ReleaseChannel, PropertiesJson: e.Properties(),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) transaction(ctx context.Context, fn func(*queries.Queries) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(queries.New(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Summary(ctx context.Context, now time.Time) (Summary, error) {
	rows, err := queries.New(s.db).Counts(ctx, queries.CountsParams{OccurredAtMs: now.UTC().AddDate(0, 0, -RetentionDays).UnixMilli(), OccurredAtMs_2: now.UnixMilli()})
	if err != nil {
		return Summary{}, err
	}
	var result Summary
	for _, row := range rows {
		switch row.Name {
		case domain.CommandCompletedName:
			result.CLICommands = row.Total
		case domain.SessionCreatedName:
			result.HiveSessions = row.Total
		case domain.TerminalStartedName:
			result.TerminalStarts = row.Total
		}
	}
	return result, nil
}

// ReadSummary does not create or migrate history when collection is disabled.
func ReadSummary(ctx context.Context, dataDir string, now time.Time) (Summary, error) {
	path := filepath.Join(dataDir, Filename)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return Summary{}, nil
	} else if err != nil {
		return Summary{}, err
	}
	db, err := sqlite.Open(ctx, path, options(true))
	if err != nil {
		return Summary{}, err
	}
	defer func() { _ = db.Close() }()
	return (&Store{db: db}).Summary(ctx, now)
}

// Clear serializes the capture cutoff with every writer, including other processes.
func (s *Store) Clear(ctx context.Context) (time.Time, error) {
	var cutoff time.Time
	err := s.transaction(ctx, func(q *queries.Queries) error {
		cutoff = time.Now().UTC()
		if err := q.SetClearCutoff(ctx, queries.SetClearCutoffParams{MAX: cutoff.UnixNano(), InstallationID: uuid.NewString()}); err != nil {
			return err
		}
		return q.ClearEvents(ctx)
	})
	return cutoff, err
}

func (s *Store) Prune(ctx context.Context, now time.Time) error {
	for {
		count, err := queries.New(s.db).Prune(ctx, now.AddDate(0, 0, -RetentionDays).UnixMilli())
		if err != nil {
			return fmt.Errorf("prune usage history: %w", err)
		}
		if count < 500 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}
