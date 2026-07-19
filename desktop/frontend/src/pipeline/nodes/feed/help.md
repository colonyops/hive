# Feed

A **feed** node is a terminal (1 input, 0 outputs): every message that arrives upserts into the referenced feed as an unread item, rendered in the sidebar.

## Fields

- `feed` — the id of a feed declared in `profiles/*.yml`. Feed ids are globally unique (they're durable `feed_item` keys).

## Behavior

A message routed here always lands — there's nothing downstream to wire. New items are marked unread until the user reads them in the sidebar.
