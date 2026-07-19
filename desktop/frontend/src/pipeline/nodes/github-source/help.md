# GitHub source

A **GitHub source** node emits messages from an already-configured GitHub search or notifications source. It has no inputs — this is where a flow starts.

## Fields

- `source` — the id of a `github-search` or `github-notifications` source defined in `profiles/*.yml`. Must resolve to a source of a matching kind (validated by Go on Deploy).

## Behavior

The source itself runs in the backend: Go polls GitHub on the source's configured `interval` and appends each item to the event log under topic `source:<source-id>`. This node has one output — every item becomes a `msg` whose `Payload` mirrors the normalized PR/Issue/notification shape, and whose `Meta.kind` is one of `pr`, `issue`, or `notification`.
