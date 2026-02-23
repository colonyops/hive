---
icon: lucide/list-todo
---

# Todos (Experimental)

Hive includes an experimental todo workflow for tracking operator actions requested by agents.

!!! warning "Experimental"
    Todo behavior and command output are experimental and may change in future releases.

## Create Todos

Use `hive todo add` to create a todo item:

```bash
hive todo add --title "Review plan" --uri "review://.hive/plans/auth.md"
```

Sources:

- `agent` (default) — requires a detected session ID
- `human`
- `system`

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

## Status Lifecycle

Allowed transitions are enforced by the todo domain model:

- `pending -> acknowledged`
- `pending -> completed`
- `pending -> dismissed`
- `acknowledged -> completed`
- `acknowledged -> dismissed`

Transitions from `completed` or `dismissed` are rejected.
