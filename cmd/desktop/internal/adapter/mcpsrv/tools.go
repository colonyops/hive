package mcpsrv

import "github.com/modelcontextprotocol/go-sdk/mcp"

// noInput is the input type of a tool that takes no arguments. The SDK
// requires a struct or map so the inferred schema has type "object".
type noInput struct{}

// The two ends of the surface-wide `detail` ladder.
//
// Every tool whose answer carries source-supplied JSON — an inbox item's
// payload, an event's detail, a dry run's messages — is a tool whose response
// size is set by data nobody here chose. One PR body is kilobytes, and a
// listing repeats it per item while a dry run repeats it per node, so the
// tool that answers happily in a test blows a client's limit against real
// data. Rather than a cap that silently truncates, each such tool takes a
// `detail` argument and reports the level it answered at, so an omitted field
// is never mistaken for an absent one.
//
// Individual tools may add rungs between these (see detailEmitted); summary
// and full mean the same thing everywhere.
const (
	// detailSummary omits every source-supplied payload. What remains is this
	// app's own vocabulary — ids, counters, states, timestamps — which is
	// bounded by the schema rather than by the source.
	detailSummary = "summary"
	// detailFull holds nothing back.
	detailFull = "full"
)

// register declares every tool this adapter serves.
//
// This is the tool table: the one place a tool is declared, carrying metadata
// and nothing else. Dispatch is the handler's, and every handler is a thin
// call into *app.App — the same rule httpapi's operations table follows, and
// the reason a registry used for enumeration never carries dispatch.
//
// Input schemas are inferred from each handler's typed input struct, so a tool
// cannot advertise a field its handler does not accept. Descriptions are
// written for a model rather than for a person reading a reference: what the
// tool answers, what it will not do, and what a surprising answer means.
func (ctrl *Controller) register(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:  "get_status",
		Title: "Desktop status",
		Description: "Report the running build and whether the webhook listener is up, on which host and port. " +
			"Use this first to confirm the app is reachable and to discover the webhook base URL a push should go to.",
	}, ctrl.GetStatus)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "list_profiles",
		Title: "List profiles",
		Description: "List every profile with its load status and whether it has an avatar. " +
			"Profile ids from here are what every other tool's profile argument takes; get_flow turns one into its graph and node ids. " +
			"A profile whose flow file does not parse still lists, with valid=false.",
	}, ctrl.ListProfiles)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "get_flow",
		Title: "Read a profile's flow",
		Description: "Return one profile's graph: every node with its id, type and config, and the wires between them. " +
			"This is where node ids come from — execute_flow's nodeId and the node-image tools take them. " +
			"A profile whose flow file does not parse still answers, with valid=false and the load error.",
	}, ctrl.GetFlow)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "list_feeds",
		Title: "List feeds",
		Description: "List the feeds a profile's graph declares, with their total, unread and archived counts. " +
			"A declared feed nothing has landed in yet still lists, at zero — so an empty feed and a feed id that does not exist are different answers.",
	}, ctrl.ListFeeds)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "list_inbox",
		Title: "List inbox items",
		Description: "List a profile's inbox items, optionally scoped to one feed or resolved by the source's own external id. " +
			"Each item carries the id of the feed claiming it, empty when unrouted. " +
			"detail defaults to summary, which omits each item's raw source payload — a payload is as large as the source made it (a PR body is kilobytes) " +
			"and a listing repeats it per item, so a whole feed at full is megabytes. Ask for full once you have narrowed to the items you need, " +
			"by feed, by externalId or with limit.",
	}, ctrl.ListInbox)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "list_inbox_item_events",
		Title: "List an inbox item's events",
		Description: "List one inbox item's lifecycle events, resolved by itemId or by an externalId that matches exactly one item. " +
			"An externalId matching items in more than one profile is a conflict — pass profile to disambiguate. " +
			"Each event's own detail is source-specific raw JSON and is omitted unless detail is full.",
	}, ctrl.ListInboxItemEvents)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "list_item_sessions",
		Title: "List the hive sessions an inbox item started",
		Description: "List the hive sessions one inbox item created, newest first, each with the state hive reports for it now; " +
			"slug is the tmux session name an attach targets. Resolved by itemId or by an externalId matching exactly one item. " +
			"Links to sessions hive no longer has are dropped as a side effect of this read, but only when hive answered — a failed listing drops nothing.",
	}, ctrl.ListItemSessions)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "list_actions",
		Title: "List the action catalog",
		Description: "List the action catalog with the actions.yml it was loaded from and whether that file currently parses. " +
			"An invalid edit leaves the previous catalog in effect and reports its error here, so this is how to confirm an edit actually loaded.",
	}, ctrl.ListActions)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "refresh_sources",
		Title: "Force a source refresh",
		Description: "Force one producer tick across all sources, dropping fetch caches. " +
			"Returns aggregate totals rather than a per-source breakdown. Use it after changing a flow or pushing a test event instead of waiting for the next poll.",
	}, ctrl.RefreshSources)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_profile",
		Title:       "Create a profile",
		Description: "Create a profile, seeded with the starter graph when exactly one GitHub account is connected and empty otherwise.",
	}, ctrl.CreateProfile)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "delete_profile",
		Title: "Delete a profile",
		Description: "Delete a profile, its flow files, its avatar, and its inbox state. This is not reversible. " +
			"An id that matches no profile is not_found — nothing is ever reported as deleted that was not.",
	}, ctrl.DeleteProfile)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_profile_image",
		Title:       "Read a profile's avatar",
		Description: "Return a profile's avatar as a PNG image. A profile with no avatar is not_found — its rail falls back to a letter chip.",
	}, ctrl.GetProfileImage)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "set_profile_image",
		Title: "Set a profile's avatar",
		Description: "Set a profile's avatar from base64-encoded image bytes (PNG, JPEG, GIF or WebP — the format is sniffed, so no media type is needed). " +
			"The image is normalized to a 128x128 PNG.",
	}, ctrl.SetProfileImage)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "clear_profile_image",
		Title:       "Clear a profile's avatar",
		Description: "Clear a profile's avatar so its rail reverts to the letter chip.",
	}, ctrl.ClearProfileImage)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_node_image",
		Title:       "Read a source node's feed mark",
		Description: "Return a source node's feed-mark image as a PNG. A node with no mark is not_found — its items fall back to the node's glyph.",
	}, ctrl.GetNodeImage)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "set_node_image",
		Title: "Set a source node's feed mark",
		Description: "Set a source node's feed-mark image from base64-encoded image bytes (PNG, JPEG, GIF or WebP). " +
			"It is normalized to a 128x128 PNG and shown on that source's items instead of its icon. " +
			"Webhook and command sources carry a mark; other node types are rejected.",
	}, ctrl.SetNodeImage)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "clear_node_image",
		Title:       "Clear a source node's feed mark",
		Description: "Clear a source node's feed-mark image so its items revert to the node's icon.",
	}, ctrl.ClearNodeImage)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "execute_flow",
		Title: "Dry-run a flow",
		Description: "Dry-run a flow against input you supply and report what every node did, committing nothing — no feed membership, " +
			"inbox rows, notifications, queued actions or durable kv. Name the flow with exactly one of flowId (an installed flow, enabled or not), " +
			"flow (a flow document as a JSON object, same schema as flows/<id>.yaml, version included) or flowYaml (that document as YAML text), " +
			"so an unsaved edit can be executed before it is deployed. Messages are delivered to nodeId — any node, not only a source, which is how " +
			"one function node is exercised in isolation against a captured payload; get_flow is where node ids come from. " +
			"Sources never fetch: a source node relays what you inject. " +
			"Envelope fields left empty are filled in. kv seeds an in-memory sandbox (nodeId -> key -> value) that is the whole world a kv.get sees, " +
			"so notify-once logic is testable against a known starting state; what the run would have written comes back in kvMutations. " +
			"Each node reports its drops, timing, console output, and a structured error with line and column for a script failure, whose line " +
			"numbers are relative to the on_message body you wrote. detail controls how many copies of each message body come back: \"emitted\" " +
			"(the default) reports what each node put on its output ports, \"summary\" drops message bodies entirely — use it for a snapshot run, " +
			"whose payloads would otherwise be repeated once per node per item — and \"full\" adds each node's received messages. " +
			"Prefer this over deploying an edit and waiting for a poll.",
	}, ctrl.ExecuteFlow)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "list_workspaces",
		Title: "List agent workspaces",
		Description: "List every agent workspace with its command template, whether that command carries a permission bypass (danger), and its schedule ids. " +
			"dir is what every schedule tool's workspace argument takes. A workspace whose manifest does not parse still lists, " +
			"with problem set; it refuses schedule writes until the file is fixed.",
	}, ctrl.ListWorkspaces)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "list_schedules",
		Title: "List a workspace's scheduled chats",
		Description: "List one workspace's schedules in manifest order, each with when it fires next and its newest run whatever the outcome. " +
			"A workspace that does not exist is not_found; one whose manifest does not parse is invalid, with the reason.",
	}, ctrl.ListSchedules)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "put_schedule",
		Title: "Create or update a scheduled chat",
		Description: "Create or update one schedule in a workspace's agent-workspace.yaml. A new id is appended and needs cron and prompt. " +
			"An existing id is edited in place, field by field: a field you omit keeps its stored value, so re-timing a paused schedule leaves it paused, " +
			"and name: \"\" clears the name. Every other key and comment in the file is kept. The cron and the prompt template are validated before " +
			"anything is written; a rejected edit is invalid and leaves the file as it was. The schedule is live as soon as the call returns. " +
			"A new or re-timed schedule never fires for a time before it existed.",
	}, ctrl.PutSchedule)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "remove_schedule",
		Title: "Remove a scheduled chat",
		Description: "Delete one schedule from a workspace's agent-workspace.yaml. Its run history stays readable through schedule_runs. " +
			"An id that matches no schedule is not_found; nothing is ever reported as removed that was not.",
	}, ctrl.RemoveSchedule)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "preview_schedule",
		Title: "Dry-run a schedule edit",
		Description: "Report the next occurrences a cron expression produces and the prompt template rendered against sample data, both with a previous run " +
			"behind it and as the first run sees it with .LastRun unset, without saving anything. " +
			"A cron or template that does not parse comes back in cronError or promptError rather than as a failed call. Use it before put_schedule.",
	}, ctrl.PreviewSchedule)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "schedule_runs",
		Title: "Read a schedule's run history",
		Description: "List one schedule's runs, newest first: what each honored, why it ran, whether it launched a chat, and any error. " +
			"An empty list means the schedule has not run. History outlives the schedule, so a removed schedule still answers; a workspace that does not exist, " +
			"or an id that is neither declared nor has ever run, is not_found. A run's session is the chat it launched, gone once that chat has ended. " +
			"Running a schedule now is not a tool here: it spawns an agent CLI, which this server never does. That is the Chats area's \"Run now\".",
	}, ctrl.ScheduleRuns)
}
