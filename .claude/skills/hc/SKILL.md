---
name: hc
description: |
  Use `hive hc` commands to track tasks and epics for agent workflows.
  Like GitHub Issues but scoped to a repository and designed for machine consumption.

  Use when:
  - Creating work items (epics, tasks)
  - Checking item status or listing open work
  - Recording progress via comments
  - Claiming the next actionable task for a session
  - Querying epic context as a structured summary
compatibility: claude
---

# Honeycomb (hc) — Multi-Agent Task Coordination

Honeycomb is hive's built-in task coordination system. A conductor agent creates epics and tasks; worker agents claim leaf items, record progress, and mark work done.

Session ID and repo key are auto-detected from the working directory.

## Quick Reference

```
hive hc create <title>           Create a single epic or task (tasks require --parent)
hive hc create                   Bulk create from stdin JSON
hive hc list [epic-id]           List items (optional epic filter)
hive hc show <id>                Show item + comments
hive hc update <id>              Update status or assignment
hive hc next <epic-id>           Get next actionable task for an epic
hive hc comment <id> <message>   Add a comment to an item
hive hc context <epic-id>        Show epic context block
hive hc prune                    Remove old completed items
```

## Why This System Helps

The hardest part of agent handoffs isn't knowing *what* to do — it's re-orienting after a context compaction or session switch. `hive hc context` solves this directly: one command gives a structured snapshot of what's done, what's yours, and what's left.

**Commands by value, in order:**

1. **`context`** — run this at the start of every session before doing anything else. It replaces reading through git log, comments, and prior prompts.
2. **`next --assign`** — claim work atomically. Prevents two agents grabbing the same task without any coordination overhead.
3. **`comment` with `CHECKPOINT:`** — the one note you leave before stopping. The next agent reading `context` will see it alongside your task.

**`list --json`** is for programmatic use; the tree view is for humans. Always use `--json` when parsing output.

**`prune`** is a conductor maintenance command — workers don't need to think about it.

**Task granularity matters.** This system is only as useful as the tasks a conductor creates. Coarse tasks ("implement auth") give `next` nothing useful to sequence. Well-decomposed leaf tasks ("add JWT validation middleware", "write login endpoint tests") make `next` and `context` genuinely load-bearing for multi-agent coordination.

---

## Workflow for Worker Agents

### 1. Check context at start of session

```bash
# Get epic context (markdown, suitable for pasting into prompt)
hive hc context <epic-id>

# Get context as JSON for programmatic use
hive hc context <epic-id> --json
```

### 2. Claim the next available task

```bash
# Get and assign in one step
hive hc next <epic-id> --assign

# Or get first, then assign
hive hc next <epic-id>
hive hc update <id> --assign --status in_progress
```

### 3. Record progress (use sparingly)

Comments accumulate in context and consume tokens — only add one when there is something worth preserving for the next agent. Prefer a single substantive note over several incremental ones.

Good reasons to comment:
- A non-obvious decision was made ("chose polling over webhooks because X")
- Stopping mid-task and the next agent needs orientation
- A blocker or external dependency was discovered

```bash
hive hc comment <id> "Switched to optimistic locking — pessimistic caused deadlocks under load"
```

### 4. Record a handoff checkpoint when stopping mid-task

One comment at stop time, prefixed with `CHECKPOINT:`, is enough. Include what is done and what remains.

```bash
hive hc comment <id> "CHECKPOINT: DB schema done, need to wire API handlers next"
```

### 5. Mark task complete

```bash
hive hc update <id> --status done
```

## Workflow for Conductor Agents

### Create an epic with tasks (bulk mode)

```bash
echo '{
  "title": "Implement Authentication",
  "type": "epic",
  "children": [
    {"title": "Add JWT library", "type": "task"},
    {"title": "Implement login endpoint", "type": "task"},
    {"title": "Implement logout endpoint", "type": "task"},
    {"title": "Add session middleware", "type": "task"}
  ]
}' | hive hc create
```

All items are output as JSON lines (one per line):
```
{"id":"hc-abc1","type":"epic","title":"Implement Authentication",...}
{"id":"hc-abc2","type":"task","title":"Add JWT library","epic_id":"hc-abc1",...}
...
```

### Create a single item

```bash
hive hc create "Fix login redirect" --type task --parent <epic-id>
hive hc create "New feature epic" --type epic
```

### Monitor progress

```bash
# List all open tasks for an epic
hive hc list <epic-id> --status open

# List items assigned to a specific session
hive hc list --session <session-id>

# Show item + full comment history
hive hc show <id>
```

## Command Reference

### `hive hc create [title]`

Single-item mode (title as positional arg):
- `--type epic|task` — item type (default: task; tasks require `--parent`)
- `--desc <text>` — item description
- `--parent <id>` — parent item ID (required for tasks)

Bulk mode (no positional arg, reads JSON from stdin or `--file`):
- `--file <path>` — read JSON from file instead of stdin

### `hive hc list [epic-id]`

- `[epic-id]` — optional positional arg to filter by epic (returns children only, not the epic itself)
- `--status open|in_progress|done|cancelled` — filter by status
- `--session <id>` — filter by session ID

Output: JSON lines, one item per line.

### `hive hc show <id>`

Output: JSON lines — item first, then comments in chronological order.

### `hive hc update <id>`

- `--status open|in_progress|done|cancelled` — new status
- `--assign` — assign to current session
- `--unassign` — remove session assignment

### `hive hc next <epic-id>`

Returns the next actionable leaf task (open/in_progress, no open/in_progress children).

- `<epic-id>` — required positional arg to scope to an epic
- `--assign` — assign to current session and set status to in_progress

Exits with error if no actionable tasks found.

### `hive hc comment <id> <message>`

Adds a comment to an item. All positional args after the ID are joined as the message.

Use sparingly — comments accumulate in the context block and are visible to every subsequent agent reading this epic. Prefer one meaningful note over several incremental ones. For handoffs, prefix with `CHECKPOINT:`.

### `hive hc context <epic-id>`

Assembles context block containing:
- Epic title and description
- Task counts by status
- Tasks assigned to current session (with latest comment) — **My Tasks**
- All open/in-progress tasks not assigned to the current session — **Other Open Tasks**

- `--json` — output as single JSON object (default: markdown)

### `hive hc prune`

- `--older-than <duration>` — remove items older than this (default: 168h / 7 days)
- `--status <status>` — statuses to prune, repeatable (default: done, cancelled)
- `--dry-run` — show count without removing

Output: `{"action":"pruned","count":N}`

## Item Structure

```json
{
  "id": "hc-abc123",
  "repo_key": "owner/repo",
  "epic_id": "hc-epic1",
  "parent_id": "hc-epic1",
  "session_id": "mysession",
  "title": "Implement login endpoint",
  "desc": "Optional description",
  "type": "task",
  "status": "in_progress",
  "depth": 1,
  "created_at": "2026-01-01T00:00:00Z",
  "updated_at": "2026-01-01T01:00:00Z"
}
```

Epics have `type: "epic"` and empty `epic_id` (they are their own root).

## Comment Structure

```json
{
  "id": "hcc-def456",
  "item_id": "hc-abc123",
  "message": "CHECKPOINT: auth middleware done, need tests",
  "created_at": "2026-01-01T02:00:00Z"
}
```

## Status Values

| Status        | Meaning                              |
| ------------- | ------------------------------------ |
| `open`        | Not started, available to claim      |
| `in_progress` | Actively being worked on             |
| `done`        | Completed                            |
| `cancelled`   | Abandoned, will not be completed     |
