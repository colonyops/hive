# The Gitea connector takes typed search filters, not a query string

- **Status:** accepted
- **Date:** 2026-08-12

## Context

`sources.github` takes a `query:` string because GitHub has a search DSL. Gitea
does not: `GET /repos/issues/search` takes discrete parameters (`state`, `type`,
`labels`, `owner`, `assigned`, `review_requested`, ...) and `q` is only free
text. The endpoint does not validate (`type=bogus` returns an unfiltered page
that looks like a successful search), and its involvement parameters intersect,
so `created=true&review_requested=true` is empty in practice.

## Decision

A `kind: search` node is configured with typed fields (`items`, `state`,
`involving`, `owner`, `labels`, `text`), validated in `Config.Validate` against
the enumerations the connector declares. Nothing reaches the API unchecked.

`involving` is a **union** the connector composes: one request per entry,
merged by item id, sorted newest-updated first, and truncated to `limit`. A
partial failure fails the whole `Produce`, because a successful snapshot is
authoritative and half a union would archive everything the failed half owned.

`items: all` is the connector's word for "no filter", which the client sends by
omitting `type`.

## Consequences

- A mistyped value fails on save rather than silently returning everything, and
  the editor renders real controls from the schema.
- A union of *n* relationships is *n* requests per poll; the editor hint and the
  node doc say so. An intersection cannot be expressed. Several source nodes
  into one feed is the composition instead, as with GitHub.
