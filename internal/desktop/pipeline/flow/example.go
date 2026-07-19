package flow

// ExampleFlow is a concrete, worked-example flows/*.yaml document exercising
// every registered node type: github-source -> github-filter ->
// function(outputs: 2) -> {feed, action}. It mirrors the role a starter
// template plays — a helpful worked example rather than an empty file, for
// docs (docs/source-pipeline.md) and any future "start from an example"
// flows-editor affordance.
//
// Nothing here writes this to disk, and FlowStore never seeds an empty flows
// directory with it: the action id below ("review-pr") is a placeholder that
// won't resolve against a real install's actions.yml, so silently writing
// this out (or wiring it as the flows editor's "New flow" template) would
// hand a user a partially-broken flow rather than a helpful one. A caller
// that wants this content (docs, or a future template picker) uses the string
// directly. FlowStore.Create seeds a real, resolvable starter flow instead.
func ExampleFlow() string {
	return flowFileHeader + `
version: 1
name: Frontend Triage
nodes:
  # A github-source node embeds its own GitHub fetch config — a "search"
  # source runs the query, a "notifications" source drains the inbox. The
  # backend producer polls this node and appends its items to the event log.
  - id: in-prs
    type: github-source
    kind: search
    query: "is:open is:pr archived:false"

  # A github-filter node is a client-side gate: groups AND together, values
  # within a group OR, excludes win over includes. Port 0 = pass, port 1 =
  # fail (unwired here, so failing messages are simply discarded).
  - id: drop-bots
    type: github-filter
    exclude_authors: ["*[bot]"]
    repos: ["colonyops/*"]

  # A function node runs author-trusted JS against every message. outputs: 2
  # means on_message must return a 2-element port-indexed array; port 0 fans
  # out to both terminals below, port 1 is left unwired.
  - id: tag
    type: function
    outputs: 2
    on_message: |
      if (msg.Payload.state === "closed") return null; // discard
      msg.Payload.tag = "review";
      return [msg, null];

  # Terminal: the node IS the feed — its items persist as unread feed_item
  # rows keyed by this node's id and show in the sidebar under FEEDS.
  - id: team-feed
    type: feed

  # Terminal: enqueues an output_command against the "review-pr" action
  # (actions.yml — the desktop config dir, not a repo .hive/ directory).
  - id: spawn-review
    type: action
    action: review-pr

wires:
  - { from: in-prs, to: drop-bots }
  - { from: drop-bots, to: tag }
  - { from: tag, out: 0, to: team-feed }
  - { from: tag, out: 0, to: spawn-review }
`
}
