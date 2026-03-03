---
allowed-tools: Bash(git status:*), Bash(git log:*), Bash(git diff:*), Bash(git commit:*), Bash(git add:*), Bash(git checkout:*)
description: executes a plan previously written to the repo
---

# Execute Plan

Execute a plan from `.hive/plans/`.

## Process

1. If a specific plan file is provided as an argument, use that file
2. Otherwise, find the latest plan in `.hive/plans/` (sorted by filename, which uses YYYY-MM-DD prefix)
3. Read the plan fully to understand your task
4. Ensure you're on a feature branch; create one if needed

For each plan step:

- Mark step as "IN PROGRESS" in the plan
- Implement the necessary changes
- Run appropriate tests/linting
- Commit changes with descriptive messages
- Mark step as "COMPLETED" in the plan
- Document any deviations or blockers encountered
- Make small, focused commits rather than large changes
- Run final verification after completing all steps

The plan is a guide - if you find a better approach during implementation, document your reasoning and proceed with the improved solution.
