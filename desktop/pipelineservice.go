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
func (s *PipelineService) InvokeAction(actionID string, item feed.Item, input pipeline.ActionInvocationInput) (pipeline.ActionRunView, error) {
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
	payload, err := json.Marshal(item)
	if err != nil {
		return pipeline.ActionRunView{}, fmt.Errorf("encoding action item: %w", err)
	}
	return s.worker.Confirm(context.Background(), actionID, item.ID, payload, input)
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
