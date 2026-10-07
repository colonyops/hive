package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
)

// InboxService owns the durable inbox as a *reader and triager* sees it: the
// item views the sidebar renders and the explicit action invocations a user
// confirms from the detail pane.
//
// The event log itself is deliberately not here. Reading it, committing runs
// against it, and the replay protocol are the flow engine's, and the engine
// talks to the store directly — routing them through a service would only put
// a facade between two parts of the core with no caller in between.
type InboxService struct {
	items    *stores.InboxItemStore
	commands *stores.OutputCommandStore
	nodeRuns *stores.NodeRunStore
	actions  *actions.ActionStore
	worker   *dispatch.Worker
}

type InboxDeps struct {
	Items    *stores.InboxItemStore
	Commands *stores.OutputCommandStore
	NodeRuns *stores.NodeRunStore
	Catalog  *actions.ActionStore
	Worker   *dispatch.Worker
}

func newInboxService(d InboxDeps) *InboxService {
	return &InboxService{items: d.Items, commands: d.Commands, nodeRuns: d.NodeRuns, actions: d.Catalog, worker: d.Worker}
}

func (s *InboxService) ListByFeed(ctx context.Context, profileID, feedID string, limit int) ([]stores.InboxItem, error) {
	items, err := s.items.ListByFeed(ctx, profileID, feedID, limit)
	return items, Wrap(err, KindInternal, "listing feed %q", feedID)
}

func (s *InboxService) ListArchivedByFeed(ctx context.Context, profileID, feedID string, limit int) ([]stores.InboxItem, error) {
	items, err := s.items.ListArchivedByFeed(ctx, profileID, feedID, limit)
	return items, Wrap(err, KindInternal, "listing archived items in feed %q", feedID)
}

// ListTrash returns unrouted and ignored items for the Trash view.
func (s *InboxService) ListTrash(ctx context.Context, profileID string, limit int) ([]stores.InboxItem, error) {
	items, err := s.items.ListTrash(ctx, profileID, limit)
	return items, Wrap(err, KindInternal, "listing trash for %q", profileID)
}

// Feed returns the claiming feed, or empty for an unrouted item.
func (s *InboxService) Feed(ctx context.Context, profileID string, itemID int64) (string, error) {
	feedID, err := s.items.FeedIDForItem(ctx, profileID, itemID)
	return feedID, Wrap(err, KindInternal, "resolving the feed for item %d", itemID)
}

func (s *InboxService) Feeds(ctx context.Context, itemIDs []int64) (map[int64]string, error) {
	feeds, err := s.items.FeedIDsForItems(ctx, itemIDs)
	return feeds, Wrap(err, KindInternal, "resolving feeds for %d items", len(itemIDs))
}

// Events returns KindNotFound when the item does not exist; an existing item
// can have an empty event list.
func (s *InboxService) Events(ctx context.Context, itemID int64, limit int) ([]stores.InboxEvent, error) {
	if err := s.requireItem(ctx, itemID); err != nil {
		return nil, err
	}
	events, err := s.items.Events(ctx, itemID, limit)
	return events, Wrap(err, KindInternal, "listing events for item %d", itemID)
}

// requireItem reports KindNotFound for an item id no row backs.
func (s *InboxService) requireItem(ctx context.Context, itemID int64) error {
	_, err := s.getItem(ctx, itemID)
	return err
}

func (s *InboxService) getItem(ctx context.Context, itemID int64) (stores.InboxItem, error) {
	item, err := s.items.GetByID(ctx, itemID)
	if stores.IsNotFound(err) {
		return stores.InboxItem{}, Wrap(err, KindNotFound, "inbox item %d not found", itemID)
	}
	return item, Wrap(err, KindInternal, "reading inbox item %d", itemID)
}

func (s *InboxService) SetUnread(ctx context.Context, itemID, revision int64, unread bool) (stores.InboxItem, error) {
	item, err := s.items.SetUnread(ctx, itemID, revision, unread)
	return item, s.itemWriteError(err, itemID)
}

// MarkRead clears unread in one feed, or the whole profile when feedID is
// empty. Archived and ignored items retain their state.
func (s *InboxService) MarkRead(ctx context.Context, profileID, feedID string) (int64, error) {
	n, err := s.items.MarkRead(ctx, profileID, feedID)
	return n, Wrap(err, KindInternal, "marking items read in %q", profileID)
}

func (s *InboxService) ToggleArchived(ctx context.Context, itemID, revision int64) (stores.InboxItem, error) {
	item, err := s.items.ToggleArchived(ctx, itemID, revision)
	return item, s.itemWriteError(err, itemID)
}

func (s *InboxService) ToggleIgnored(ctx context.Context, itemID, revision int64) (stores.InboxItem, error) {
	item, err := s.items.ToggleIgnored(ctx, itemID, revision)
	return item, s.itemWriteError(err, itemID)
}

// itemWriteError classifies a revision-guarded write. A stale revision is the
// caller's view having moved, which is the whole reason KindConflict exists:
// re-read and retry rather than report a failure.
func (s *InboxService) itemWriteError(err error, itemID int64) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, stores.ErrStale):
		return Wrap(err, KindConflict, "inbox item %d changed underneath you", itemID)
	default:
		return Wrap(err, KindInternal, "updating inbox item %d", itemID)
	}
}

func (s *InboxService) FeedCounts(ctx context.Context, profileID string) ([]stores.FeedCount, error) {
	counts, err := s.items.FeedCounts(ctx, profileID)
	return counts, Wrap(err, KindInternal, "counting feeds for %q", profileID)
}

// ListItems returns inbox items newest-first, optionally scoped to a profile
// (empty matches all), unfiltered by feed or triage — a debug/observation read.
func (s *InboxService) ListItems(ctx context.Context, profileID string, limit int) ([]stores.InboxItem, error) {
	items, err := s.items.ListAll(ctx, profileID, limit)
	return items, Wrap(err, KindInternal, "listing inbox items")
}

// FindItems returns every inbox item sharing an external id, optionally scoped
// to a profile (empty profileID matches all). The same external id can exist
// across profiles and source scopes, hence a slice.
func (s *InboxService) FindItems(ctx context.Context, profileID, externalID string) ([]stores.InboxItem, error) {
	if externalID == "" {
		return nil, Errorf(KindInvalid, "external id is required")
	}
	items, err := s.items.FindByExternalID(ctx, profileID, externalID)
	return items, Wrap(err, KindInternal, "finding items for external id %q", externalID)
}

// ActionViews returns the configured actions applicable to an inbox item:
// shown-in-detail, applies_to matches the item's canonical kind, and every
// hard template capability is satisfiable for the item's payload.
func (s *InboxService) ActionViews(ctx context.Context, itemID int64) ([]actions.View, error) {
	item, err := s.decodeItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	views := make([]actions.View, 0)
	for _, action := range s.actions.List() {
		if !action.ShowInDetail {
			continue
		}
		if ok, _ := dispatch.ActionApplicability(action, item); !ok {
			continue
		}
		views = append(views, action.View())
	}
	return views, nil
}

// NewSessionDraft projects ordered inbox items into one prefilled New Session form.
func (s *InboxService) NewSessionDraft(ctx context.Context, itemIDs []int64) (dispatch.SessionDraft, error) {
	if len(itemIDs) == 0 {
		return dispatch.SessionDraft{}, Errorf(KindInvalid, "at least one inbox item is required")
	}
	seen := make(map[int64]struct{}, len(itemIDs))
	items := make([]dispatch.SessionDraftItem, 0, len(itemIDs))
	orderedIDs := make([]int64, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		if itemID <= 0 {
			return dispatch.SessionDraft{}, Errorf(KindInvalid, "inbox item id must be positive")
		}
		if _, exists := seen[itemID]; exists {
			continue
		}
		seen[itemID] = struct{}{}
		item, err := s.getItem(ctx, itemID)
		if err != nil {
			return dispatch.SessionDraft{}, err
		}
		orderedIDs = append(orderedIDs, itemID)
		items = append(items, dispatch.SessionDraftItem{Title: item.Title, URL: item.URL, Payload: item.Payload})
	}
	draft, err := dispatch.RenderSessionDraftItems(items)
	if err != nil {
		return dispatch.SessionDraft{}, Wrap(err, KindInternal, "rendering session draft for %d items", len(items))
	}
	draft.ItemIDs = orderedIDs
	return draft, nil
}

// InvokeActionRequest is one explicit, user-confirmed action invocation.
type InvokeActionRequest struct {
	ActionID string
	ItemID   int64
	Input    dispatch.ActionInvocationInput
}

// InvokeAction records the user's explicit confirmation and executes.
//
// It carries the whole authorization chain: the item must decode, the action
// must exist in the catalog, be shown in the detail pane, apply to the item's
// kind, and the item must have an id. Executable configuration is always
// re-resolved from the catalog rather than taken from the caller.
func (s *InboxService) InvokeAction(ctx context.Context, req InvokeActionRequest) (dispatch.ActionRunView, error) {
	item, err := s.decodeItem(ctx, req.ItemID)
	if err != nil {
		return dispatch.ActionRunView{}, err
	}
	action, ok := s.actions.Get(req.ActionID)
	if !ok {
		return dispatch.ActionRunView{}, Errorf(KindNotFound, "unknown action %q", req.ActionID)
	}
	if !action.ShowInDetail {
		return dispatch.ActionRunView{}, Errorf(KindInvalid, "action %q is not available in the detail pane", req.ActionID)
	}
	if _, isClipboard := action.Config.(*actions.ClipboardConfig); isClipboard {
		// A clipboard action produces text, not a durable command. It must not
		// enqueue an output_command (RenderClipboardAction is its path), so the
		// worker never sees it and re-copying never prompts for a rerun.
		return dispatch.ActionRunView{}, Errorf(KindInvalid, "action %q is a clipboard action; copy it from the detail pane instead", req.ActionID)
	}
	if applicable, reason := dispatch.ActionApplicability(action, item); !applicable {
		return dispatch.ActionRunView{}, Errorf(KindInvalid, "action %q does not apply to item %d: %s", req.ActionID, req.ItemID, reason)
	}
	if item.ID == "" {
		return dispatch.ActionRunView{}, Errorf(KindInvalid, "action %q: item id is required", req.ActionID)
	}
	// Preflight the collected inputs against the catalog's declaration so an
	// incomplete form is refused outright, rather than becoming a durable
	// command the worker then fails.
	if _, err := action.ResolveInputs(req.Input.Inputs); err != nil {
		return dispatch.ActionRunView{}, Wrap(err, KindInvalid, "invoking action %q", req.ActionID)
	}
	// The command's key is the action item's id, which is not enough to find
	// the row again; the item's own identity rides along so a session this
	// invocation creates stays reachable from the item that asked for it.
	origin, err := s.items.RefByID(ctx, req.ItemID)
	if err != nil {
		return dispatch.ActionRunView{}, Wrap(err, KindInternal, "reading inbox item %d", req.ItemID)
	}
	view, err := s.worker.Confirm(ctx, req.ActionID, item.ID, item.Payload, origin, req.Input)
	if err != nil {
		return dispatch.ActionRunView{}, s.confirmError(err, req.ActionID)
	}
	return view, nil
}

// RenderClipboardAction renders one clipboard action once per ordered item and
// joins the entries into the single block the desktop writes to the clipboard.
// It never enqueues an output command, so copying remains repeatable.
func (s *InboxService) RenderClipboardAction(ctx context.Context, actionID string, itemIDs []int64, inputs map[string]string) (string, error) {
	action, ok := s.actions.Get(actionID)
	if !ok {
		return "", Errorf(KindNotFound, "unknown action %q", actionID)
	}
	if _, isClipboard := action.Config.(*actions.ClipboardConfig); !isClipboard {
		return "", Errorf(KindInvalid, "action %q is not a clipboard action", actionID)
	}
	if !action.ShowInDetail {
		return "", Errorf(KindInvalid, "action %q is not available in the detail pane", actionID)
	}
	if len(itemIDs) == 0 {
		return "", Errorf(KindInvalid, "at least one inbox item is required")
	}
	resolved, err := action.ResolveInputs(inputs)
	if err != nil {
		return "", Wrap(err, KindInvalid, "rendering clipboard action %q", actionID)
	}
	entries := make([]string, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		item, err := s.decodeItem(ctx, itemID)
		if err != nil {
			return "", err
		}
		if applicable, reason := dispatch.ActionApplicability(action, item); !applicable {
			return "", Errorf(KindInvalid, "action %q does not apply to item %d: %s", actionID, itemID, reason)
		}
		if item.ID == "" {
			return "", Errorf(KindInvalid, "action %q: item id is required", actionID)
		}
		text, err := dispatch.RenderClipboardText(action, item.ID, item.Payload, resolved)
		if err != nil {
			return "", Wrap(err, KindInvalid, "rendering clipboard action %q for item %d", actionID, itemID)
		}
		entries = append(entries, text)
	}
	return strings.Join(entries, "\n\n"), nil
}

// confirmError classifies a refused confirmation. A rerun with no completed
// prior run is the caller asking for something that cannot exist yet, not a
// failure of ours — which is why the store had to start wrapping sql.ErrNoRows.
func (s *InboxService) confirmError(err error, actionID string) error {
	if errors.Is(err, dispatch.ErrDispatchBusy) {
		return Wrap(err, KindConflict, "action %q cannot start because the manual action limit is reached", actionID)
	}
	if errors.Is(err, stores.ErrOutputCommandActive) {
		return Wrap(err, KindConflict, "action %q is already running", actionID)
	}
	if stores.IsNotFound(err) {
		return Wrap(err, KindInvalid, "action %q has no completed run to repeat", actionID)
	}
	return Wrap(err, KindInternal, "invoking action %q", actionID)
}

// NodeRuns returns up to limit of a flow's most recent node_run rows, newest
// first, for the canvas's live per-node status and recent activity list.
func (s *InboxService) NodeRuns(ctx context.Context, flowID string, limit int) ([]stores.NodeRunRecord, error) {
	runs, err := s.nodeRuns.List(ctx, flowID, limit)
	return runs, Wrap(err, KindInternal, "listing node runs for flow %q", flowID)
}

func (s *InboxService) ActionRun(ctx context.Context, commandID int64) (dispatch.ActionRunView, error) {
	row, err := s.commands.Get(ctx, commandID)
	if err != nil {
		if stores.IsNotFound(err) {
			return dispatch.ActionRunView{}, Wrap(err, KindNotFound, "action run %d not found", commandID)
		}
		return dispatch.ActionRunView{}, Wrap(err, KindInternal, "reading action run %d", commandID)
	}
	view := dispatch.ActionRunView{CommandID: row.ID, Status: row.Status, Error: row.LastError, Stdout: row.Stdout, Stderr: row.Stderr}
	if row.ResultJSON != "" {
		if err := json.Unmarshal([]byte(row.ResultJSON), &view.Result); err != nil {
			return dispatch.ActionRunView{}, Wrap(err, KindInternal, "decoding action run %d result", commandID)
		}
	}
	return view, nil
}

// decodeItem reads and decodes one inbox item, classifying the two ways it
// can fail: the row is gone, or its payload is not a canonical item.
func (s *InboxService) decodeItem(ctx context.Context, itemID int64) (dispatch.DecodedActionItem, error) {
	row, err := s.getItem(ctx, itemID)
	if err != nil {
		return dispatch.DecodedActionItem{}, err
	}
	item, err := dispatch.DecodeActionItem(row.Payload, row.ExternalID)
	if err != nil {
		return dispatch.DecodedActionItem{}, Wrap(err, KindInvalid, "decoding inbox item %d payload", itemID)
	}
	return item, nil
}
