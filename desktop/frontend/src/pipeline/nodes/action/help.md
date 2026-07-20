# Action

An **action** node is a terminal (one input, no outputs). Every arriving
message creates a durable `output_command` for the selected global action.
Commands deduplicate on `(action_id, msg.Key)`, so retries or duplicate graph
invocations cannot repeat a side effect.

## Selecting an action

The `action` field is an id from the global desktop `actions.yml` catalog. The
catalog supports create, edit, delete, and safe external YAML reload. Its
`show_in_detail` flag only controls whether a manual button appears on a feed
item; flow action nodes can reference the action regardless of that flag or
its `applies_to` kind scope.

## Execution

- **launch-session** renders its prompt and repository templates. A configured
  repository launches headlessly; without one, the detail pane asks for
  repository, session name, and agent before the interactive launch.
- **shell** runs an author-configured command. Failed runs retain bounded
  stdout/stderr diagnostics.
- **publish-message** renders a message and publishes it durably to one fixed,
  literal topic with sender `hive-desktop` and no session id.

Action results are typed: a successful run reports either the launched session
or the published message. Failed runs remain readable from the durable command
record with their diagnostics.
