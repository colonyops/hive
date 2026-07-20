package main

import (
	"context"

	"github.com/colonyops/hive/internal/desktop/feed"
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

// ReadFrom returns up to limit event_log rows after consumer's persisted
// offset, in ascending order. The frontend never supplies an offset: the
// SQLite checkpoint is the source of truth across runtime restarts.
func (s *PipelineService) ReadFrom(consumer string, limit int) ([]pipelinedb.Msg, error) {
	return s.db.ReadForConsumer(context.Background(), consumer, limit)
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

// FeedItemCounts returns per-feed total/unread counts for every feed in a
// flow, for the sidebar's rail badges.
func (s *PipelineService) FeedItemCounts(flowID string) ([]pipeline.FeedCount, error) {
	return s.db.FeedItemCounts(context.Background(), flowID)
}

// ActionsFor returns the configured actions for an item kind ("PR"/"Issue"),
// for the detail-pane action picker. The catalog is static today (see
// feed.ActionsFor); this is the seam where actions.yml-driven actions plug in.
func (s *PipelineService) ActionsFor(kind string) []feed.Action {
	return feed.ActionsFor(kind)
}

// NodeRuns returns up to limit of a flow's most recent node_run rows,
// newest first, for the flows canvas's live per-node status and RECENT
// activity list.
func (s *PipelineService) NodeRuns(flowID string, limit int) ([]pipeline.NodeRunRecord, error) {
	return s.db.NodeRuns(context.Background(), flowID, limit)
}
