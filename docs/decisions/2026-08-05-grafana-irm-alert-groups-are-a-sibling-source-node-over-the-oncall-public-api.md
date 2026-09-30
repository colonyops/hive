# Grafana IRM alert groups are a sibling source node over the OnCall public API

- **Status:** accepted
- **Date:** 2026-08-05

## Context

`sources.grafana_alerts` reads a stack's Grafana Alertmanager. For a feed meant
to mirror an on-call channel that is the wrong set. Measured against one squad's
IRM integration on a Grafana Cloud ops stack, of 31 live alert groups 14 never
reached the stack's Alertmanager at all — they are evaluated in another Mimir and
POSTed straight to the IRM integration — and 4 more carried no team label to
scope on. Grouping differs too: IRM collapses related alerts, so the
Alertmanager node emits roughly twice the items an on-call channel shows.

IRM exposes alert groups through two surfaces, both authenticating with the
Grafana service-account token the `grafana/<account>` credential already holds:
the documented OnCall v1 API on its own regional host, and the stack-local
plugin resource proxy at `/api/plugins/grafana-irm-app/resources/alertgroups/`.

## Decision

IRM alert groups are `sources.grafana_irm_alerts`, a distinct connector rather
than a mode of `sources.grafana_alerts`, polled over the **OnCall public v1
API**. The OnCall host is read from the stack's IRM plugin settings
(`jsonData.onCallApiUrl`) and cached per connected stack, so the extra hop is
paid once per connection rather than once per poll.

Three things follow from choosing the documented surface:

- Scoping is `integration` and `team`, the API's own filter parameters, so a
  feed pins to a squad without restating upstream routing locally.
- The payload carries the group's **IRM** labels plus the common labels and
  annotations from `last_alert.payload`. The public list serializer embeds that
  latest source notification, so the connector can explain what is firing
  without scraping `render_for_web` or issuing one request per group. Labels
  that vary between alert instances are not presented as group facts.
- Upstream's `new` is emitted as `firing`, so both Grafana source nodes share
  one `state` vocabulary and a `function` node can route on it without knowing
  which produced the item.

Separately, `sources.grafana_alerts` gained a `matchers` field that passes
Alertmanager label matchers through verbatim as repeated `filter` params.

## Consequences

Both nodes now claim a *filtered* set, and absence is still authoritative
against it: a poll that fetches the complete matching set may treat anything
missing as resolved. This is what makes editing `matchers`, `integration` or
`team` reconcile items out of the feed — correct, because they are no longer in
the set the node claims, but it means scope edits archive live alerts and should
be deliberate.

The same invariant is why the alert-group page walk fails rather than truncates.
A short snapshot is indistinguishable from "these groups resolved", so
exhausting the page bound returns an error and leaves the previous snapshot
standing instead of archiving everything past the cut.

Reading the OnCall API needs `grafana-irm-app.alert-groups:read`, which a
Viewer-role service account may not carry — unlike the other Grafana source
nodes, a working credential can still be refused.
