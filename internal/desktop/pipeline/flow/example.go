package flow

// ExampleFlow is a concrete, worked-example flows/*.yaml document exercising
// every registered node type: github-source -> github-filter ->
// function(outputs: 2) -> {feed, action}. It mirrors feed.ExampleConfig()'s
// role — a helpful starting point rather than an empty file, for docs
// (docs/source-pipeline.md) and any future "start from an example"
// flows-editor affordance.
//
// Unlike feed.ExampleConfig() — which feed.Store.ConfigInfo falls back to
// automatically when profiles.yaml doesn't exist yet — nothing here writes
// this to disk, and FlowStore never seeds an empty flows directory with it.
// The source/feed/action ids below ("team-prs"/"team-review"/"review-pr")
// are placeholders that won't resolve against a real install's
// profiles.yaml/actions.yml, so silently writing this out (or wiring it as
// the flows editor's "New flow" template) would hand a user a
// permanently-broken flow rather than a helpful one. A caller that wants
// this content (docs, or a future template picker) uses the string
// directly.
func ExampleFlow() string {
	return flowFileHeader + `
version: 1
name: Frontend Triage
nodes:
  # A github-source node names one github-* source from profiles/*.yml — it
  # does not fetch GitHub itself, it only reads whatever the backend
  # producer already appended to the event log under topic "source:team-prs".
  - id: in-prs
    type: github-source
    source: team-prs

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

  # Terminal: upserts into the "team-review" feed (profiles/*.yml) as an
  # unread feed_item.
  - id: team-feed
    type: feed
    feed: team-review

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
