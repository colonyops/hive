# GitHub source

A **GitHub source** node emits messages from an embedded GitHub search or notifications source. It has no inputs — this is where a flow starts.

## Fields

- `credential` — required. The connected GitHub account to fetch as, written as `github/<login>` (for example `github/octocat`). Connect an account under Settings ▸ Integrations.
- `kind` — `search` runs a GitHub Search API query; `notifications` drains that account's inbox.
- `query` — required for `search`, unused for `notifications`.
- `limit` — optional max items per fetch (search caps at 100, notifications at 50).
- `interval` — optional, e.g. `1h`. The shortest time between fetches, for a source that costs more than its freshness is worth. It still only runs on a poll tick, so the real cadence rounds up to the next one; empty fetches every tick. A manual refresh ignores it, and it is not persisted — a restart fetches once from every source.

## Behavior

The source itself runs in the backend: Go polls every enabled flow's source nodes and appends each item to the event log under topic `source:<flowId>/<nodeId>`. This node has one output — every item becomes a `msg` whose payload mirrors the normalized PR/Issue/notification shape.

Search-backed pull requests include `ci` (`passing`, `pending`, `failing`, or `none`), `review` (`open`, `draft`, `approved`, `changes_requested`, or `review_required`), `additions`, and `deletions`. Function nodes can use the line totals directly from `msg.Payload`. Notification-backed items omit these fields.

`credential` is a reference, never a token. Flow files are meant to live in a dotfiles repo, so the secret stays in the OS keychain and only the account name is written here. Sources on different accounts fetch independently — separate caches, separate rate limits — so one account being throttled does not stall another.
