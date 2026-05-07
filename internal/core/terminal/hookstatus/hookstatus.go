// Package hookstatus provides read/write access to hook-reported pane status entries.
// Hooks (external processes) write status to the KV store; the TUI reads it
// here before falling back to text-pattern detection.
package hookstatus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/core/terminal"
)

const keyPrefix = "hook.status"

type hookEntry struct {
	Status string `json:"status"`
}

// Store provides access to hook-written pane status via the KV store.
type Store struct {
	kv kv.KV
}

// New creates a hookstatus Store backed by the given KV store.
func New(kvStore kv.KV) *Store {
	return &Store{kv: kvStore}
}

// entryKey returns the KV key for a session+pane pair.
func entryKey(sessionID, paneID string) string {
	return fmt.Sprintf("%s:%s:%s", keyPrefix, sessionID, paneID)
}

// Write records a hook-reported status for a pane.
func (s *Store) Write(ctx context.Context, sessionID, paneID string, status terminal.Status) error {
	return s.kv.Set(ctx, entryKey(sessionID, paneID), hookEntry{Status: string(status)})
}

// Read returns the hook-reported status for a pane.
// Returns ("", false) if no entry exists.
func (s *Store) Read(ctx context.Context, sessionID, paneID string) (terminal.Status, bool) {
	var entry hookEntry
	if err := s.kv.Get(ctx, entryKey(sessionID, paneID), &entry); err != nil {
		return "", false
	}
	return terminal.Status(entry.Status), true
}

// Delete removes the hook status entry for a pane.
func (s *Store) Delete(ctx context.Context, sessionID, paneID string) error {
	return s.kv.Delete(ctx, entryKey(sessionID, paneID))
}

// IsFresh returns true if the entry was written within maxAge.
func (s *Store) IsFresh(ctx context.Context, sessionID, paneID string, maxAge time.Duration) bool {
	raw, err := s.kv.GetRaw(ctx, entryKey(sessionID, paneID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false
		}
		return false
	}
	return time.Since(raw.UpdatedAt) <= maxAge
}
