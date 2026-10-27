# PostHog ingests issues through the error-tracking query endpoint

- **Status:** accepted
- **Date:** 2026-08-03

## Context

PostHog has three ways to read error-tracking issues. The REST list
`GET /api/projects/:id/error_tracking/issues/` has no ordering, no status
filter, no date window, and omits `last_seen` and the occurrence counts, so a
poll returns an arbitrary slice. The raw `POST /api/projects/:id/query/` with an
`ErrorTrackingQuery` has ordering and aggregates, but that query type is
internal to PostHog and has broken external consumers before
(PostHog/posthog#40075). `POST /api/projects/:id/error_tracking/query/issues/`
wraps the same query behind a typed, published serializer that takes `status`,
`dateRange`, `orderBy`, `orderDirection`, `limit` (1-100), `offset`, and
`filterTestAccounts`.

## Decision

1. **Issues come from `error_tracking/query/issues/`.** Neither the raw
   `/query/` endpoint nor the REST issues list is used.
2. **An issue is keyed by its PostHog issue id**, never by an event. PostHog
   already groups occurrences under that id, so a spike updates one item.
3. **A new occurrence is `lastSeen` advancing.** The occurrence key reads the
   raw `lastSeen` string, not a parsed timestamp, so an unparseable format
   degrades to "no new occurrence" instead of notifying on every poll.
4. **Neither PostHog connector confirms absence.** The issue query is
   filtered, windowed, and capped, so an issue leaving the result may have aged
   out or ranked below the cut. The alert list carries every alert with its
   state, so an alert leaving it was deleted, not resolved. The Grafana alerts
   connector differs on purpose: its Alertmanager response is the complete
   firing set.
5. **A credential binds a host and a project.** The account is
   `<host>-<projectID>`. A node names only the credential, so it cannot point
   a key at a project it was not connected to.
6. **Alert states are normalized.** PostHog's display strings (`Firing`,
   `Not firing`) are lowercased and snake-cased, so flows route on
   `not_firing`.

## Consequences

- `limit` is a ranked top-N. A project with more active issues than the limit
  does not surface all of them.
- A resolve in PostHog is only seen while the issue is inside the node's
  `date_from` window. A window shorter than the poll gap can miss it; the item
  then ages out under the flow's retention.
- If PostHog ships an ordered, filterable REST issues list, revisit points 1
  and 4.
