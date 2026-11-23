---
name: hc
description: |
  Use `hive hc` commands (or the task_* MCP tools) to track tasks and epics for agent workflows.
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

The caller (who claims a task) is `--caller`, else `$HIVE_AGENT_SESSION`, else the session whose checkout holds the working directory. A Hive chat counts as a caller too: a task it claims is assigned to its chat id. The repository comes from `--repo`, else the caller session's remote, else the git remote of the working directory.

## Quick Reference

Every command prints JSON Lines when stdout is not a terminal, as for an agent or a pipe. At a terminal, `list` prints a tree, `get` a styled item and `context get` markdown. Over MCP (Hive Desktop's `hive-desktop` server, or `hive mcp serve --toolset tasks`) the same operations are tools with the names in the last column.

```
hive hc create <title> --type epic                     Create an epic                  task_create
hive hc create <title> --parent-id <id>                Create a task under a parent    task_create
hive hc create --input -                               Create a tree from stdin JSON   task_create
hive hc list [epic-id] [--status all]                  List items (open by default)    task_list
hive hc get <task-id>                                  Show item, then its comments    task_get
hive hc update <task-id> [--status s] [--assign] ...   Change an item                  task_update
hive hc next <epic-id> [--assign]                      Next actionable task, or claim  task_next
hive hc comment create <task-id> "<message>"           Add a comment                   task_comment_create
hive hc context get <epic-id>                          Epic context block              task_context_get
hive hc clear [--older-than 24h] [--dry-run]           Remove finished items           task_clear
```

## Workflow for Worker Agents

### 1. Check context at start of session

```bash
hive hc context get <epic-id>
```

### 2. Claim the next available task

```bash
# Get and assign in one step
hive hc next <epic-id> --assign

# Or get first, then assign
hive hc next <epic-id>
hive hc update <task-id> --assign --status in_progress
```

### 3. Record progress (use sparingly)

Comments accumulate in context and consume tokens — only add one when there is something worth preserving for the next agent. Prefer a single substantive note over several incremental ones.

Good reasons to comment:
- A non-obvious decision was made ("chose polling over webhooks because X")
- Stopping mid-task and the next agent needs orientation
- A blocker or external dependency was discovered

```bash
hive hc comment create <task-id> "Switched to optimistic locking — pessimistic caused deadlocks under load"
```

### 4. Record a handoff checkpoint when stopping mid-task

One comment at stop time, prefixed with `CHECKPOINT:`, is enough. Include what is done and what remains.

```bash
hive hc comment create <task-id> "CHECKPOINT: DB schema done, need to wire API handlers next"
```

### 5. Mark task complete

```bash
hive hc update <task-id> --status done
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
}' | hive hc create --input -
```

All items are output as JSON lines (one per line):
```
{"id":"hc-abc1","type":"epic","title":"Implement Authentication",...}
{"id":"hc-abc2","type":"task","title":"Add JWT library","epic_id":"hc-abc1",...}
...
```

### Create a single item

```bash
hive hc create "Fix login redirect" --parent-id <epic-id>
hive hc create "New feature epic" --type epic
```

### Monitor progress

```bash
# List all open tasks for an epic
hive hc list <epic-id>

# List items assigned to a specific session
hive hc list --session-id <session-id>

# Show item + full comment history
hive hc get <task-id>
```

## Command Reference

Each command takes its fields as flags, or the whole input as JSON with `--input` (inline, `@file`, or `-` for stdin). A refused call prints `{"kind": ..., "message": ...}` and exits with a code for the kind: invalid 2, not_found 3, conflict 4, unavailable 5.

### `hive hc create [title]`

- `--type epic|task` — item type (default: task)
- `--desc <text>` — item description
- `--parent-id <id>` — parent item (required for a task with no children)
- `children` — set through `--input`: a list of `{title, type, desc, ref, blockers, children}`. An item with children must be an epic. `ref` is a label local to the call; `blockers` lists refs that must finish first.
- `--repo owner/repo` — the repository (default: from the caller)

Output: every created item, root first.

### `hive hc list [epic-id]`

- `[epic-id]` — filter by epic (returns children only, not the epic itself)
- `--status open|in_progress|done|cancelled|all` — default: open
- `--session-id <id>` — filter by assignee (bare or `local:<id>`)
- `--all-repos` — every repository

### `hive hc get <task-id>`

Output: the item first, then comments in chronological order.

### `hive hc update <task-id>`

- `--status open|in_progress|done|cancelled` — new status
- `--title <text>` — new title (empty string rejected)
- `--desc <text>` — new description (empty string clears it)
- `--assign` — assign to the caller (a session or chat)
- `--unassign` — remove the assignment
- `--add-blocker <id>` — an item that must finish first; a cycle is a conflict (exit 4)
- `--remove-blocker <id>` — remove an explicit blocker

**Cascade**: when an epic transitions to `done` or `cancelled`, all non-terminal descendants are automatically set to the same status. Items already in a terminal state are not changed.

### `hive hc next <epic-id>`

Returns the next actionable leaf task (open/in_progress, no open/in_progress children or blockers).

- `--assign` — resume a task already in progress for the caller, else claim the next open one and set it in_progress

No actionable task is not_found (exit 3).

### `hive hc comment create <task-id> <message>`

Quote the message: it is one argument.

Use sparingly — comments accumulate in the context block and are visible to every subsequent agent reading this epic. Prefer one meaningful note over several incremental ones. For handoffs, prefix with `CHECKPOINT:`.

### `hive hc context get <epic-id>`

Returns the context block as one JSON object (markdown at a terminal):
- `epic` — the epic item
- `counts` — task counts by status
- `my_tasks` — the caller's tasks, each with its latest comment
- `all_open_tasks` — every open or in-progress task

### `hive hc clear`

- `--older-than <duration>` — only items older than this (default: any age)
- `--statuses <status>` — statuses to remove, repeatable (default: done, cancelled)
- `--all-repos` — every repository; needed outside a git repository
- `--dry-run` — count without removing

Output: `{"action":"pruned","count":N}` (`"would prune"` for a dry run)

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
