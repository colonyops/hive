// Package inbox declares the feed, inbox and source operations of the
// desktop's app-control MCP server.
package inbox

import "github.com/colonyops/hive/internal/ops"

var Toolset = ops.Toolset{Name: "inbox", MCPPath: "/mcp"}

func All() []ops.Op {
	return []ops.Op{FeedList, InboxList, InboxEventList, InboxSessionList, SourceRefresh}
}

type FeedListIn struct {
	ProfileID string `json:"profileId" jsonschema:"Profile id to list feeds for; ids come from profile_list."`
}

var FeedList = ops.Spec[FeedListIn]{
	Name: "feed.list", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "List feeds",
	Guidance: "List the feeds a profile's graph declares, with their total, unread and archived counts. " +
		"A declared feed nothing has landed in yet still lists, at zero — so an empty feed and a feed id that does not exist are different answers.",
}

type InboxListIn struct {
	ProfileID  string `json:"profileId,omitempty"  jsonschema:"Profile id to scope to; required when feed is set."`
	ExternalID string `json:"externalId,omitempty" jsonschema:"A source's own id (e.g. a GitHub node id); returns every matching item across profiles."`
	Feed       string `json:"feed,omitempty"       jsonschema:"Feed id from feed_list (e.g. hive/desktop-prs) to return only that feed's items."`
	Archived   bool   `json:"archived,omitempty"   jsonschema:"When true, list archived items instead of active ones."`
	Limit      int    `json:"limit,omitempty"      jsonschema:"Maximum items to return; defaults to 200."`
	Detail     string `json:"detail,omitempty"     jsonschema:"summary (the default) omits each item's raw source payload; full includes it, and its size is the source's to decide."`
}

var InboxList = ops.Spec[InboxListIn]{
	Name: "inbox.list", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "List inbox items",
	Guidance: "List a profile's inbox items, optionally scoped to one feed or resolved by the source's own external id. " +
		"Each item carries the id of the feed claiming it, empty when unrouted. " +
		"detail defaults to summary, which omits each item's raw source payload — a payload is as large as the source made it (a PR body is kilobytes) " +
		"and a listing repeats it per item, so a whole feed at full is megabytes. Ask for full once you have narrowed to the items you need, " +
		"by feed, by externalId or with limit.",
	Fields: ops.Constraints{"detail": ops.SummaryOrFull},
}

type InboxEventListIn struct {
	ItemID     int64  `json:"itemId,omitempty"     jsonschema:"Inbox item id. Provide this or externalId."`
	ExternalID string `json:"externalId,omitempty" jsonschema:"External id resolving to one item; provide this or itemId."`
	ProfileID  string `json:"profileId,omitempty"  jsonschema:"Profile id used to disambiguate an externalId that matches items in more than one profile."`
	Limit      int    `json:"limit,omitempty"      jsonschema:"Maximum events to return; defaults to 50."`
	Detail     string `json:"detail,omitempty"     jsonschema:"summary (the default) omits each event's raw source-specific detail; full includes it."`
}

var InboxEventList = ops.Spec[InboxEventListIn]{
	Name: "inbox_event.list", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "List an inbox item's events",
	Guidance: "List one inbox item's lifecycle events, resolved by itemId or by an externalId that matches exactly one item. " +
		"An externalId matching items in more than one profile is a conflict — pass profileId to disambiguate. " +
		"Each event's own detail is source-specific raw JSON and is omitted unless detail is full.",
	Fields: ops.Constraints{"detail": ops.SummaryOrFull},
}

type InboxSessionListIn struct {
	ItemID     int64  `json:"itemId,omitempty"     jsonschema:"Inbox item id. Provide this or externalId."`
	ExternalID string `json:"externalId,omitempty" jsonschema:"External id resolving to one item; provide this or itemId."`
	ProfileID  string `json:"profileId,omitempty"  jsonschema:"Profile id used to disambiguate an externalId that matches items in more than one profile."`
}

var InboxSessionList = ops.Spec[InboxSessionListIn]{
	Name: "inbox_session.list", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "List the hive sessions an inbox item started",
	Guidance: "List the hive sessions one inbox item created, newest first, each with the state hive reports for it now; " +
		"slug is the tmux session name an attach targets. Resolved by itemId or by an externalId matching exactly one item. " +
		"Links to sessions hive no longer has are dropped as a side effect of this read, but only when hive answered — a failed listing drops nothing.",
}

var SourceRefresh = ops.Spec[ops.NoInput]{
	Name: "source.refresh", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset,
	Summary: "Force a source refresh",
	Guidance: "Force one producer tick across all sources, dropping fetch caches. " +
		"Returns aggregate totals rather than a per-source breakdown. Use it after changing a flow or pushing a test event instead of waiting for the next poll.",
}
