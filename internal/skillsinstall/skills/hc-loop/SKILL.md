---
name: hc-loop
description: >
  Orchestration loop for hive hc tasks. Reads pending tasks, dispatches
  general-purpose sub-agents (via the Agent tool) to implement each task,
  critically reviews the output, and commits only code that passes quality
  checks.
allowed-tools: "Bash(hive hc:*),Bash(git:*),Bash(task:*),Bash(make:*),Bash(go:*),Bash(bun:*),Read,Agent(*)"
version: "2.0.0"
author: "Hayden"
license: "MIT"
argument-hint: "[task-id | --all | --limit N]"
---

# Task Loop — Agent-Dispatched Implementation

You are the **manager**. You do NOT implement tasks yourself. You dispatch sub-agents
using the `Agent` tool, review their output, and commit approved work.

**CRITICAL RULE**: Never write or edit application code directly. ALL implementation
work MUST be delegated to a sub-agent via the `Agent` tool.

## Prerequisites Check

Before starting the loop, verify the environment:

```bash
hive hc list --status open   # confirm tasks exist
git status                   # confirm clean working tree
git branch --show-current    # confirm on a feature branch (never main)
```

If the working tree is dirty, stop and ask the user to stash or commit existing changes first.
If on `main`, stop and ask the user to create a feature branch.

## Step 1: Select Tasks

**If an argument was given** (e.g., `hc-42` or `hc-42,hc-43`):
- Use those specific task IDs

**If `--all` was given**:
- Run `hive hc list --status open` to get all open tasks
- Check parent/child hierarchy — skip tasks with open blockers

**If `--limit N` was given**:
- Take the first N tasks from `hive hc list --status open`

**Default (no args)**:
- Run `hive hc list --status open`
- Present the list to the user and ask which tasks to run this loop iteration

## Step 2: Build Context for Each Task

For each selected task, gather:

1. **Task details**: `hive hc show <id>` — get full description, acceptance criteria, labels
2. **Recent git history**: `git log --oneline -15`
3. **Relevant files**: Based on task description, identify and read the files most likely affected
4. **Test command**: Determine the correct test runner (`task test`, `go test ./...`, `bun test`, etc.)
5. **Lint command**: Determine the correct lint runner (`task lint`, `golangci-lint run`, etc.)

## Step 3: Dispatch Workers via Agent Tool

**You MUST use the `Agent` tool** to dispatch each task to a sub-agent using
`subagent_type: "general-purpose"`.

For each task, call the Agent tool like this:

```
Agent(
  subagent_type: "general-purpose",
  description: "Implement hc-<ID>: <short title>",
  prompt: <see template below>
)
```

### Worker Prompt Template

```
You are implementing hive hc task <ID>: <TITLE>

## Task Description
<full hive hc show output>

## Codebase Context
<relevant file contents with line numbers>

## Recent History
<git log output>

## Commands
- Run tests with: <test command>
- Run lint with: <lint command>

## Instructions
1. Mark this task in-progress: hive hc update <ID> --status in_progress --assign
2. Implement the task as described
3. Run tests and lint — fix any failures
4. Do NOT commit — leave changes unstaged or staged but uncommitted
5. Report back with:
   a) Summary of changes made
   b) List of files modified
   c) Test and lint results
   d) Any open questions
```

### Parallelism Decision

- Dispatch tasks **sequentially** (one at a time) since workers share the same working tree
- Wait for each worker to complete and be reviewed before dispatching the next

## Step 4: Review Each Completed Worker

When a worker agent returns, perform a critical code review of its output.

### Automated Checks

After the worker finishes, check the results:

```bash
git diff HEAD              # read every line
<test command>             # must pass
<lint command>             # must pass
```

### Review Checklist

- [ ] Changes are scoped to the task — no unrelated modifications
- [ ] No introduced security vulnerabilities
- [ ] Error cases handled at system boundaries (not deep in business logic)
- [ ] No over-engineered abstractions for a single use case
- [ ] No TODO/FIXME left in new code
- [ ] Existing tests still pass; new tests added for non-trivial logic
- [ ] Follows codebase naming and structure conventions

### Decision

**APPROVE**: All checks pass and review is clean.

Stage and commit the changes:

```bash
git add -p                              # stage only task-related changes
git commit -m "<clear message>          # see commit message format below
                                        # closes hc-<id>"
hive hc update <id> --status done       # mark task done
```

**REVISE**: Minor issues that a worker can fix.

Resume the worker agent (using its agent ID) with specific feedback and required changes.
Limit to 2 revision cycles before escalating to the user.

**REJECT / ESCALATE**: Fundamental design issue, scope creep, or two failed revisions.

```bash
git checkout -- .                        # discard changes
hive hc update <id> --status open        # return to queue
```

Report the issue to the user with the specific problem and a recommended path forward.

## Step 5: Commit Message Format

```
<imperative summary, max 72 chars>

- <what changed and why, bullet point per file/area>
- <any non-obvious decisions>

closes hc-<id>
```

Example:
```
add pagination to user list endpoint

- add limit/offset query params to GET /users handler
- update UserRepo.List to accept pagination params
- add integration test for edge cases (empty page, last page)

closes hc-47
```

## Step 6: Loop Summary

After all tasks are processed, report:

```
Loop complete.
- Dispatched: N tasks
- Committed:  N tasks (list with commit hashes)
- Revised:    N tasks (list)
- Escalated:  N tasks (list with reasons)

Remaining open: <hive hc list --status open count>
```

## Notes

- **Agent dispatch**: Always use `Agent` tool with `subagent_type: "general-purpose"`.
  Dispatch tasks sequentially since workers share the working tree.
- **Revisions**: Use the `resume` parameter on the Agent tool to continue a worker's
  context when requesting revisions.
- **Branch safety**: Never run on `main`. Always on a feature branch.
- **Context window**: For large tasks, include only the most relevant files in the
  worker prompt. Workers have limited context windows.
- **You are the manager**: Your job is to orchestrate, review, and commit. Never
  implement directly.
