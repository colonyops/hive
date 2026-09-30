---
icon: lucide/plug
description: Supported source providers, local webhooks, and command-based inputs.
---

# Sources

Add sources in the flow editor. Connect provider accounts under **Settings ▸ Integrations**.

| Source | Support | Available inputs |
| --- | --- | --- |
| GitHub | Stable | Search and notifications |
| Gitea and Forgejo | Stable | Filtered search and notifications |
| Grafana | Stable | Managed alerts, IRM alerts, and Prometheus metrics |
| PostHog | Stable | Error tracking and insight alerts |
| RSS feeds | Stable | Entries from an RSS, Atom, or JSON Feed URL |
| Webhooks | Stable | JSON sent to a local endpoint |
| Commands and CLIs | Stable | JSON returned by a shell command |

Provider tokens are stored in the OS keychain. Flow files refer to a connected account by name.

## GitHub

GitHub sources support:

- any GitHub search query;
- the connected account's notification inbox;
- multiple accounts with separate polling and rate limits.

Connect with GitHub's device flow or a classic personal access token. Hive requests `repo` for private repository results and `notifications` for the notification inbox.

Use `sources.github` in a flow. Choose `search` and provide a query, or choose `notifications`.

Search results for pull requests include CI and review status plus added and deleted line counts. These values appear in the feed and detail pane. Function nodes can read `ci`, `review`, `additions`, and `deletions` from the item payload. Notification results do not include these fields.

Hive does not update read, archive, or notification state on GitHub.

## Gitea and Forgejo

Gitea sources work with Gitea and Forgejo instances. They support:

- issue and pull request searches;
- filters for state, owner, labels, involvement, and text;
- the connected account's notifications.

Connect an instance URL and access token under **Settings ▸ Integrations**. The token needs `read:user`, `read:issue`, and `read:notification`. Use `sources.gitea` in a flow.

## Grafana

Connect a Grafana stack with its URL and a Viewer service account token. IRM also needs `grafana-irm-app.alert-groups:read`. The same account can be used by three source types:

- `sources.grafana_alerts` for firing Grafana-managed alerts;
- `sources.grafana_irm_alerts` for active IRM alert groups;
- `sources.grafana_metrics` for a PromQL query result.

Alert sources can be filtered by labels, integration, or team. An IRM alert group's details include the latest source description and common labels. Its payload exposes `alertLabels`, `annotations`, `cluster`, `namespace`, and `severity` to function nodes and action templates. Metrics sources require a Prometheus-compatible data source UID and an expression.

## PostHog

Connect a PostHog instance and personal API key, then select a project. Error tracking needs `project:read` and `error_tracking:read`; insight alerts need `project:read` and `alert:read`. PostHog provides:

- `sources.posthog_errors` for error-tracking issues;
- `sources.posthog_alerts` for insight alerts.

## RSS feeds

An RSS source reads one feed URL. RSS, Atom, and JSON Feed all use the same field; Hive reads the document, not the file extension. Use `sources.rss` in a flow.

The feed must be readable without credentials. This source sends no token and no basic auth.

Set these fields:

- `url`, the feed document;
- `limit`, how many of the newest entries to ingest per fetch (50 by default, 500 at most);
- `interval`, the shortest time between fetches. New nodes start at `30m`. A feed publishes far less often than the poll tick runs.

Each entry becomes an item with kind `Post`. Its title, link, author, categories, and dates come from the feed. Its body is the entry summary reduced to markdown text, with links kept. The feed's own title is shown above the entry. Actions target these items with `applies_to: [Post]`.

How much of that arrives is the feed's choice, not Hive's. A feed that publishes no summary gives you a title and a link. Where a site offers more than one feed, the fuller one is worth using: Hacker News's own feed at `news.ycombinator.com/rss` has no summary and no entry ids, while the same stories through `hnrss.org/frontpage` carry a summary, the submitter, and proper ids. Without ids an entry is matched on its title and date, so editing a title publishes it again as a new item.

The item's link is whatever the feed puts in its `link` element. For an aggregator that is usually the article it points at, not the discussion page.

Each fetch carries the previous response's `ETag` and `Last-Modified`, so an unchanged feed costs one request and no parse. Refreshing the inbox drops that cache and fetches every feed again.

A fetch either returns the whole window or fails. An unreachable host, a non-2xx response, a document over 8 MiB, and a page the parser cannot read as a feed are all failures. A failed fetch changes nothing: the previous entries stay, nothing is archived, and the error is recorded in Activity.

A feed is a window, not a list: publishers drop old entries as they add new ones. Hive does not archive an entry that scrolls off the end, because that means the entry is old, not finished. Adding a node ingests everything still in the window on the first tick, so point a notify node at a busy feed only if you want that.

## Webhooks

A webhook source accepts JSON from scripts and local tools. It listens on:

```
http://127.0.0.1:<port>/hooks/<path>
```

Create a `sources.webhook` node and give it a path. The local HTTP server must be enabled. **Settings ▸ Integrations ▸ Webhooks** shows the current base URL and lets you choose a fixed port.

```sh
curl -X POST "http://127.0.0.1:$PORT/hooks/ci-alerts" \
  -H "Content-Type: application/json" \
  -H "X-Hive-Secret: $SECRET" \
  -d '{
    "id": "build-4821",
    "kind": "build",
    "title": "Unit tests failed",
    "url": "https://ci.example.com/builds/4821",
    "state": "failing"
  }'
```

Webhook rules:

- The listener only accepts local connections.
- Requests use `POST` with a JSON body of up to 1 MiB.
- An optional node secret is sent in `X-Hive-Secret`. Hive stores this secret in the flow file, so handle that file as a shared secret.
- A stable top-level `id` updates the same item on later deliveries.
- `title`, `url`, and `kind` control the basic feed display.
- `resolved`, `closed`, and `done` states archive the item.

Other fields remain available to function nodes and action templates.

!!! tip "Ask the Hive workspace"
    The **Hive** workspace can create the webhook node and sender with the `hive-webhook-sources` skill.

## Commands and CLIs

A command source runs on the polling schedule and reads a JSON snapshot from stdout.

```yaml
- id: todo
  type: sources.exec
  command: "todo list --json | jq -s ."
  timeout: 30s
  interval: 1h
```

The command must define a timeout of no more than two minutes, exit successfully, and print a JSON array. Each object needs a stable top-level `id`. Items missing from the next successful snapshot are archived.

A failed command leaves the previous snapshot unchanged and records the error in Activity.
