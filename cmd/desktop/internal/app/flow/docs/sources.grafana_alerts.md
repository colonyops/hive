# Grafana alerts source

A **Grafana alerts source** node emits one item per currently firing Grafana-managed alert on a connected stack. It has no inputs — this is where a flow starts.

## Fields

- `credential` — required. The connected Grafana stack to fetch as, written as `grafana/<account>`. Connect a stack under Settings ▸ Integrations by pasting its URL and a service-account token; a Viewer-role service account is enough to read alerts.
- `matchers` — optional. Alertmanager label matchers, one per entry. An alert must match **every** one to be fetched. The operators are `=`, `!=`, `=~` and `!~`, and the value is passed through untouched:
- `interval` — optional, e.g. `1h`. The shortest time between fetches, for a source that costs more than its freshness is worth. It still only runs on a poll tick, so the real cadence rounds up to the next one; empty fetches every tick. A manual refresh ignores it, and it is not persisted — a restart fetches once from every source.

  ```yaml
  matchers:
    - squad=adaptive-telemetry
    - severity=~critical|warning
    - team!=infra
  ```

  With no matchers the node fetches the stack's entire active alert set. On a large shared stack that is tens of thousands of alerts and tens of megabytes per tick, so a feed scoped to a team should say so here rather than narrow the result in a downstream `function` node.

## Behavior

The source runs in the backend: Go polls the stack's Alertmanager on each tick and appends the result to the event log under topic `source:<flowId>/<nodeId>`. Each firing alert becomes **one message keyed by its fingerprint**, so an alert maps to its own durable item. The payload carries the alert's `title` (its summary annotation, or its `alertname`), a `state` of `firing`, and the raw `labels` and `annotations` for a `function` node to route on.

An alert that is still firing re-reports the same fingerprint and payload and is deduplicated — it produces no new event. When an alert stops firing it leaves the Alertmanager's active set, so the source treats its absence as authoritative: the item is marked `resolved` and archived on the next poll, rather than left to age out.

Filtering happens on the server and does not change item identity — alerts are keyed by fingerprint either way. What it does change is the set this node claims: absence is judged against the *matched* set, so tightening `matchers` archives the alerts that no longer match. That is the correct reading — they are no longer in this node's set — but it means editing matchers reconciles items out of the feed, so change them deliberately.

## Choosing between this and the IRM source

This node reads the stack's own Alertmanager. If a feed is meant to mirror what reaches an on-call channel, `sources.grafana_irm_alerts` is usually the right object instead: alerts evaluated elsewhere and posted straight to an IRM integration never appear here, and IRM groups related alerts where this node lists one item per instance.
