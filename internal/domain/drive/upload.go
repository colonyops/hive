package drive

import (
	"context"
	"time"
)

// Upload records the last version of a document that was pushed to a drive
// server. It only informs listings; nothing acts on it.
type Upload struct {
	ID         string
	Server     string // driveapi.Host form
	Space      Space
	Path       string
	SHA256     string
	UploadedAt time.Time
}

type UploadStore interface {
	// Record keeps one row per (server, space, path): a later push replaces it.
	Record(ctx context.Context, u Upload) error
	// List returns the uploads of one space on one server, keyed by path.
	List(ctx context.Context, server string, space Space) (map[string]Upload, error)
}
