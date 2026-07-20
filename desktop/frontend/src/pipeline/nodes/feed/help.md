# Feed

A **feed** node is a terminal (1 input, 0 outputs): every message that arrives upserts into this node's feed as an unread `feed_item`, rendered in the sidebar.

## Fields

A feed node has no config fields. Its durable feed id is the flow-qualified node id (`<flowId>/<nodeId>`).

## Behavior

A message routed here always lands — there's nothing downstream to wire. New items are marked unread until the user reads them in the sidebar.
