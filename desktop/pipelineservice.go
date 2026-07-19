package main

import (
	"context"

	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

// PipelineService is the Wails service exposing the desktop pipeline's
// event log to the frontend. All data comes from the injected
// *pipelinedb.DB; this type is wire glue only, matching FeedService's
// thin-glue style.
//
// Phase 2 ships only this minimal offset read/commit pair — enough for a
// consumer to tail the log and checkpoint its position. Phase 3 extends
// Commit into the full protocol (outputs/discards/feed_item/node_run) once
// the frontend graph runtime consumes the log; this service's surface will
// grow then, not change shape now.
type PipelineService struct {
	db *pipelinedb.DB
}

func NewPipelineService(db *pipelinedb.DB) *PipelineService {
	return &PipelineService{db: db}
}

// ReadFrom returns up to limit event_log rows appended after offset, in
// ascending order.
func (s *PipelineService) ReadFrom(offset int64, limit int) ([]pipelinedb.Msg, error) {
	msgs, _, err := s.db.ReadFrom(context.Background(), offset, limit)
	return msgs, err
}

// Commit records offset as consumer's last-read position. Monotonic: a
// commit at or below the currently stored offset is a no-op.
func (s *PipelineService) Commit(consumer string, offset int64) error {
	return s.db.Commit(context.Background(), consumer, offset)
}
