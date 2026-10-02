# Launch chat

A **launch-chat** node is a terminal (one input, no outputs). Every item that
reaches it opens a chat in an agent workspace, with the rendered prompt as the
chat's first message. It carries the launch details itself, so a flow that is
the only place a chat starts does not need an entry in `actions.yml`. Use an
`action` node instead when the same launch is also offered from the item menu.

## Fields

- `workspace` (required) — the agent workspace's directory name under the
  configured workspace root.
- `prompt` (required) — the chat's opening message, a Go template.

```yaml
- id: triage
  type: launch-chat
  workspace: incident-triage
  prompt: |
    Triage this alert.

    {{ untrustedNotice }}

    {{ untrustedStart "source" "grafana" }}
    {{ .Payload.title }}

    {{ .Payload.description }}
    {{ untrustedEnd }}
```

The workspace's `command:` must carry `.Prompt` through `shq`; otherwise Hive
refuses the launch rather than dropping the item. The shipped command presets
already do. The workspace's own agent runs the chat, so there is no `agent`
field.

## Templates

`prompt` renders over the same data a `launch-session` action sees: `.Payload`
is the item, `.Key` its source id, and `.Raw` the item's JSON.

Fence outside content with `{{ untrustedStart }}` and `{{ untrustedEnd }}`,
and put `{{ untrustedNotice }}` before the fence, as the `launch-session`
action docs describe. Hive does not fence anything for you. A prompt that
renders blank fails the launch.

## One launch per item

The node launches once per item, not once per update. An item that changes
and reaches the node again with the same key does not open a second chat;
an item that Hive deleted and that later returns counts as new. A
failed launch retries a few times and then stays failed; its error is in
Activity.

## On the item

The chat is linked to the item that started it. The item's detail view lists
the chat, and selecting it opens the chat.

## Replay

Recomputing a flow on startup or deploy never launches anything: replay
commits feed memberships only.
