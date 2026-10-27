package store

import (
	"context"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/store/db"
)

// DriveUploadStore implements drive.UploadStore using SQLite.
type DriveUploadStore struct {
	db *db.DB
}

var _ drive.UploadStore = (*DriveUploadStore)(nil)

func NewDriveUploadStore(db *db.DB) *DriveUploadStore {
	return &DriveUploadStore{db: db}
}

func (s *DriveUploadStore) Record(ctx context.Context, u drive.Upload) error {
	err := s.db.Ctx(ctx).UpsertDriveUpload(ctx, db.UpsertDriveUploadParams{
		ID:         u.ID,
		Server:     u.Server,
		Space:      u.Space.Key,
		Path:       u.Path,
		Sha256:     u.SHA256,
		UploadedAt: u.UploadedAt.UnixNano(),
	})
	if err != nil {
		return fmt.Errorf("record drive upload: %w", err)
	}
	return nil
}

func (s *DriveUploadStore) List(ctx context.Context, server string, space drive.Space) (map[string]drive.Upload, error) {
	rows, err := s.db.Ctx(ctx).ListDriveUploads(ctx, db.ListDriveUploadsParams{Server: server, Space: space.Key})
	if err != nil {
		return nil, fmt.Errorf("list drive uploads: %w", err)
	}
	out := make(map[string]drive.Upload, len(rows))
	for _, row := range rows {
		out[row.Path] = drive.Upload{
			ID:         row.ID,
			Server:     row.Server,
			Space:      space,
			Path:       row.Path,
			SHA256:     row.Sha256,
			UploadedAt: time.Unix(0, row.UploadedAt),
		}
	}
	return out, nil
}
