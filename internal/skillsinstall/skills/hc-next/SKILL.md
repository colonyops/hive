---
name: hc-next
description: Start the next hive hc task, prioritizing work related to current branch
argument-hint:
---

# Next - Start Working

Find and start the next task, prioritizing current work context.

## Workflow

1. **Check current branch**: `git branch --show-current`
2. **Find related work** (in order of priority):
   - In-progress issues: `hive hc list --status in_progress`
   - Ready tasks with no blockers: `hive hc next <epic-id> --assign`
3. **Load the task**: `hive hc show <issue-id>` for full context
4. **Start working**: Mark as in-progress if needed, then begin implementation

## Output

Report which task you're starting and why, then get to work.
