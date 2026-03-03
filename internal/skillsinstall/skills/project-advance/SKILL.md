---
name: project-advance
description: >
  Advance a project work item through its lifecycle. Reads work items from the Obsidian
  Projects folder, finds the next ready item by priority and phase, then runs the
  appropriate workflow (research, design-doc, plan-write, hive hc). Use when you want to
  continue project work or let AI pick and execute the next task autonomously.
allowed-tools: "Bash(*),Read,Write,Task(*)"
version: "1.1.0"
author: "User"
---

# Project Advance

Find the next work item to progress and run the appropriate AI workflow for its current phase.

## Vault Resolution

```bash
echo "$OBSIDIAN_NOTEBOOK_DIR"
```

If empty, stop and tell the user to set `OBSIDIAN_NOTEBOOK_DIR`.

## Step 1: Discover Work Items

Scan all work item notes:

```bash
find "$OBSIDIAN_NOTEBOOK_DIR/Projects" -path "*/Work Items/*.md" -type f
```

Read the frontmatter of each file to extract: `phase`, `priority`, `project`, `repos`.

## Step 2: Select Work Item

**If an argument was provided** (file path or partial title), use that specific work item.

**Otherwise**, filter and rank candidates:
- Exclude `phase: done`, `phase: review`, and `phase: blocked` (those need human attention)
- Prefer items already in an active phase (`research`, `design`, `planning`, `building`) over `backlog`
- Then by priority: `high` → `medium` → `low`
- Then by phase progression order: `research` → `design` → `planning` → `building`

Show the user the top 3 candidates and confirm which to advance before proceeding.

## Step 3: Resolve Project Context

Before running any workflow, determine the project name from the work item's `project` field
(e.g., `"[[Adaptive Logs API Alignment]]"` → `Adaptive Logs API Alignment`).

All artifacts for this work item are stored under the project folder:

```
$OBSIDIAN_NOTEBOOK_DIR/Projects/<Project Name>/
├── Research/       ← research docs go here
├── Design Docs/    ← design docs go here
└── Work Items/
    └── <Work Item Name>.md
```

Ensure these subdirectories exist before writing:

```bash
mkdir -p "$OBSIDIAN_NOTEBOOK_DIR/Projects/<Project Name>/Research"
mkdir -p "$OBSIDIAN_NOTEBOOK_DIR/Projects/<Project Name>/Design Docs"
```

## Step 4: Run Workflow by Phase

### `backlog`

Help refine the work item into something actionable:
1. Review the current objective and acceptance criteria with the user
2. Ask clarifying questions: what does done look like? what are the constraints?
3. Update the note with refined content
4. Ask if it's ready to move to `research` phase

### `research`

Run deep codebase and context research:
1. Read the work item's objective, repos, and any existing notes
2. Use parallel research agents (codebase-analyzer, codebase-locator) focused on the repos listed
3. Produce a research summary document
4. Save to Obsidian under the project:
   - Folder: `$OBSIDIAN_NOTEBOOK_DIR/Projects/<Project Name>/Research/`
   - Filename: `YYYY-MM-DD-<slugified-work-item-title>.md`
   - Frontmatter: include `work-item: "[[<Work Item Name>]]"` and `project: "[[<Project Name>]]"`
5. Update the work item's Artifacts section: `- Research: [[<filename>]]`
6. Advance `phase` to `design`

Create a review todo pointing to the research doc in Obsidian:

```bash
VAULT="${OBSIDIAN_NOTEBOOK_DIR##*/}"
VAULT_ENC="${VAULT// /%20}"
FILE_ENC="Projects/${PROJECT// /%20}/Research/${FILENAME// /%20}"
hive todo add \
  --title "Review research: <work-item-title>" \
  --uri "obsidian://vault/${VAULT_ENC}/${FILE_ENC}"
```

### `design`

Create a design document:
1. Read the work item and its linked research doc (if exists)
2. Invoke the `design-doc` skill with this context
3. Save to the project folder:
   - Folder: `$OBSIDIAN_NOTEBOOK_DIR/Projects/<Project Name>/Design Docs/`
   - Follow the obsidian skill conventions for frontmatter and slug filenames
4. Update the work item's Artifacts section: `- Design Doc: [[<filename>]]`
5. Advance `phase` to `planning`

Create a review todo pointing to the design doc in Obsidian:

```bash
VAULT="${OBSIDIAN_NOTEBOOK_DIR##*/}"
VAULT_ENC="${VAULT// /%20}"
FILE_ENC="Projects/${PROJECT// /%20}/Design%20Docs/${FILENAME// /%20}"
hive todo add \
  --title "Review design: <work-item-title>" \
  --uri "obsidian://vault/${VAULT_ENC}/${FILE_ENC}"
```

### `planning`

Write an implementation plan:
1. Read the work item, research doc, and design doc
2. Invoke the `plan-write` command with this context — it will save the plan to `.hive/plans/`
3. Update the work item's Artifacts section:
   - `- Plan: .hive/plans/<filename>`
4. Leave `phase` at `planning` — the work item is now ready to dispatch via `project-dispatch`

Create a review todo for the plan:

```bash
hive todo add \
  --title "Review & dispatch: <work-item-title>" \
  --uri "review://.hive/plans/<plan-filename>"
```

### `building`

Check implementation status:
1. Check hive hc for any linked issues: `hive hc list`
2. Check for open PRs in the linked repos (if using gh CLI)
3. Summarize: what's done, what's in flight, what's blocked
4. If all hive hc issues are done and PRs merged, prompt user to advance to `review`

### `review`

Surface what's ready for human review:
1. List linked PRs and their status
2. Summarize what was built and what acceptance criteria were met
3. Prompt user to verify against acceptance criteria and mark `phase: done` when complete

## Step 5: Update Work Item Note

After completing the workflow step, update the note using the Edit tool:
- Append new artifact links to the Artifacts section
- Update the `phase` frontmatter field to the next phase

When updating frontmatter, do a targeted Edit of just the changed property line — don't rewrite the whole file.

## Advancement Map

| Current Phase | Next Phase | Workflow Triggered        |
|---------------|------------|---------------------------|
| backlog       | research   | refine with user          |
| research      | design     | research agents           |
| design        | planning   | design-doc skill          |
| planning      | —          | plan-write (then dispatch) |
| building      | review     | status check              |
| review        | done       | human verification        |
