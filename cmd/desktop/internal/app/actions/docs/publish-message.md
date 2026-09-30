# Publish message

A **publish-message** action renders a message and publishes it durably to one
topic, with sender `hive-desktop` and no session id. Use it to hand work to
another agent or process that is listening on a topic.

## Fields

- `message_template` (required) — the message body.
- `topic` (required) — the destination topic. It must be a **constant literal**:
  no wildcards (`*`) and no template syntax (`{{ }}`). Routing is an authoring
  decision, not a runtime one, so a topic can never be computed from payload
  data.

## Templates

`message_template` is a Go `text/template` rendered over the target's data (see
"Template data" above). On an `item` target the payload is at `.Payload` — e.g.
`{{ .Payload.title }} ({{ .Payload.url }})`; on a `session` or `window` target
the session is at `.Session` — e.g. `{{ .Session.Name }} needs a look`.
