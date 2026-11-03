package mcpsrv

import (
	"cmp"
	"context"
	"encoding/json"
	"maps"
	"slices"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/inbox"
)

const (
	defaultListLimit  = 200
	defaultEventLimit = 50
)

// feedView is one feed and what has landed in it. Name comes from the graph and
// the counts from inbox membership, so a feed can appear with either half
// missing — see ListFeeds.
type feedView struct {
	ID string `json:"id"`
	// Declared is false for a feed that only inbox rows still claim: its node
	// was removed from the graph but its items were never reassigned.
	Declared bool   `json:"declared"`
	Name     string `json:"name,omitempty"`
	Total    int64  `json:"total"`
	Unread   int64  `json:"unread"`
	Archived int64  `json:"archived"`
}

type feedsResult struct {
	Feeds []feedView `json:"feeds"`
}

// ListFeeds lists a profile's feeds: the ones its graph declares, joined onto
// inbox counts, plus any feed only the counts know about.
//
// The counts alone omit a declared feed nothing has landed in yet, which makes
// an empty feed and a misspelled one the same answer. The graph alone would
// hide a feed whose node was deleted out from under its items. Neither half is
// authoritative on its own, so this reports the union and says which is which.
func (ctrl *Controller) ListFeeds(ctx context.Context, in inbox.FeedListIn) (feedsResult, error) {
	if in.ProfileID == "" {
		return feedsResult{}, app.Errorf(app.KindInvalid, "profileId is required")
	}
	counts, err := ctrl.core.Inbox.FeedCounts(ctx, in.ProfileID)
	if err != nil {
		return feedsResult{}, err
	}
	byID := make(map[string]stores.FeedCount, len(counts))
	for _, c := range counts {
		byID[c.FeedID] = c
	}

	f, err := ctrl.core.Flows.Get(ctx, in.ProfileID)
	if err != nil && (len(counts) == 0 || app.KindOf(err) != app.KindNotFound) {
		return feedsResult{}, err
	}

	feeds := make([]feedView, 0, len(f.Nodes)+len(counts))
	for _, node := range f.FeedNodes() {
		id := f.FeedID(node.ID)
		count := byID[id]
		delete(byID, id)
		feeds = append(feeds, feedView{
			ID: id, Declared: true, Name: cmp.Or(node.Name, node.ID),
			Total: count.Total, Unread: count.Unread, Archived: count.Archived,
		})
	}
	undeclared := slices.Sorted(maps.Keys(byID))
	for _, id := range undeclared {
		count := byID[id]
		feeds = append(feeds, feedView{
			ID: id, Total: count.Total, Unread: count.Unread, Archived: count.Archived,
		})
	}
	return feedsResult{Feeds: feeds}, nil
}

// inboxItemView is the store view plus the feed that claims the item, so a flat
// listing can answer "which feed is this in?" — a stores.InboxItem carries no
// feed id of its own.
//
// Payload shadows the embedded field so it can be omitted rather than nulled;
// it is the one field here whose size the source decides, and a listing repeats
// it per item. Everything else is this app's own vocabulary and is bounded.
type inboxItemView struct {
	stores.InboxItem
	FeedID  string           `json:"feedId"`
	Payload *json.RawMessage `json:"payload,omitempty"`
}

type itemsResult struct {
	Items  []inboxItemView `json:"items"`
	Detail string          `json:"detail"`
}

// The Out type is `any` here and on the other tools answering with store views.
// The SDK infers an output schema from Out, and its inferrer rejects a
// jsonschema struct tag beginning with "WORD=" — which is exactly the shape the
// store views carry for the OpenAPI reflector they were written for. Omitting
// the output schema costs nothing: it is advisory, the content is still the
// same JSON, and the input schemas — the ones a model actually calls through —
// are inferred in full from the input types in internal/ops.
func (ctrl *Controller) ListInbox(ctx context.Context, in inbox.InboxListIn) (any, error) {
	if in.Feed != "" && in.ProfileID == "" {
		return nil, app.Errorf(app.KindInvalid, "profileId is required when feed is set")
	}
	if in.Limit < 0 {
		return nil, app.Errorf(app.KindInvalid, "limit must be positive")
	}
	detail := cmp.Or(in.Detail, ops.DetailSummary)
	limit := cmp.Or(in.Limit, defaultListLimit)

	var (
		items []stores.InboxItem
		err   error
	)
	switch {
	case in.ExternalID != "":
		items, err = ctrl.core.Inbox.FindItems(ctx, in.ProfileID, in.ExternalID)
	case in.Feed != "" && in.Archived:
		items, err = ctrl.core.Inbox.ListArchivedByFeed(ctx, in.ProfileID, in.Feed, limit)
	case in.Feed != "":
		items, err = ctrl.core.Inbox.ListByFeed(ctx, in.ProfileID, in.Feed, limit)
	default:
		items, err = ctrl.core.Inbox.ListItems(ctx, in.ProfileID, limit)
	}
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		if err := ctrl.explainEmptyScope(ctx, in.ProfileID, in.Feed); err != nil {
			return nil, err
		}
	}

	views, err := ctrl.itemsWithFeed(ctx, items, detail)
	if err != nil {
		return nil, err
	}
	return itemsResult{Items: views, Detail: detail}, nil
}

// explainEmptyScope turns an empty listing into not_found when the profile or
// feed it named does not exist, so "nothing has landed here" and "you named the
// wrong thing" stop being the same answer.
//
// It runs only after the query came back empty, never before it. A profile
// whose flow file was deleted while its inbox rows survived is a state worth
// being able to read, and gating the query on the graph would make exactly that
// state invisible. An empty profile is the deliberate unscoped read.
func (ctrl *Controller) explainEmptyScope(ctx context.Context, profile, feed string) error {
	if profile == "" {
		return nil
	}
	f, err := ctrl.core.Flows.Get(ctx, profile)
	if err != nil || feed == "" {
		return err
	}
	for _, node := range f.FeedNodes() {
		if f.FeedID(node.ID) == feed {
			return nil
		}
	}
	return app.Errorf(app.KindNotFound, "profile %q declares no feed %q", profile, feed)
}

// itemsWithFeed annotates each item with the feed that claims it (empty when
// unrouted), resolved in one query so the listing avoids an N+1.
func (ctrl *Controller) itemsWithFeed(ctx context.Context, items []stores.InboxItem, detail string) ([]inboxItemView, error) {
	ids := make([]int64, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	feeds, err := ctrl.core.Inbox.Feeds(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]inboxItemView, len(items))
	for i, it := range items {
		out[i] = inboxItemView{InboxItem: it, FeedID: feeds[it.ID]}
		if detail == ops.DetailFull {
			out[i].Payload = &items[i].Payload
		}
	}
	return out, nil
}

// eventView drops each event's raw detail at summary, for the reason
// inboxItemView drops a payload: it is source-supplied and repeats per event.
type eventView struct {
	stores.InboxEvent
	Detail *json.RawMessage `json:"detail,omitempty"`
}

type eventsResult struct {
	Events []eventView `json:"events"`
	Detail string      `json:"detail"`
}

func (ctrl *Controller) ListInboxItemEvents(ctx context.Context, in inbox.InboxEventListIn) (any, error) {
	if in.Limit < 0 {
		return nil, app.Errorf(app.KindInvalid, "limit must be positive")
	}
	detail := cmp.Or(in.Detail, ops.DetailSummary)
	itemID, err := ctrl.resolveItemID(ctx, in.ItemID, in.ExternalID, in.ProfileID)
	if err != nil {
		return nil, err
	}
	events, err := ctrl.core.Inbox.Events(ctx, itemID, cmp.Or(in.Limit, defaultEventLimit))
	if err != nil {
		return nil, err
	}
	views := make([]eventView, len(events))
	for i, event := range events {
		views[i] = eventView{InboxEvent: event}
		if detail == ops.DetailFull {
			views[i].Detail = &events[i].Detail
		}
	}
	return eventsResult{Events: views, Detail: detail}, nil
}

// resolveItemID accepts an explicit itemId, or an externalId that must resolve
// to exactly one item. Every tool addressing a single item goes through it, so
// they all accept the same two ways of naming one.
func (ctrl *Controller) resolveItemID(ctx context.Context, itemID int64, externalID, profile string) (int64, error) {
	if itemID != 0 {
		return itemID, nil
	}
	items, err := ctrl.core.Inbox.FindItems(ctx, profile, externalID)
	if err != nil {
		return 0, err
	}
	switch len(items) {
	case 0:
		return 0, app.Errorf(app.KindNotFound, "no item with external id %q", externalID)
	case 1:
		return items[0].ID, nil
	default:
		return 0, app.Errorf(app.KindConflict, "external id %q matches %d items; add profileId to disambiguate", externalID, len(items))
	}
}

type itemSessionsResult struct {
	Sessions []app.ItemSessionView `json:"sessions"`
}

// ListItemSessions reports the hive sessions one inbox item started.
//
// The read reconciles as a side effect: a link a *successful* hive listing
// cannot account for is dropped, while a failed listing drops nothing — absent
// evidence is never evidence of absence.
func (ctrl *Controller) ListItemSessions(ctx context.Context, in inbox.InboxSessionListIn) (any, error) {
	itemID, err := ctrl.resolveItemID(ctx, in.ItemID, in.ExternalID, in.ProfileID)
	if err != nil {
		return nil, err
	}
	sessions, err := ctrl.core.Sessions.ItemSessions(ctx, itemID)
	if err != nil {
		return nil, err
	}
	return itemSessionsResult{Sessions: sessions}, nil
}
