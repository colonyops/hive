# Grafana metrics source

A **Grafana metrics source** node runs a PromQL query against a connected Grafana stack and emits its result. It has no inputs — this is where a flow starts.

## Fields

- `credential` — required. The connected Grafana stack to fetch as, written as `grafana/<account>` (the account is the stack host and org, resolved when you connect). Connect a stack under Settings ▸ Integrations by pasting its URL and a service-account token; a Viewer-role service account is enough to query metrics.
- `datasource_uid` — required. The uid of the Prometheus-compatible datasource the query runs against.
- `expr` — required. A PromQL expression, for example `up` or `sum(rate(http_requests_total[5m]))`.
- `title` — optional. The feed item's title. Defaults to the query when empty.
- `interval` — optional, e.g. `1h`. The shortest time between fetches, for a source that costs more than its freshness is worth. It still only runs on a poll tick, so the real cadence rounds up to the next one; empty fetches every tick. A manual refresh ignores it, and it is not persisted — a restart fetches once from every source.

## Behavior

The source runs in the backend: Go polls every enabled flow's source nodes on each tick and appends the result to the event log under topic `source:<flowId>/<nodeId>`. This node emits **one message per poll**, keyed by the node id, so the whole node maps to a single durable item. The message payload is `{ title, result }`, where `result` is the datasource's query response verbatim (`resultType` plus the `result` series array).

The item's feed presentation — its title and url — is minted at ingest from what this node emits, from the configured `title`. A downstream `function` node reads `msg.Payload.result` and decides **whether** the item appears in a feed by routing it or not; it cannot change the item's title or url. When the function stops routing the item, it leaves the feed on the next poll.

Because a metric's value changes almost every poll, each changed poll appends an event and is routed through the graph. A `function` node fed by this source must therefore be a **pure function of its input** — no dedup or counters that assume one run per change — and the query should return a bounded number of series, since the item's payload carries the whole result.
