package queries

import (
	"context"
	"fmt"
)

const (
	defaultMinimumReclaimableRatio = 0.20
	defaultMinimumReclaimableBytes = 16 << 20
)

// CompactionPolicy defines when reclaimable SQLite pages justify a full-file
// rewrite.
type CompactionPolicy struct {
	MinimumReclaimableRatio float64
	MinimumReclaimableBytes int64
}

// DefaultCompactionPolicy requires both 20 percent and 16 MiB of the database
// to be reclaimable before compacting.
func DefaultCompactionPolicy() CompactionPolicy {
	return CompactionPolicy{
		MinimumReclaimableRatio: defaultMinimumReclaimableRatio,
		MinimumReclaimableBytes: defaultMinimumReclaimableBytes,
	}
}

// CompactionResult reports the space considered by Compact and whether VACUUM
// and its WAL checkpoint completed.
type CompactionResult struct {
	PageCount        int64
	FreelistCount    int64
	PageSize         int64
	ReclaimableBytes int64
	Compacted        bool
}

// Compact runs a full VACUUM when policy says the reclaimable space justifies
// it. Concurrent database work may cause SQLite to return a busy error.
func (db *DB) Compact(ctx context.Context, policy CompactionPolicy) (CompactionResult, error) {
	if err := policy.validate(); err != nil {
		return CompactionResult{}, err
	}
	if db.tx != nil {
		return CompactionResult{}, fmt.Errorf("compact database outside a transaction")
	}

	result := CompactionResult{}
	var err error
	if result.PageCount, err = db.pragmaInt64(ctx, "PRAGMA page_count", "page_count"); err != nil {
		return result, err
	}
	if result.FreelistCount, err = db.pragmaInt64(ctx, "PRAGMA freelist_count", "freelist_count"); err != nil {
		return result, err
	}
	if result.PageSize, err = db.pragmaInt64(ctx, "PRAGMA page_size", "page_size"); err != nil {
		return result, err
	}
	result.ReclaimableBytes = result.FreelistCount * result.PageSize

	if !policy.shouldCompact(result) {
		return result, nil
	}
	if _, err := db.conn.ExecContext(ctx, "VACUUM"); err != nil {
		return result, fmt.Errorf("vacuum database: %w", err)
	}

	// In WAL mode VACUUM writes the compact image into the sidecar. The main
	// file does not shrink until that image is checkpointed back.
	var busy, logPages, checkpointedPages int64
	if err := db.conn.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logPages, &checkpointedPages); err != nil {
		return result, fmt.Errorf("checkpoint compacted database: %w", err)
	}
	if busy != 0 {
		return result, fmt.Errorf("checkpoint compacted database: %d busy readers", busy)
	}

	result.Compacted = true
	return result, nil
}

func (db *DB) pragmaInt64(ctx context.Context, query, name string) (int64, error) {
	var value int64
	if err := db.conn.QueryRowContext(ctx, query).Scan(&value); err != nil {
		return 0, fmt.Errorf("read SQLite %s: %w", name, err)
	}
	return value, nil
}

func (policy CompactionPolicy) validate() error {
	if policy.MinimumReclaimableRatio < 0 || policy.MinimumReclaimableRatio > 1 {
		return fmt.Errorf("minimum reclaimable ratio must be between 0 and 1")
	}
	if policy.MinimumReclaimableBytes < 0 {
		return fmt.Errorf("minimum reclaimable bytes must not be negative")
	}
	return nil
}

func (policy CompactionPolicy) shouldCompact(result CompactionResult) bool {
	if result.PageCount <= 0 || result.FreelistCount <= 0 || result.PageSize <= 0 {
		return false
	}
	ratio := float64(result.FreelistCount) / float64(result.PageCount)
	return ratio >= policy.MinimumReclaimableRatio &&
		result.ReclaimableBytes >= policy.MinimumReclaimableBytes
}
