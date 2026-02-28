---
name: hc
description: Use hive hc commands to track tasks and epics for agent workflows. Like GitHub Issues but scoped to a repository and designed for machine consumption. Use when creating work items, checking status, recording progress, claiming next work, or querying epic context.
compatibility: claude
---

# Honeycomb (hc) — Multi-Agent Task Coordination

Honeycomb is hive's built-in task coordination system. A conductor agent creates epics and tasks; worker agents claim leaf items, record progress, and mark work done.

Session ID and repo key are auto-detected from the working directory.

## Quick Reference

```
hive hc create <title>           Create a single task or epic
hive hc create                   Bulk create from stdin JSON
hive hc list [epic-id]           List items (optional epic filter)
hive hc show <id>                Show item + comments
hive hc update <id>              Update status or assignment
hive hc next [epic-id]           Get next actionable task
hive hc log <id> <message>       Add a log comment
hive hc checkpoint <id> <msg>    Record a handoff checkpoint
hive hc context <epic-id>        Show epic context block
hive hc prune                    Remove old completed items
```

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
hive hc next --assign

# Or get first, then assign
hive hc next
hive hc update <id> --assign --status in_progress
```

### 3. Record progress with log comments

```bash
hive hc log <id> "Implemented the database schema"
hive hc log <id> "Added rate limiting middleware"
```

### 4. Record a handoff checkpoint when stopping mid-task

```bash
hive hc checkpoint <id> "DB schema done, need to wire API handlers next"
```

The checkpoint command prefixes the message with "CHECKPOINT:" to distinguish it from general log notes.

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
- `--type epic|task` — item type (default: task)
- `--desc <text>` — item description
- `--parent <id>` — parent item ID
- `--assign` — assign to current session after creation

Bulk mode (no positional arg, reads JSON from stdin or `--file`):
- `--file <path>` — read JSON from file instead of stdin

### `hive hc list [epic-id]`

- `[epic-id]` — optional positional arg to filter by epic
- `--status open|in_progress|done|cancelled` — filter by status
- `--session <id>` — filter by session ID

Output: JSON lines, one item per line.

### `hive hc show <id>`

Output: JSON lines — item first, then comments in chronological order.

### `hive hc update <id>`

- `--status open|in_progress|done|cancelled` — new status
- `--assign` — assign to current session
- `--unassign` — remove session assignment

### `hive hc next [epic-id]`

Returns the next actionable leaf task (open/in_progress, no open/in_progress children).

- `[epic-id]` — optional positional arg to scope to an epic
- `--assign` — assign to current session and set status to in_progress

Exits with error if no actionable tasks found.

### `hive hc log <id> <message>`

Adds a general log comment. All positional args after the ID are joined as the message.

### `hive hc checkpoint <id> <message>`

Adds a checkpoint comment prefixed with "CHECKPOINT:". Use when stopping mid-task to leave context for the next agent.

### `hive hc context <epic-id>`

Assembles context block containing:
- Epic title and description
- Task counts by status
- Tasks assigned to current session (with latest comment)
- All open/in-progress tasks

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
