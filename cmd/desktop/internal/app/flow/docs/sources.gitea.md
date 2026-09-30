# Gitea source

A **Gitea source** node emits messages from a Gitea or Forgejo instance — either a filtered issue and pull-request search, or that account's notification inbox. It has no inputs; this is where a flow starts. Forgejo serves the same API, so one node type covers both.

## Fields

- `credential` — required. The connected account to fetch as, written as `gitea/<host>-<login>` (for example `gitea/git.example.com-octocat`). Connect an instance under Settings ▸ Integrations.
- `kind` — `search` runs a filtered query; `notifications` drains that account's inbox.
- `limit` — optional max items per fetch (search caps at 100, notifications at 50).

The rest apply to `search` only, and a `notifications` node carrying any of them is rejected:

- `items` — `all` (default), `issues`, or `pulls`.
- `state` — `open` (default), `closed`, or `all`.
- `involving` — return only items the connected account is related to: `created`, `assigned`, `mentioned`, `review_requested`, `reviewed`. Listing several returns the **union** — items matching any one of them.
- `owner` — limit the search to one user's or organization's repositories.
- `labels` — return items carrying any of these labels.
- `text` — free-text search over title and body.
- `interval` — optional, e.g. `1h`. The shortest time between fetches, for a source that costs more than its freshness is worth. It still only runs on a poll tick, so the real cadence rounds up to the next one; empty fetches every tick. A manual refresh ignores it, and it is not persisted — a restart fetches once from every source.

Everything is optional: a search with no filters returns every open issue and pull request the token can see, newest-updated first.

## Behavior

The source runs in the backend: Go polls every enabled flow's source nodes and appends each item to the event log under topic `source:<flowId>/<nodeId>`. This node has one output — every item becomes a `msg` whose payload is the same normalized pull-request/issue shape a GitHub source emits, so the same downstream filters and actions work on both.

Filters are typed fields rather than a query string because Gitea has no search syntax to write one in: its search endpoint takes discrete parameters, and `text` is only a free-text match on title and body. Unknown values are rejected when the flow is saved, because the server itself ignores them and would answer an unfiltered page — a typo would otherwise read as "these are all my open pull requests".

`involving` is the one field that costs more than it looks: the API intersects those relationships, so a union is one request per entry, issued on every poll. Two entries is two requests; the results are merged, deduplicated, and cut back to `limit`.

An item that stops appearing in a search — merged, closed, relabelled out of the filter, or simply pushed off the page by newer activity — has its current state looked up before anything is archived, one request per item. A notifications source usually skips that: Gitea reports each thread's subject state on the notification itself, so most items are archived before they ever leave the inbox — only an item pushed off the page while still open gets the lookup.

The access token needs the `read:user` scope (connecting resolves which account it authenticates as), plus `read:issue` for a search source and `read:notification` for a notifications one.

`credential` is a reference, never a token. Flow files are meant to live in a dotfiles repo, so the secret stays in the OS keychain and only the account name is written here. The host is bound to the account when it is connected, which is why the account half names it — a node cannot point an account's token at a different server, and two instances never collide.
