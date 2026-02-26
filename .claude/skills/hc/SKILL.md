---
name: hc
description: Manage honeycomb tasks with hive hc. Use when creating epics/tasks, claiming work, recording checkpoints, or coordinating multi-agent workflows. Covers create, list, show, next, checkpoint, context, and update commands.
compatibility: claude
---

# Honeycomb (hive hc) Task Coordination

Honeycomb is the built-in task system for multi-agent coordination. A conductor agent creates a task tree; worker agents claim and complete items via `hive hc next`.

## hive hc vs hive todo

| Use `hive hc` when... | Use `hive todo` when... |
|----------------------|------------------------|
| Multiple agents share work | Single agent tracking personal reminders |
| Tasks have hierarchy (epics + children) | Flat checklist |
| Workers claim items via `next` | No agent assignment needed |
| Progress checkpoints matter | Simple done/not-done |

## Key Commands

```bash
# Create items
hive hc create "My Epic" --type epic
hive hc create "A Task" --type task --parent <epic-id>

# List and inspect
hive hc list --json                   # all items, LLM-readable JSONL
hive hc list --json --status open     # filter by status
hive hc list --json <epic-id>         # items under a specific epic
hive hc show --json <id>              # item + full activity log

# Claim and update
hive hc next                          # claim next available leaf task
hive hc next <epic-id>                # claim within a specific epic
hive hc update <id> --status in_progress
hive hc update <id> --status done

# Checkpoints (handoff notes)
hive hc checkpoint <id> "Completed X. Next: Y. Blocker: Z."

# Context block (assembled view for LLMs)
hive hc context --json <epic-id>

# Cleanup
hive hc prune --dry-run               # preview what would be removed
hive hc prune --older-than 168h       # remove done/cancelled items older than 1 week
```

## Bulk Creation (Conductor Pattern)

Pass a JSON tree on stdin to create an entire epic + task hierarchy atomically:

```bash
echo '{
  "title": "Implement feature X",
  "type": "epic",
  "children": [
    {"title": "Research existing patterns", "type": "task"},
    {"title": "Write implementation", "type": "task"},
    {"title": "Add tests", "type": "task"}
  ]
}' | hive hc create
```

Output is one JSON line per created item.

## Session Workflow

**Conductor agent** (sets up work):
```bash
# Create epic and tasks
echo '<json tree>' | hive hc create

# Assign tasks to specific sessions
hive hc update <id> --assign <session-id>
```

**Worker agent** (consumes work):
```bash
# Get next available task for this session
hive hc next

# Work on it, then checkpoint progress
hive hc checkpoint <id> "Finished X. Starting Y next."

# Mark complete
hive hc update <id> --status done
```

## Checkpoint Protocol

Record a checkpoint at the end of every session for any in-progress item:

```bash
hive hc checkpoint <id> "What was completed. What comes next. Any blockers."
```

Checkpoints appear in `hive hc show --json <id>` as activity entries with `"type": "checkpoint"`.

## JSON Output (--json flag)

All commands that output items support `--json` for JSONL (one object per line):

```json
{"id":"hc-abc123","type":"epic","status":"open","title":"My Epic","depth":0,"epic_id":"","blocked":false,...}
{"id":"hc-def456","type":"task","status":"in_progress","title":"A Task","depth":1,"epic_id":"hc-abc123",...}
```

Key fields:
- `id` — unique identifier, always `hc-` prefixed
- `type` — `epic` or `task`
- `status` — `open`, `in_progress`, `done`, `cancelled`
- `depth` — 0 for epics, 1+ for children
- `epic_id` — parent epic's ID (empty for epics themselves)
- `blocked` — true if item has open children (leaf tasks are unblocked)

## Messaging Integration

Conductors can monitor worker progress by subscribing to the epic's activity topic:

```bash
hive msg sub "hc.<epic-id>.activity"
```

Every status change and logged activity publishes a notification to this topic automatically.

## Status Values

| Status | Meaning |
|--------|---------|
| `open` | Not started |
| `in_progress` | Claimed and being worked |
| `done` | Completed |
| `cancelled` | Abandoned |
