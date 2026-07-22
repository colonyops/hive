package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/colonyops/hive/internal/desktop/pipeline"
	"github.com/colonyops/hive/internal/desktop/pipeline/actions"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

// PipelineService is the Wails service exposing the desktop pipeline's
// event log, configured actions, and commit protocol to the frontend.
type PipelineService struct {
	db            *pipelinedb.DB
	actions       *actions.ActionStore
	worker        *pipeline.Worker
	launchOptions pipeline.SessionLaunchOptionsProvider
}

func NewPipelineService(db *pipelinedb.DB, actionStore *actions.ActionStore, worker *pipeline.Worker, launchOptions pipeline.SessionLaunchOptionsProvider) *PipelineService {
	return &PipelineService{db: db, actions: actionStore, worker: worker, launchOptions: launchOptions}
}

// ReadFrom returns up to limit event_log rows after consumer's persisted
// offset, in ascending order. The frontend never supplies an offset: the
// SQLite checkpoint is the source of truth across runtime restarts.
func (s *PipelineService) ReadFrom(consumer string, limit int) ([]pipelinedb.Msg, error) {
	return s.db.ReadForConsumer(context.Background(), consumer, limit)
}

// Commit applies the frontend graph runtime's batch atomically. Feed outputs
// are accepted without durable effect until membership claims land; action
// outputs, node-run metrics, and the consumer offset are persisted atomically.
// Idempotent by offset: replaying a batch already applied (UpToOffset <= the
// consumer's current offset) is a no-op.
func (s *PipelineService) Commit(batch pipeline.CommitBatch) error {
	return s.db.CommitBatch(context.Background(), batch)
}

// EventLogTailOffset returns a Wails-safe decimal tail for the startup/deploy
// replay protocol.
func (s *PipelineService) EventLogTailOffset() (string, error) {
	tail, err := s.db.EventLogTailOffset(context.Background())
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(tail, 10), nil
}

// ActivateReplay atomically advances the consumer and installs the prepared
// membership state for a startup or deploy replay.
func (s *PipelineService) ActivateReplay(profileID, tail string, claims []pipelinedb.FeedMembershipClaim, feedIDs, sourceIDs []string) error {
	offset, err := strconv.ParseInt(tail, 10, 64)
	if err != nil || offset < 0 {
		return fmt.Errorf("invalid event log tail %q", tail)
	}
	return s.db.ActivateReplay(context.Background(), profileID, offset, claims, feedIDs, sourceIDs)
}

// ListUnarchivedInboxItems returns the JSON/Wails-friendly immutable inbox
// identity and payload needed for claims-only synthetic replay.
func (s *PipelineService) ListUnarchivedInboxItems(profileID string) ([]pipelinedb.InboxItemView, error) {
	return s.db.ListUnarchivedInboxItems(context.Background(), profileID)
}

// ListReplaySourceSnapshots returns each source's latest authoritative
// snapshot so membership replay preserves source provenance.
func (s *PipelineService) ListReplaySourceSnapshots(profileID, throughOffset string) ([]pipelinedb.Msg, error) {
	offset, err := strconv.ParseInt(throughOffset, 10, 64)
	if err != nil || offset < 0 {
		return nil, fmt.Errorf("invalid replay snapshot offset %q", throughOffset)
	}
	return s.db.ListReplaySourceSnapshots(context.Background(), profileID, offset)
}

// ENUM(inbox, open, archive, all, ignored)
type InboxView string

// ListInboxItems reads one deterministic inbox view for a profile.
func (s *PipelineService) ListInboxItems(profileID string, view InboxView, limit int) ([]pipelinedb.InboxItemView, error) {
	return s.db.ListInboxItems(context.Background(), profileID, view.String(), limit)
}

func (s *PipelineService) ListInboxItemsByFeed(profileID, feedID string, limit int) ([]pipelinedb.InboxItemView, error) {
	return s.db.ListInboxItemsByFeed(context.Background(), profileID, feedID, limit)
}

func (s *PipelineService) InboxItemEvents(itemID int64, limit int) ([]pipelinedb.InboxEventView, error) {
	return s.db.InboxItemEvents(context.Background(), itemID, limit)
}

func (s *PipelineService) MarkInboxItemUnread(itemID, revision int64, unread bool) (pipelinedb.InboxItemView, error) {
	return s.db.SetInboxItemUnread(context.Background(), itemID, revision, unread)
}

func (s *PipelineService) ToggleInboxItemArchived(itemID, revision int64) (pipelinedb.InboxItemView, error) {
	return s.db.ToggleInboxItemArchived(context.Background(), itemID, revision, time.Now().UnixMilli())
}

func (s *PipelineService) ToggleInboxItemIgnored(itemID, revision int64) (pipelinedb.InboxItemView, error) {
	return s.db.ToggleInboxItemIgnored(context.Background(), itemID, revision, time.Now().UnixMilli())
}

func (s *PipelineService) InboxCounts(profileID string) (pipelinedb.InboxCounts, error) {
	return s.db.InboxCounts(context.Background(), profileID)
}

func (s *PipelineService) FeedCounts(profileID string) ([]pipelinedb.FeedInboxCount, error) {
	return s.db.FeedCounts(context.Background(), profileID)
}

// ActionViews returns the configured actions available for an item kind
// ("PR"/"Issue"). The actions store is the single source for both these
// detail-pane views and flow output actions.
func (s *PipelineService) ActionViews(kind string) []actions.View {
	return s.actions.ViewsFor(kind)
}

// SessionLaunchOptions supplies the configured repository and agent choices
// for interactive launch-session actions. It intentionally exposes no local
// checkout paths or executable action configuration.
func (s *PipelineService) SessionLaunchOptions() (pipeline.SessionLaunchOptions, error) {
	if s.launchOptions == nil {
		return pipeline.SessionLaunchOptions{}, fmt.Errorf("session launch options are unavailable")
	}
	return s.launchOptions.SessionLaunchOptions(context.Background())
}

// InvokeAction records the user's explicit confirmation for actionID against
// item and executes it. It accepts only actions that apply to the item's kind;
// executable configuration is always re-resolved from ActionStore.
func (s *PipelineService) InvokeAction(actionID string, itemID int64, input pipeline.ActionInvocationInput) (pipeline.ActionRunView, error) {
	row, err := s.db.Queries().GetInboxItemByID(context.Background(), itemID)
	if err != nil {
		return pipeline.ActionRunView{}, fmt.Errorf("reading inbox item %d: %w", itemID, err)
	}
	if row.SourceKind != "github" {
		return pipeline.ActionRunView{}, fmt.Errorf("unsupported action source kind %q", row.SourceKind)
	}
	item, err := pipeline.DecodeGitHubActionItem(row.Payload, row.ExternalID)
	if err != nil {
		return pipeline.ActionRunView{}, fmt.Errorf("decode GitHub inbox item %d payload: %w", itemID, err)
	}
	action, ok := s.actions.Get(actionID)
	if !ok {
		return pipeline.ActionRunView{}, fmt.Errorf("unknown action %q", actionID)
	}
	if !action.ShowInDetail {
		return pipeline.ActionRunView{}, fmt.Errorf("action %q is not available in the detail pane", actionID)
	}
	if !actions.AppliesTo(action, item.Kind) {
		return pipeline.ActionRunView{}, fmt.Errorf("action %q does not apply to %q", actionID, item.Kind)
	}
	if item.ID == "" {
		return pipeline.ActionRunView{}, fmt.Errorf("action %q: item id is required", actionID)
	}
	if s.worker == nil {
		return pipeline.ActionRunView{}, fmt.Errorf("action execution is unavailable")
	}
	return s.worker.Confirm(context.Background(), actionID, item.ID, item.Payload, input)
}

// NodeRuns returns up to limit of a flow's most recent node_run rows,
// newest first, for the flows canvas's live per-node status and RECENT
// activity list.
func (s *PipelineService) NodeRuns(flowID string, limit int) ([]pipeline.NodeRunRecord, error) {
	return s.db.NodeRuns(context.Background(), flowID, limit)
}

func (s *PipelineService) ActionRun(commandID int64) (pipeline.ActionRunView, error) {
	row, err := s.db.OutputCommand(context.Background(), commandID)
	if err != nil {
		return pipeline.ActionRunView{}, err
	}
	view := pipeline.ActionRunView{CommandID: row.ID, Status: row.Status}
	if row.LastError.Valid {
		view.Error = row.LastError.String
	}
	if row.Stdout.Valid {
		view.Stdout = row.Stdout.String
	}
	if row.Stderr.Valid {
		view.Stderr = row.Stderr.String
	}
	if row.ResultJson.Valid {
		if err := json.Unmarshal([]byte(row.ResultJson.String), &view.Result); err != nil {
			return pipeline.ActionRunView{}, fmt.Errorf("decode action run %d result: %w", commandID, err)
		}
	}
	return view, nil
}
