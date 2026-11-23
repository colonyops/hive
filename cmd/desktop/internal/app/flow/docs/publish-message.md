# Publish message

A **publish-message** node is a terminal (one input, no outputs). Every item
update that reaches it publishes a message to a hive messaging topic, the same
bus `hive msg` reads. An agent or script subscribed to the topic receives it.
It carries the message itself, so a flow does not need a `publish-message`
entry in `actions.yml`. Use an `action` node instead when the same message is
also offered from the item menu.

## Fields

- `topic` (required) -- the topic to publish to, such as `deploys` or
  `agent.<session-id>.inbox`. It is a literal: no wildcards, spaces, or
  templates.
- `message` (required) -- the message body, a Go template.

```yaml
- id: announce
  type: publish-message
  topic: reviews
  message: |
    Review requested on {{ .Payload.repo }}#{{ .Payload.num }}: {{ .Payload.title }}
    {{ .Payload.url }}
```

The sender is `hive-desktop`.

## Templates

`message` renders over the same data a `publish-message` action sees:
`.Payload` is the item, `.Key` its source id, and `.Raw` the item's JSON. A
message that renders blank fails.

When the subscriber is an agent, fence outside content with
`{{ untrustedStart }}` and `{{ untrustedEnd }}` and put
`{{ untrustedNotice }}` before the fence.

## One message per update

The node publishes once for each new version of an item, not once per poll.
An item that changes, such as a pull request that gets a new commit, publishes
again. A failed publish retries a few times and then stays failed; its error
is in Activity.

## Replay

Recomputing a flow on startup or deploy never publishes anything: replay
commits feed memberships only.
