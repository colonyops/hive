# Action

An **action** node is a terminal (1 input, 0 outputs): every message that arrives enqueues an `output_command` against the referenced action.

## Fields

- `action` — the id of an action declared in the desktop `actions.yml` (`launch-session`, `shell`, or `publish-event`).

## Behavior

The output worker executes the action, deduped on `(action_id, msg.Key)`. Actions with `auto_apply: true` fire immediately; others queue for confirmation in the detail pane.
