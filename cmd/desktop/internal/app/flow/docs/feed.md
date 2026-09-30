# Feed

A **feed** node is a terminal (1 input, 0 outputs): every message routed here creates an unread item under this feed. A feed's durable identity is its flow-qualified node id (`<flowId>/<nodeId>`) — what membership claims are keyed on.

## Fields

- `icon` — optional. One glyph from a curated feed icon set (e.g. `git-branch`, `bell`, `star`, `rss`, `webhook`, `bug`); empty uses the default (`git-branch`).
- `description` — optional, up to 500 characters. Free-text context for what this feed collects — useful for explaining what an LLM-generated feed is for.

`icon` and `description` are cosmetic only and never affect which items land in the feed.

## Behavior

A message routed here always lands — there's nothing downstream to wire. Items stay unread until triaged.

A feed never interrupts: it is a place items live, read at the reader's own pace. To be notified about something, route it to a [notify](notify.md) node — typically a second branch off the same filter, so the items land in the feed *and* raise a banner.
