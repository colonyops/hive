# PostHog error tracking source

A **PostHog error tracking source** node emits one item per error-tracking issue in a connected project. It has no inputs — this is where a flow starts.

## Fields

- `credential` — required. The connected PostHog project to fetch as, written as `posthog/<account>`. Connect a project under Settings ▸ Integrations by pasting your instance URL and a personal API key, then picking a project; the key needs the `project:read` and `error_tracking:read` scopes. There is no host or project field here — both are bound to the account when it is connected, so a node cannot point a key at a project it was not connected to.
- `status` — `active` (default), `resolved`, `suppressed`, or `all`. Which issues to fetch.
- `order_by` — `last_seen` (default), `first_seen`, `occurrences`, `users`, or `sessions`. How issues are ranked before `limit` is applied, descending.
- `date_from` — the start of the window the counts cover, as a PostHog relative date such as `-7d` (default) or `-24h`.
- `limit` — how many issues one poll fetches, 1 to 100. Defaults to 25. Because ranking happens before the cut, this is "the top N issues by `order_by`", not "the first N".
- `include_test_accounts` — include traffic PostHog classifies as internal or test. Off by default, matching PostHog's own default.
- `interval` — optional, e.g. `1h`. The shortest time between fetches, for a source that costs more than its freshness is worth. It still only runs on a poll tick, so the real cadence rounds up to the next one; empty fetches every tick. A manual refresh ignores it, and it is not persisted — a restart fetches once from every source.

## Behavior

The source runs in the backend: Go queries the project's error-tracking issues on each tick and appends the result to the event log under topic `source:<flowId>/<nodeId>`. Each issue becomes **one message keyed by its issue id**, which is the roll-up that matters — PostHog has already grouped every occurrence of one exception under that id, so a spike of ten thousand events updates a single feed item instead of flooding the inbox.

The payload carries the issue's `title` as `name: description` — PostHog's `name` is the exception class alone, so a project with several unrelated `TypeError`s would otherwise get several identically-titled items — a `kind` of `Error` so an action can target error items specifically, a `body` holding the occurrence/user/session counts, first/last seen, and the top `source` file for the detail pane, a `url` that deep-links to the issue in PostHog, a `state` of `active`, `resolved` or `suppressed`, the `occurrences`, `users` and `sessions` counts over the `date_from` window, and `firstSeen`/`lastSeen`/`library`/`source` for a `function` node to route on. The issue query returns no stack trace — PostHog exposes those per-issue through a separate sampled-events endpoint, which this node does not call.

Two transitions are called out rather than reported as ordinary updates: an issue that goes terminal is summarized as **Resolved** and archived, and one that comes back out of a terminal state is summarized as **Regressed**. Between those, an issue whose `lastSeen` has advanced since the last poll is a new occurrence; an issue that has not been seen again is a trivial update, so a steadily-firing issue does not re-notify on every tick.

An issue that stops appearing is **not** treated as resolved. The query is filtered by status, bounded by `date_from` and capped by `limit`, so an issue can leave the result set by aging out of the window or being ranked below the cut — archiving on absence would close live issues. Resolution is only recognised when PostHog reports it as a status change, which means an issue resolved in PostHog is seen on the next poll only while it is still inside the configured window.
