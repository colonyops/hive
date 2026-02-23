---
icon: lucide/list-todo
---

# Todos (Experimental)

Hive includes an experimental todo workflow for tracking operator actions requested by agents.

!!! warning "Experimental"
    Todo behavior and command output are experimental and may change in future releases.

## Open the Todo Panel

In the TUI, press `t` to open the todo panel (or run `:TodoPanel` from the command palette).

When the panel opens:

- all `pending` items are auto-acknowledged to `acknowledged`
- the default filter is `Open` (pending + acknowledged)
- items are sorted with open items first, then completed/dismissed

## Todo Panel Keybindings

| Key | Action |
| --- | --- |
| `j` / `down` | Move down |
| `k` / `up` | Move up |
| `tab` | Toggle filter (`Open` / `All`) |
| `enter` | Run action for selected todo URI |
| `c` | Mark selected item completed |
| `d` | Mark selected item dismissed |
| `esc` / `q` | Close panel |

## What Enter Does

`enter` behavior depends on URI scheme:

| Scheme | Behavior |
| --- | --- |
| `session://...` | Marks todo completed immediately |
| `review://...` | Opens the Review view for the document path |
| `http://...`, `https://...` | Opens in your OS browser/app (`open` on macOS, `xdg-open` elsewhere) |
| custom scheme with `todos.actions` | Runs configured shell template |
| custom scheme without `todos.actions` | Falls back to OS open on the full URI |

Additional behavior:

- if URI is empty, Hive shows `no URI on this todo` and does not update status
- external actions complete the todo only when command execution succeeds
- failed external actions keep todo open and show a warning
- `review://...` todos are auto-completed when review is finalized for the matched document

## Create Todos

Use `hive todo add` to create a todo item:

```bash
hive todo add --title "Review plan" --uri "review://.hive/plans/auth.md"
```

Sources:

- `agent` (default) — requires a detected session ID
- `human`
- `system`

## Instructing Agents to Create Todos

Todo creation is an instruction policy, not an automatic hook.

- Define the behavior in your agent instruction file (for example, `AGENTS.md`) and in any shared skills that run work.
- Put the requirement at the end of the workflow as a final step.
- Use exactly one todo when human follow-up is needed, and avoid duplicate todos for the same request.

Recommended final-step template for skills:

```text
Last step (required):
If work is complete and needs human follow-up (or is blocked on human input), create one todo before your final response:
hive todo add --title "<actionable human task>" --uri "review://<path>"
```

Recommended URI choices:

- `review://.hive/...` for plans/research/docs that should be reviewed
- `https://...` for external links
- `session://<id>` only for generic session follow-up when no better URI exists

Example skill footer:

```bash
# Final step: request human follow-up when needed
hive todo add --title "Review auth rollout plan" --uri "review://.hive/plans/auth-rollout.md"
```

## List Todos

List all todos or filter by status:

```bash
hive todo list
hive todo list --status pending
```

## Update Status

Valid update targets:

- `acknowledged`
- `completed`
- `dismissed`

```bash
hive todo update <id> --status acknowledged
hive todo update <id> --status completed
```

You cannot set status back to `pending` from CLI update.

## Status Lifecycle

Allowed transitions are enforced by the todo domain model:

- `pending -> acknowledged`
- `pending -> completed`
- `pending -> dismissed`
- `acknowledged -> completed`
- `acknowledged -> dismissed`

Transitions from `completed` or `dismissed` are rejected.

## Override Enter Actions

Use `todos.actions` to define handlers for custom URI schemes:

```yaml
todos:
  actions:
    jira: "open https://jira.example.com/browse/{{ .Value | shq }}"
    notion: "open {{ .URI | shq }}"
```

Template variables:

- `.Scheme` - URI scheme (`jira`)
- `.Value` - URI value (`PROJ-123` from `jira://PROJ-123`)
- `.URI` - full URI (`jira://PROJ-123`)

Built-in schemes cannot be overridden: `session`, `review`, `http`, `https`.

See [Todo Configuration](../configuration/todos.md) for full settings (actions, limiter, notifications).
