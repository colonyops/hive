# The Gitea connector takes typed search filters, not a query string

- **Status:** accepted
- **Date:** 2026-08-12

## Context

`sources.github` configures a search as one `query:` string, because GitHub has
a search DSL and passing it through is the whole interface. Gitea does not.
`GET /repos/issues/search` takes discrete parameters — `state`, `type`,
`labels`, `owner`, `assigned`, `review_requested`, and friends — and its `q` is
only a free-text match over title and body. There is no syntax to carry.

That leaves two shapes for the node's config: mirror GitHub with a `query:`
string in a DSL this connector would have to define and parse, or spell the
parameters as typed fields.

Two facts about the endpoint decide it. First, it does not validate:
`type=bogus` and `state=bogus` are ignored and answered with an unfiltered
page, so anything it does not understand comes back looking like a successful
search. Second, the involvement parameters **intersect**:
`created=true&review_requested=true` returns items that are both, which is
empty in practice — the reading almost nobody wants from a field that lists
several relationships.

## Decision

The config is typed fields (`items`, `state`, `involving`, `owner`, `labels`,
`text`), validated in `Config.Validate` against the enumerations the connector
declares. Nothing reaches the API unchecked.

`involving` is a **union**, and the connector composes it: one request per
entry, merged by item id, re-sorted newest-updated first, and truncated to the
node's `limit`. A partial failure fails the whole `Produce`, because a
successful snapshot is authoritative and half a union would archive everything
the failed half owned.

## Consequences

The schema states exactly what a Gitea search can express, so the editor
renders real controls and a mistyped value fails on save rather than silently
returning everything. Nothing has to own a parser or decide what a GitHub
qualifier means here.

The cost is visible where it is chosen: a union of *n* relationships is *n*
requests per poll, which the editor's hint and the node doc both say. A user who
wants the intersection instead cannot express it — that is the trade, and the
composition (several source nodes into one feed) is the same one GitHub users
make, since GitHub's qualifiers intersect too.

Gitea's vocabulary does not leak into the flow file: `items: all` is the
connector's word for "do not filter", which the client spells by omitting
`type` — the endpoint ignores a `type` it does not recognize, so sending
`type=all` would only work by accident.
