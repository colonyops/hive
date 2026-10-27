# Grafana IRM alerts source

A **Grafana IRM alerts source** node emits one item per active Grafana IRM (OnCall) alert group on a connected stack, carrying the group's triage state. It has no inputs — this is where a flow starts.

## Fields

- `credential` — required. The connected Grafana stack to fetch as, written as `grafana/<account>` — the same credential the other Grafana source nodes use. The token needs `grafana-irm-app.alert-groups:read`; if the stack answers `403`, the service account's role is too low rather than the token being wrong.
- `integration` — optional. An IRM integration id (e.g. `CFRPV98RPR1U8`), found on the integration's page in Grafana. This is usually what pins a feed to one squad, because the integration is the unit the upstream routes deliver to. Empty fetches every integration.
- `team` — optional. An IRM team id. Empty fetches every team.
- `interval` — optional, e.g. `1h`. The shortest time between fetches, for a source that costs more than its freshness is worth. It still only runs on a poll tick, so the real cadence rounds up to the next one; empty fetches every tick. A manual refresh ignores it, and it is not persisted — a restart fetches once from every source.

## Behavior

The source runs in the backend. On each tick Go resolves the stack's OnCall API host from the IRM plugin's settings — cached per stack, so it costs one request per connection rather than one per poll — then lists the active alert groups and appends them to the event log under topic `source:<flowId>/<nodeId>`.

Each alert group becomes **one message keyed by its IRM id**. The payload carries:

- `title` — the group's title
- `state` — `firing`, `acknowledged`, `silenced` or `resolved`
- `url` — the group's Slack permalink, falling back to its IRM web page
- `integration`, `team` — the group's scoping ids
- `labels` — sorted `key=value` tags from the source alert and the group's IRM labels
- `alertLabels` and `annotations` — the source alert's common maps, merged with the IRM labels, for a `function` node to route on
- `cluster`, `namespace`, `severity` — commonly acted-on labels lifted to top level
- `alertsCount`, `createdAt`, `acknowledgedAt`, `silencedAt`

The detail body starts with the source alert's description and labels, then shows the IRM group's count and triage timestamps. The connector reads this context from `last_alert.payload`, which the public alert-groups listing embeds, so it does not add one request per group.

`acknowledged` is genuine triage state: someone has picked the alert up. A move between two active states — firing to acknowledged, acknowledged to silenced — is reported as activity, so a feed can react to a colleague taking an alert. A group re-observed at an unchanged state produces no new event.

Absence is authoritative. The listing is the complete active set for the node's scope, so a group that stops appearing is resolved: its item is marked `resolved` and archived on the next poll rather than left to age out. As with matchers on `sources.grafana_alerts`, narrowing `integration` or `team` archives the groups that fall outside the new scope.

## Choosing between this and the Alertmanager source

`sources.grafana_alerts` reads the stack's Grafana Alertmanager, which is a different set. An alert evaluated in another Mimir and posted straight to an IRM integration never reaches the stack's Alertmanager, and alerts that do reach it may carry no team label to scope on. IRM also groups related alerts, so this node emits roughly one item per on-call notification where the Alertmanager source emits one per firing instance.

Reach for this node when the feed should mirror what an on-call channel sees, and for `sources.grafana_alerts` when it should mirror what the stack is evaluating.

For Alertmanager-shaped integrations, the source context comes from the latest notification's `commonLabels` and `commonAnnotations`. Labels that differ between alert instances are not presented as facts about the whole group. Other integration payloads may not expose common maps; in that case the node keeps the IRM labels and triage facts it can read safely.
