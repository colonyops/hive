package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/pipeline"
	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

// PipelineService is the Wails service exposing the desktop pipeline's
// event log, configured actions, and commit protocol to the frontend.
type PipelineService struct {
	db      *pipelinedb.DB
	actions *actions.ActionStore
	worker  *pipeline.Worker
}

func NewPipelineService(db *pipelinedb.DB, actionStore *actions.ActionStore, worker *pipeline.Worker) *PipelineService {
	return &PipelineService{db: db, actions: actionStore, worker: worker}
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

// ActionViews returns the configured actions available for an item kind
// ("PR"/"Issue"). The actions store is the single source for both these
// detail-pane views and flow output actions.
func (s *PipelineService) ActionViews(kind string) []actions.View {
	return s.actions.ViewsFor(kind)
}

// InvokeAction records the user's explicit confirmation for actionID against
// item and executes it. It accepts only actions that apply to the item's kind;
// executable configuration is always re-resolved from ActionStore.
func (s *PipelineService) InvokeAction(actionID string, item feed.Item) error {
	action, ok := s.actions.Get(actionID)
	if !ok {
		return fmt.Errorf("unknown action %q", actionID)
	}
	if !actions.AppliesTo(action, item.Kind) {
		return fmt.Errorf("action %q does not apply to %q", actionID, item.Kind)
	}
	if item.ID == "" {
		return fmt.Errorf("action %q: item id is required", actionID)
	}
	if s.worker == nil {
		return fmt.Errorf("action execution is unavailable")
	}
	payload, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("encoding action item: %w", err)
	}
	return s.worker.Confirm(context.Background(), actionID, item.ID, payload)
}

// NodeRuns returns up to limit of a flow's most recent node_run rows,
// newest first, for the flows canvas's live per-node status and RECENT
// activity list.
func (s *PipelineService) NodeRuns(flowID string, limit int) ([]pipeline.NodeRunRecord, error) {
	return s.db.NodeRuns(context.Background(), flowID, limit)
}
