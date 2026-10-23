# Grafana IRM alert groups are a sibling source node over the OnCall public API

- **Status:** accepted
- **Date:** 2026-08-05

## Context

`sources.grafana_alerts` reads a stack's Grafana Alertmanager. That is the
wrong set for a feed that mirrors an on-call channel: many alerts are evaluated
elsewhere and POSTed straight to an IRM integration, never reaching the stack's
Alertmanager, and IRM groups related alerts that Alertmanager reports
separately.

## Decision

IRM alert groups are `sources.grafana_irm_alerts`, a separate connector, polled
over the documented OnCall public v1 API with the same `grafana/<account>`
credential. The OnCall host comes from the stack's IRM plugin settings
(`jsonData.onCallApiUrl`) and is cached per connected stack.

- Scoping is the API's own `integration` and `team` filters.
- The payload carries the group's IRM labels plus the common labels and
  annotations from `last_alert`, so the item explains what is firing without a
  request per group.
- Upstream's `new` is emitted as `firing`, so both Grafana nodes share one
  `state` vocabulary.

`sources.grafana_alerts` takes `matchers`, Alertmanager label matchers passed
through as `filter` params.

## Consequences

- Both nodes claim a filtered set, and absence is authoritative against it.
  Editing `matchers`, `integration`, or `team` therefore archives items that
  leave the set, so scope edits should be deliberate.
- The alert-group page walk fails instead of truncating when it hits its page
  bound. A short snapshot would read as "these groups resolved".
- Reading the OnCall API needs an IRM alert-group read permission that a
  Viewer-role service account may lack, so a credential that works for the
  other Grafana nodes can still be refused here.
