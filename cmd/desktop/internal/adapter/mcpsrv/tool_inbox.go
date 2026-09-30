package mcpsrv

import (
	"cmp"
	"context"
	"encoding/json"
	"maps"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
)

const (
	defaultListLimit  = 200
	defaultEventLimit = 50
)

type listFeedsInput struct {
	Profile string `json:"profile" jsonschema:"Profile id to list feeds for; ids come from list_profiles."`
}

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
func (ctrl *Controller) ListFeeds(ctx context.Context, _ *mcp.CallToolRequest, in listFeedsInput) (*mcp.CallToolResult, feedsResult, error) {
	if in.Profile == "" {
		return nil, feedsResult{}, ctrl.toolError(app.Errorf(app.KindInvalid, "profile is required"))
	}
	counts, err := ctrl.core.Inbox.FeedCounts(ctx, in.Profile)
	if err != nil {
		return nil, feedsResult{}, ctrl.toolError(err)
	}
	byID := make(map[string]stores.FeedCount, len(counts))
	for _, c := range counts {
		byID[c.FeedID] = c
	}

	f, err := ctrl.core.Flows.Get(ctx, in.Profile)
	if err != nil && (len(counts) == 0 || app.KindOf(err) != app.KindNotFound) {
		return nil, feedsResult{}, ctrl.toolError(err)
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
	return nil, feedsResult{Feeds: feeds}, nil
}

type listInboxInput struct {
	Profile    string `json:"profile,omitempty"    jsonschema:"Profile id to scope to; required when feed is set."`
	ExternalID string `json:"externalId,omitempty" jsonschema:"A source's own id (e.g. a GitHub node id); returns every matching item across profiles."`
	Feed       string `json:"feed,omitempty"       jsonschema:"Feed id from list_feeds (e.g. hive/desktop-prs) to return only that feed's items."`
	Archived   bool   `json:"archived,omitempty"   jsonschema:"When true, list archived items instead of active ones."`
	Limit      int    `json:"limit,omitempty"      jsonschema:"Maximum items to return; defaults to 200."`
	Detail     string `json:"detail,omitempty"     jsonschema:"summary (the default) omits each item's raw source payload; full includes it, and its size is the source's to decide."`
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
// are inferred in full from the adapter-local input structs above.
func (ctrl *Controller) ListInbox(ctx context.Context, _ *mcp.CallToolRequest, in listInboxInput) (*mcp.CallToolResult, any, error) {
	if in.Feed != "" && in.Profile == "" {
		return nil, nil, ctrl.toolError(app.Errorf(app.KindInvalid, "profile is required when feed is set"))
	}
	if in.Limit < 0 {
		return nil, nil, ctrl.toolError(app.Errorf(app.KindInvalid, "limit must be positive"))
	}
	detail, err := readDetail(in.Detail)
	if err != nil {
		return nil, nil, ctrl.toolError(err)
	}
	limit := cmp.Or(in.Limit, defaultListLimit)

	var items []stores.InboxItem
	switch {
	case in.ExternalID != "":
		items, err = ctrl.core.Inbox.FindItems(ctx, in.Profile, in.ExternalID)
	case in.Feed != "" && in.Archived:
		items, err = ctrl.core.Inbox.ListArchivedByFeed(ctx, in.Profile, in.Feed, limit)
	case in.Feed != "":
		items, err = ctrl.core.Inbox.ListByFeed(ctx, in.Profile, in.Feed, limit)
	default:
		items, err = ctrl.core.Inbox.ListItems(ctx, in.Profile, limit)
	}
	if err != nil {
		return nil, nil, ctrl.toolError(err)
	}
	if len(items) == 0 {
		if err := ctrl.explainEmptyScope(ctx, in.Profile, in.Feed); err != nil {
			return nil, nil, ctrl.toolError(err)
		}
	}

	views, err := ctrl.itemsWithFeed(ctx, items, detail)
	if err != nil {
		return nil, nil, ctrl.toolError(err)
	}
	return nil, itemsResult{Items: views, Detail: detail}, nil
}

// readDetail resolves a read tool's detail argument. These tools have no middle
// rung: there is one source-supplied field on each and it is either there or
// not.
func readDetail(detail string) (string, error) {
	switch detail {
	case "":
		return detailSummary, nil
	case detailSummary, detailFull:
		return detail, nil
	default:
		return "", app.Errorf(app.KindInvalid, "detail must be %q or %q", detailSummary, detailFull)
	}
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
		if detail == detailFull {
			out[i].Payload = &items[i].Payload
		}
	}
	return out, nil
}

type listEventsInput struct {
	ItemID     int64  `json:"itemId,omitempty"     jsonschema:"Inbox item id. Provide this or externalId."`
	ExternalID string `json:"externalId,omitempty" jsonschema:"External id resolving to one item; provide this or itemId."`
	Profile    string `json:"profile,omitempty"    jsonschema:"Profile id used to disambiguate an externalId that matches items in more than one profile."`
	Limit      int    `json:"limit,omitempty"      jsonschema:"Maximum events to return; defaults to 50."`
	Detail     string `json:"detail,omitempty"     jsonschema:"summary (the default) omits each event's raw source-specific detail; full includes it."`
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

func (ctrl *Controller) ListInboxItemEvents(ctx context.Context, _ *mcp.CallToolRequest, in listEventsInput) (*mcp.CallToolResult, any, error) {
	if in.ItemID == 0 && in.ExternalID == "" {
		return nil, nil, ctrl.toolError(app.Errorf(app.KindInvalid, "provide itemId or externalId"))
	}
	if in.Limit < 0 {
		return nil, nil, ctrl.toolError(app.Errorf(app.KindInvalid, "limit must be positive"))
	}
	detail, err := readDetail(in.Detail)
	if err != nil {
		return nil, nil, ctrl.toolError(err)
	}
	itemID, err := ctrl.resolveItemID(ctx, in.ItemID, in.ExternalID, in.Profile)
	if err != nil {
		return nil, nil, ctrl.toolError(err)
	}
	events, err := ctrl.core.Inbox.Events(ctx, itemID, cmp.Or(in.Limit, defaultEventLimit))
	if err != nil {
		return nil, nil, ctrl.toolError(err)
	}
	views := make([]eventView, len(events))
	for i, event := range events {
		views[i] = eventView{InboxEvent: event}
		if detail == detailFull {
			views[i].Detail = &events[i].Detail
		}
	}
	return nil, eventsResult{Events: views, Detail: detail}, nil
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
		return 0, app.Errorf(app.KindConflict, "external id %q matches %d items; add profile to disambiguate", externalID, len(items))
	}
}

type itemSessionsInput struct {
	ItemID     int64  `json:"itemId,omitempty"     jsonschema:"Inbox item id. Provide this or externalId."`
	ExternalID string `json:"externalId,omitempty" jsonschema:"External id resolving to one item; provide this or itemId."`
	Profile    string `json:"profile,omitempty"    jsonschema:"Profile id used to disambiguate an externalId that matches items in more than one profile."`
}

type itemSessionsResult struct {
	Sessions []dispatch.ItemSessionView `json:"sessions"`
}

// ListItemSessions reports the hive sessions one inbox item started.
//
// The read reconciles as a side effect: a link a *successful* hive listing
// cannot account for is dropped, while a failed listing drops nothing — absent
// evidence is never evidence of absence.
func (ctrl *Controller) ListItemSessions(ctx context.Context, _ *mcp.CallToolRequest, in itemSessionsInput) (*mcp.CallToolResult, any, error) {
	if in.ItemID == 0 && in.ExternalID == "" {
		return nil, nil, ctrl.toolError(app.Errorf(app.KindInvalid, "provide itemId or externalId"))
	}
	itemID, err := ctrl.resolveItemID(ctx, in.ItemID, in.ExternalID, in.Profile)
	if err != nil {
		return nil, nil, ctrl.toolError(err)
	}
	sessions, err := ctrl.core.Sessions.ItemSessions(ctx, itemID)
	if err != nil {
		return nil, nil, ctrl.toolError(err)
	}
	return nil, itemSessionsResult{Sessions: sessions}, nil
}
