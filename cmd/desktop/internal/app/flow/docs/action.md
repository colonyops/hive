# Action

An **action** node is a terminal (one input, no outputs). Every arriving
message creates a durable `output_command` for the selected global action.
Commands deduplicate on `(action_id, msg.Key)`, so retries or duplicate graph
invocations cannot repeat a side effect.

To start a session or chat from a flow that is the only caller, use a
`launch-session` or `launch-chat` node instead; it carries the launch details
itself.

## Selecting an action

The `action` field is an id from the global desktop `actions.yml` catalog. The
catalog supports create, edit, delete, and safe external YAML reload. Its
`show_in_detail` flag only controls whether the action can also be invoked
manually on an item; flow action nodes can reference the action regardless of
that flag or its `applies_to` kind scope.

## Execution

This node only selects which catalog action runs — it carries no execution
semantics of its own. `action` resolves to one entry of type `launch-session`,
`shell`, or `publish-message`, and that type's own doc (see
`cmd/desktop/internal/app/actions/docs/`) is what defines how it runs. Action results are
typed to match: a successful run reports whatever that action type produces,
and a failed run remains readable from the durable command record with its
diagnostics.
