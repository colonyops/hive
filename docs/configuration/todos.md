---
icon: lucide/list-todo
---

# Todo Configuration (Experimental)

Configure how todo actions execute in the TUI, how aggressively todo creation is limited, and whether toast notifications are shown.

!!! warning "Experimental"
    Todo behavior and settings are experimental and may change in future releases.

## Configuration Shape

```yaml
todos:
  actions:
    jira: "open https://jira.example.com/browse/{{ .Value | shq }}"
  limiter:
    max_pending: 50
    rate_limit_per_session: 10s
  notifications:
    toast: true
```

## Options

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `todos.actions` | `map[string]string` | `{}` | Custom enter-action templates by URI scheme |
| `todos.limiter.max_pending` | `int` | `0` | Max pending todos allowed globally (`0` disables cap) |
| `todos.limiter.rate_limit_per_session` | `duration` | `0` | Min time between new todos per session (`0` disables limit) |
| `todos.notifications.toast` | `bool` | `true` | Show toast notifications when new todos are created |

## Action Templates

Action templates run when you press `enter` on a todo item whose URI scheme matches a key in `todos.actions`.

Template variables:

- `.Scheme` - URI scheme
- `.Value` - URI value portion
- `.URI` - full URI string

Example:

```yaml
todos:
  actions:
    linear: "open https://linear.app/acme/issue/{{ .Value | shq }}"
    docs: "hive review -f {{ .Value | shq }}"
```

Given `linear://ENG-42`, `.Value` resolves to `ENG-42`.

### Built-in Schemes

These schemes have fixed behavior and cannot be overridden:

- `session`
- `review`
- `http`
- `https`

## Enter Behavior Summary

When a todo panel item is activated with `enter`:

- `session://...` -> complete todo directly
- `review://...` -> open review view for that path
- `http(s)://...` -> open via OS handler
- custom scheme with `todos.actions` -> execute rendered template
- custom scheme without `todos.actions` -> fallback to OS open of full URI

For external actions, Hive auto-completes the todo only after a successful command exit.

## Where Todo Creation Is Defined

Hive does not auto-create todos when an agent finishes work.

- Define todo-creation behavior in your instruction layer (`AGENTS.md`, tool prompts, and reusable skills).
- Use `hive todo add` in that instruction text as a required final step when human action is needed.
- Keep `todos.*` config focused on execution behavior (actions, limits, notifications), not creation policy.

## Related Docs

- [Todos (Experimental)](../getting-started/todos.md)
- [Configuration Overview](index.md)
