package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
)

// PipelineService exposes the inbox to the frontend. Every method is a
// request build plus one call into the core.
//
// The event log is not on this surface. Reading it, committing runs against
// it, and the replay protocol used to be RPCs because the graph ran in the
// browser; with the engine in Go they are internal calls, and the
// decimal-string offset encoding they needed to survive the JavaScript number
// boundary went with them.
type PipelineService struct {
	inbox *app.InboxService
}

func NewPipelineService(inbox *app.InboxService) *PipelineService {
	return &PipelineService{inbox: inbox}
}

func (s *PipelineService) ListByFeed(ctx context.Context, profileID, feedID string, limit int) ([]stores.InboxItem, error) {
	return s.inbox.ListByFeed(ctx, profileID, feedID, limit)
}

func (s *PipelineService) ListArchivedByFeed(ctx context.Context, profileID, feedID string, limit int) ([]stores.InboxItem, error) {
	return s.inbox.ListArchivedByFeed(ctx, profileID, feedID, limit)
}

func (s *PipelineService) ListTrash(ctx context.Context, profileID string, limit int) ([]stores.InboxItem, error) {
	return s.inbox.ListTrash(ctx, profileID, limit)
}

func (s *PipelineService) FindItems(ctx context.Context, profileID, externalID string) ([]stores.InboxItem, error) {
	return s.inbox.FindItems(ctx, profileID, externalID)
}

func (s *PipelineService) Feed(ctx context.Context, profileID string, itemID int64) (string, error) {
	return s.inbox.Feed(ctx, profileID, itemID)
}

func (s *PipelineService) Events(ctx context.Context, itemID int64, limit int) ([]stores.InboxEvent, error) {
	return s.inbox.Events(ctx, itemID, limit)
}

func (s *PipelineService) SetUnread(ctx context.Context, itemID, revision int64, unread bool) (stores.InboxItem, error) {
	return s.inbox.SetUnread(ctx, itemID, revision, unread)
}

func (s *PipelineService) MarkRead(ctx context.Context, profileID, feedID string) (int64, error) {
	return s.inbox.MarkRead(ctx, profileID, feedID)
}

func (s *PipelineService) ToggleArchived(ctx context.Context, itemID, revision int64) (stores.InboxItem, error) {
	return s.inbox.ToggleArchived(ctx, itemID, revision)
}

func (s *PipelineService) ToggleIgnored(ctx context.Context, itemID, revision int64) (stores.InboxItem, error) {
	return s.inbox.ToggleIgnored(ctx, itemID, revision)
}

func (s *PipelineService) FeedCounts(ctx context.Context, profileID string) ([]stores.FeedCount, error) {
	return s.inbox.FeedCounts(ctx, profileID)
}

func (s *PipelineService) ActionViews(ctx context.Context, itemID int64) ([]actions.View, error) {
	return s.inbox.ActionViews(ctx, itemID)
}

func (s *PipelineService) NewSessionDraft(ctx context.Context, itemIDs []int64) (SessionDraft, error) {
	draft, err := s.inbox.NewSessionDraft(ctx, itemIDs)
	if err != nil {
		return SessionDraft{}, err
	}
	return sessionDraftOf(draft), nil
}

func (s *PipelineService) InvokeAction(ctx context.Context, actionID string, itemID int64, input ActionInvocationInput) (ActionRunView, error) {
	run, err := s.inbox.InvokeAction(ctx, app.InvokeActionRequest{
		ActionID: actionID,
		ItemID:   itemID,
		Input:    input.core(),
	})
	if err != nil {
		return ActionRunView{}, err
	}
	return actionRunViewOf(run), nil
}

// RenderClipboardAction returns one text block rendered across ordered items.
// The frontend writes it through the native Wails clipboard.
func (s *PipelineService) RenderClipboardAction(ctx context.Context, actionID string, itemIDs []int64, inputs map[string]string) (string, error) {
	return s.inbox.RenderClipboardAction(ctx, actionID, itemIDs, inputs)
}

func (s *PipelineService) NodeRuns(ctx context.Context, flowID string, limit int) ([]stores.NodeRunRecord, error) {
	return s.inbox.NodeRuns(ctx, flowID, limit)
}

func (s *PipelineService) ActionRun(ctx context.Context, commandID int64) (ActionRunView, error) {
	run, err := s.inbox.ActionRun(ctx, commandID)
	if err != nil {
		return ActionRunView{}, err
	}
	return actionRunViewOf(run), nil
}
