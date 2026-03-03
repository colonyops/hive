---
name: project-draft
description: >
  Draft a new project and initial work items in Obsidian. Use when the user wants to
  capture a new project idea, plan work streams, or organize a coding initiative.
allowed-tools: "Bash(mkdir:*),Bash(echo:*),Bash(git:*),Read,Write"
version: "1.1.0"
author: "User"
---

# Project Draft

Create a new project note and initial work items in the Obsidian vault.

## Vault Resolution

The vault root is stored in `$OBSIDIAN_NOTEBOOK_DIR`. Always resolve before writing:

```bash
echo "$OBSIDIAN_NOTEBOOK_DIR"
```

If the variable is empty, stop and tell the user to set `OBSIDIAN_NOTEBOOK_DIR`.

## Interview

Ask the user for the following before writing anything:

1. **Project name** — used as the folder and note title (title case, spaces OK)
2. **Goal** — one or two sentences describing what success looks like
3. **Repos** — which repositories are involved (if any)
4. **Initial work items** — the first concrete things to work on
   - For each item: name, objective, starting phase, priority

Phases: `backlog | research | design | planning | building | review | done | blocked`
Priorities: `high | medium | low`

## File Structure

```
$OBSIDIAN_NOTEBOOK_DIR/Projects/<Project Name>/
├── <Project Name>.md         ← project note
├── Research/                 ← research docs created during project-advance
├── Design Docs/              ← design docs created during project-advance
└── Work Items/
    └── <Work Item Name>.md   ← one file per work item
```

Create all directories before writing:

```bash
mkdir -p "$OBSIDIAN_NOTEBOOK_DIR/Projects/<Project Name>/Work Items"
mkdir -p "$OBSIDIAN_NOTEBOOK_DIR/Projects/<Project Name>/Research"
mkdir -p "$OBSIDIAN_NOTEBOOK_DIR/Projects/<Project Name>/Design Docs"
```

## Project Note Format

```markdown
---
tags:
  - project
type: project
status: active
repos: [<repo1>, <repo2>]
created: YYYY-MM-DD
---

# <Project Name>

## Goal

<goal statement>

## Context

<why this matters and what problem it solves>

## Work Items

- [[<Work Item Name>]]

## Notes
```

## Work Item Note Format

```markdown
---
tags:
  - work-item
type: work-item
project: "[[<Project Name>]]"
phase: <backlog|research|design|planning|building|review|done|blocked>
priority: <high|medium|low>
repos: []
created: YYYY-MM-DD
---

# <Work Item Name>

## Objective

<what this work item accomplishes>

## Acceptance Criteria

- [ ]

## Artifacts

- Research:
- Design Doc:
- Plan:
- PRs:

## Notes
```

## Dates

Use today's date for all `created` fields:

```bash
date +%Y-%m-%d
```

## After Writing

1. List all files created with their full paths
2. Create a review todo pointing to the new project note in Obsidian:

```bash
VAULT="${OBSIDIAN_NOTEBOOK_DIR##*/}"
VAULT_ENC="${VAULT// /%20}"
PROJECT_ENC="${PROJECT_NAME// /%20}"
hive todo add \
  --title "Review new project: <project-name>" \
  --uri "obsidian://vault/${VAULT_ENC}/Projects/${PROJECT_ENC}/${PROJECT_ENC}.md"
```

3. Tell the user they can run `/project-advance` to start progressing any work item
4. Remind them to open `Projects/index.base` in Obsidian for the full dashboard
