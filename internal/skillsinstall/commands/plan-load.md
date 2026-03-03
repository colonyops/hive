---
description: reads a plan previously written to the repo
---

# Load Plan

Read a plan from `.hive/plans/` to get context for your task.

## Process

1. If a specific plan file is provided as an argument, read that file
2. Otherwise, find the latest plan in `.hive/plans/` (sorted by filename, which uses YYYY-MM-DD prefix)
3. Read the plan file fully to understand the task context

Use `ls .hive/plans/` to see available plans if needed.
