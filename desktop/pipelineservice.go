package main

import (
	"context"

	"github.com/colonyops/hive/internal/desktop/pipeline"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

// PipelineService is the Wails service exposing the desktop pipeline's
// event log and commit protocol to the frontend. All data comes from the
// injected *pipelinedb.DB; this type is wire glue only, matching
// FeedService's thin-glue style.
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

// Commit applies the frontend graph runtime's batch atomically: it upserts
// feed outputs, enqueues action outputs, records node-run metrics, and
// advances the consumer's offset to batch.UpToOffset. Idempotent by offset:
// replaying a batch already applied (UpToOffset <= the consumer's current
// offset) is a no-op.
func (s *PipelineService) Commit(batch pipeline.CommitBatch) error {
	return s.db.CommitBatch(context.Background(), batch)
}

// FeedItems returns the persisted items for a feed, newest first.
func (s *PipelineService) FeedItems(feedID string) ([]pipeline.FeedItem, error) {
	return s.db.FeedItems(context.Background(), feedID)
}

// MarkFeedItemRead clears the unread flag on one feed item.
func (s *PipelineService) MarkFeedItemRead(feedID, itemID string) error {
	return s.db.MarkFeedItemRead(context.Background(), feedID, itemID)
}

// NodeRuns returns up to limit of a flow's most recent node_run rows,
// newest first, for the flows canvas's live per-node status and RECENT
// activity list.
func (s *PipelineService) NodeRuns(flowID string, limit int) ([]pipeline.NodeRunRecord, error) {
	return s.db.NodeRuns(context.Background(), flowID, limit)
}
