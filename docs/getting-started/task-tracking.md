---
icon: lucide/list-checks
---

# Task Tracking

Hive includes a built-in task tracker designed for multi-agent coordination. The `hive hc` command manages epics and tasks that agents can create, claim, and complete — no external issue tracker required.

## Concepts

**Epic** — A top-level grouping of work. Contains tasks.

**Task** — A unit of work under an epic. Agents claim tasks, record progress, and mark them done.

**Auto-detection** — Session ID and repository are detected from the working directory automatically. No configuration needed.

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
}' | hive hc create
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
}' | hive hc create
```

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

### Record progress

```bash
hive hc comment hc-e5f6g7h8 "JWT validation working, added RS256 support"
```

### Complete the task

```bash
hive hc update hc-e5f6g7h8 --status done
```

### Check epic progress

```bash
hive hc context hc-a1b2c3d4
```

Renders a markdown summary with task counts, your assigned tasks, and open work — designed for AI agent consumption.

Run `hive hc --help` for the full list of subcommands and flags.
