# PostHog ingests issues through the error-tracking query endpoint

- **Status:** accepted
- **Date:** 2026-08-03

## Context

The PostHog connector's headline signal is error tracking: new and regressed
issues are what map onto actionable feed items. PostHog exposes three ways to
read them, and they are not equivalent.

`GET /api/projects/:id/error_tracking/issues/` is the obvious REST list and is
the wrong one. Its queryset (`get_issue_detail_queryset`) carries no
`order_by`, so the LIMIT/OFFSET the pagination applies selects an arbitrary
page rather than the newest or the loudest issues. It takes no status filter
and no date window, and its serializer omits `last_seen` and the occurrence
counts — the two fields a feed item is built from. Polling it would return an
arbitrary slice of an unbounded issue history.

`POST /api/projects/:id/query/` with a raw `ErrorTrackingQuery` has the
ordering and the aggregates, but that query type is documented as internal to
PostHog and has already broken external consumers: PostHog/posthog#40075 is
their own MCP server failing after `orderBy` became required.

`POST /api/projects/:id/error_tracking/query/issues/` is the third. It wraps
the same internal query behind a typed request serializer, requires
`error_tracking:read`, carries an `operation_id` and is therefore in the
published OpenAPI document. It takes `status`, `dateRange`, `orderBy` over
`last_seen`/`first_seen`/`occurrences`/`users`/`sessions`, `orderDirection`,
`limit` (1–100), `offset` and `filterTestAccounts`, and answers with `id`,
`name`, `description`, `status`, `first_seen`, `last_seen`, `library` and an
`aggregations` object.

## Decision

1. **Issues come from `error_tracking/query/issues/`.** The internal query
   type's churn is PostHog's to absorb behind their serializer; that is what
   the wrapper is for. The raw `/query/` endpoint is not used, and the plain
   REST issues list is not used.

2. **An issue is keyed by its issue id, never by an event.** PostHog has
   already grouped every occurrence of one exception under that id, so the
   roll-up the feed needs is the identity the API already provides. A spike of
   ten thousand events updates one durable item.

3. **A new occurrence is `lastSeen` advancing, not a poll finding the issue
   again.** The classifier's occurrence key is the raw `lastSeen` string rather
   than a parsed timestamp: a format this connector cannot parse then degrades
   to "no new occurrence" instead of minting a fresh one every tick, which
   would notify on every poll forever.

4. **Neither PostHog connector confirms absence.** The issue query is filtered
   by status, bounded by a date window and capped by a limit, so an issue
   leaving the result set may have aged out or been ranked below the cut —
   archiving on absence would close live issues. The alert list carries every
   alert with its current state, so an alert leaving it was deleted, not
   resolved. This is the deliberate difference from the Grafana alerts
   connector (ADR-less, `sources.grafana_alerts`), whose Alertmanager response
   *is* the complete firing set and which therefore may treat absence as
   authoritative.

5. **A credential binds a host and a project.** The account is
   `<host>-<projectID>`, so two projects on one instance are two credentials
   and can be routed to different feeds. A node names only the credential;
   binding the host and project at connect time is what stops a node from
   pointing a key at a project it was never connected to — the same rule the
   Grafana stack URL follows.

6. **Alert states are normalized on the way in.** PostHog's `AlertState` is a
   display string — `Firing`, `Not firing` — so the connector lowercases and
   snake-cases it. Flows route on `not_firing`, not on what the API returns,
   and a casing change upstream does not silently stop matching.

## Consequences

- The issue query's `limit` is a ranked top-N, not a page-one sample. That is
  the honest shape for a feed and is documented on the node, but it does mean a
  project with more active issues than the limit does not surface all of them
  in one flow.
- An issue resolved in PostHog is only seen while it is still inside the node's
  `date_from` window. A window shorter than the poll gap can miss the
  transition; the item then ages out under the flow's retention rather than
  being archived as resolved.
- If PostHog ever ships an ordered, filterable REST issues list, points 1 and 4
  are what to revisit — absence could become authoritative and the wrapper
  dependency would go away.
- Annotations, survey responses, and PostHog's own error-tracking alerts are
  not ingested. Adding one is a third descriptor in the same package; the node
  config already names its kind explicitly, so nothing about this shape blocks
  them.
