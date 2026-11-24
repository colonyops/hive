---
icon: lucide/list-checks
---

# Task Tracking

Hive includes a built-in task tracker designed for multi-agent coordination. The `hive hc` command manages epics and tasks that agents can create, claim, and complete — no external issue tracker required.

## Concepts

**Epic** — A top-level grouping of work. Contains tasks.

**Task** — A unit of work under an epic. Agents claim tasks, record progress, and mark them done.

**Auto-detection** — Session ID and repository are detected from the working directory automatically. No configuration needed.

[Hive Desktop](../../desktop/code/terminal-mode.md#tasks) reads the same task tree from its Code area.

!!! tip "The MCP tools"
    `hive hc create`, `list`, `get`, `update`, `next`, `clear`, `comment create` and `context get` run the same operations Hive Desktop serves to agents as MCP tools (`task_next`, `task_update`), so an agent in a Hive chat can claim a task too. Each takes its whole input as JSON with `--input`, so `hive hc create --input -` reads the trees below from stdin. They print JSON Lines when stdout is not a terminal.

## Quick Start

### Create an epic with tasks

```bash
echo '{
  "title": "Implement Authentication",
  "type": "epic",
  "children": [
    {"title": "Add JWT library", "type": "task"},
    {"title": "Implement login endpoint", "type": "task"},
    {"title": "Add session middleware", "type": "task"}
  ]
}' | hive hc create --input -
```

Tasks can be nested under other tasks using `children` to break work into subtrees. `hive hc next` walks the tree and only returns leaf tasks with no incomplete children, so parent tasks act as groupings that resolve automatically when their subtasks are done.

```bash
echo '{
  "title": "Launch MVP",
  "type": "epic",
  "children": [
    {"title": "Backend", "type": "task", "children": [
      {"title": "Set up database schema", "type": "task"},
      {"title": "Implement API endpoints", "type": "task"}
    ]},
    {"title": "Frontend", "type": "task", "children": [
      {"title": "Build login page", "type": "task"},
      {"title": "Build dashboard", "type": "task"}
    ]}
  ]
}' | hive hc create --input -
```

### Express blocker dependencies

Use `ref` and `blockers` to express ordering between sibling tasks in a bulk create. The `ref` field is a local label (not stored) that other tasks can reference in their `blockers` list. `hive hc next` skips tasks that have incomplete blockers.

```bash
echo '{
  "title": "Auth System",
  "type": "epic",
  "children": [
    {"ref": "jwt", "title": "JWT middleware", "type": "task"},
    {"ref": "db", "title": "User schema migration", "type": "task"},
    {"title": "Login endpoint", "type": "task", "blockers": ["jwt", "db"]}
  ]
}' | hive hc create --input -
```

To add or remove blockers after creation:

```bash
hive hc update <id> --add-blocker <blocker-id>    # mark task as blocked by another
hive hc update <id> --remove-blocker <blocker-id>  # remove a blocker
```

### Tag items

Tags group items across epics, for example by area (`frontend`) or by state (`needs-review`). They work like session tags: they compare case-insensitively, keep the first spelling, are at most 40 characters, and cannot contain commas.

```bash
hive hc create "Fix button focus" --parent-id <epic-id> --tags frontend --tags small
hive hc update <id> --add-tag needs-review --remove-tag small
hive hc update <id> --clear-tags
hive hc list --tags frontend --status all      # items carrying every tag given
```

In a bulk create, give any item a `tags` list: `{"title": "Login", "type": "task", "tags": ["backend"]}`.

Output is JSON lines — one per created item:

```
{"id":"hc-a1b2c3d4","type":"epic","title":"Implement Authentication",...}
{"id":"hc-e5f6g7h8","type":"task","title":"Add JWT library","epic_id":"hc-a1b2c3d4",...}
...
```

### Claim the next task

```bash
hive hc next hc-a1b2c3d4 --assign
```

This finds the next open leaf task, assigns it to the current session, and sets its status to `in_progress`.

### Assign a task to another session

An orchestrator that runs outside a session can give a task to a session it dispatched:

```bash
hive hc update hc-e5f6g7h8 --assign-session <session-id>
hive hc create "Add JWT library" --parent-id hc-a1b2c3d4 --assign-session <session-id>
```

The session id can be bare or `local:<id>`, and the session must exist. In a tree passed with `--input`, set `"assignSession"` on a child. `--unassign` clears the assignment.

### Record progress

```bash
hive hc comment create hc-e5f6g7h8 "JWT validation working, added RS256 support"
```

### Complete the task

```bash
hive hc update hc-e5f6g7h8 --status done
```

When an epic is marked `done` or `cancelled`, all non-terminal descendants are automatically cascaded to the same status. Tasks already in a terminal state are not changed.

```bash
hive hc update hc-a1b2c3d4 --status done  # cascades to all open/in_progress children
```

### Edit an item's title or description

```bash
hive hc update hc-e5f6g7h8 --title "New title"
hive hc update hc-e5f6g7h8 --desc "Updated description"
hive hc update hc-e5f6g7h8 --title "New title" --status done  # combine with other flags
```

### Check epic progress

```bash
hive hc context get hc-a1b2c3d4
```

Prints the epic, task counts, your assigned tasks, and open work: markdown at a terminal, one JSON object for an agent or a pipe.

Run `hive hc --help` for the full list of subcommands and flags.
