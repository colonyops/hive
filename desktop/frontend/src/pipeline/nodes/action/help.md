# Action

An **action** node is a terminal (1 input, 0 outputs): every message that arrives enqueues an `output_command` against the referenced action.

## Fields

- `action` — the id of an action declared in the desktop `actions.yml` (`launch-session`, `shell`, or `publish-message`).

## Behavior

The output worker executes the action, deduped on `(action_id, msg.Key)`. Actions enqueue runnable work immediately; detail invocations use the same durable deduplication key.
