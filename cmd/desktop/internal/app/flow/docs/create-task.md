# Create task

A **create-task** node is a terminal (one input, no outputs). Every item that
reaches it creates a hive task, the same kind `hive hc create` makes, so an
agent can claim it with `hive hc next`.

## Fields

- `title` (required) -- the task title, a Go template. Line breaks in the
  rendered title become spaces.
- `description` -- the task description, a Go template.
- `repo` -- the repository the task belongs to, as `owner/repo`, a Go
  template. Leave it empty for a task with no repository. A value that does
  not render as `owner/repo` fails the task.
- `parent` -- the id of an existing epic or task to file the task under,
  such as `hc-a1b2c3`. It is a literal, not a template.

```yaml
- id: triage-task
  type: create-task
  title: "Triage {{ .Payload.title }}"
  repo: "{{ .Payload.repo }}"
  parent: hc-a1b2c3
  description: |
    {{ .Payload.url }}

    {{ untrustedNotice }}

    {{ untrustedStart "source" "github" }}
    {{ .Payload.body }}
    {{ untrustedEnd }}
```

## Templates

The templates render over the same data a `launch-session` action sees:
`.Payload` is the item, `.Key` its source id, and `.Raw` the item's JSON.
An agent reads the description, so fence outside content with
`{{ untrustedStart }}` and `{{ untrustedEnd }}` and put
`{{ untrustedNotice }}` before the fence. A title that renders blank fails.

## One task per item

The node creates one task per item, not one per update. An item that changes
and reaches the node again with the same key does not create a second task;
an item that Hive deleted and that later returns counts as new. A failed
create retries a few times and then stays failed; its error is in Activity.

## Replay

Recomputing a flow on startup or deploy never creates a task: replay commits
feed memberships only.
