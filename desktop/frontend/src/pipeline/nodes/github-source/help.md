# GitHub source

A **GitHub source** node emits messages from an embedded GitHub search or notifications source. It has no inputs — this is where a flow starts.

## Fields

- `kind` — `search` runs a GitHub Search API query; `notifications` drains the authenticated user's inbox.
- `query` — required for `search`, unused for `notifications`.
- `limit` — optional max items per fetch (search caps at 100, notifications at 50).

## Behavior

The source itself runs in the backend: Go polls every enabled flow's source nodes and appends each item to the event log under topic `source:<flowId>/<nodeId>`. This node has one output — every item becomes a `msg` whose payload mirrors the normalized PR/Issue/notification shape.
