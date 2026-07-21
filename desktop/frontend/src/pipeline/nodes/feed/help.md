# Feed

A **feed** node is a terminal (1 input, 0 outputs): every message that arrives upserts into this node's feed as an unread `feed_item`, rendered in the sidebar.

## Fields

- **Sidebar icon** — an optional glyph shown next to the feed in the sidebar tree. Chosen from a scoped set of feed-relevant icons; unset feeds use the default branch glyph.
- **Description** — optional context (up to 500 characters) surfaced as a tooltip when hovering the feed in the sidebar. Useful for explaining what an LLM-generated feed collects.

The feed's durable id is the flow-qualified node id (`<flowId>/<nodeId>`); these fields are purely cosmetic and never affect which items land in the feed.

## Behavior

A message routed here always lands — there's nothing downstream to wire. New items are marked unread until the user reads them in the sidebar.
