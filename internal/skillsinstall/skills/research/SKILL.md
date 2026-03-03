---
name: research
description: >
  Research codebase comprehensively using parallel sub-agents. Use when the user asks
  for deep research, wants to understand how a feature works, or needs thorough analysis
  of patterns and architecture across the codebase.
allowed-tools: "Bash(git status:*),Bash(git log:*),Bash(git diff:*),WebSearch,Read,Write,Task(*)"
version: "1.0.0"
author: "User"
---

# Research Codebase

Conduct comprehensive research across the codebase by spawning parallel sub-agents and
synthesizing their findings into a research document.

## Context Directory

Research documents are stored in `.hive/research/` managed by `hive ctx`.

**IMPORTANT:** `.hive` must be a symlink, not a directory. If it doesn't exist, run
`hive ctx init` to create it — NEVER use `mkdir`.

## Step 1: Read Mentioned Files

If the user mentions specific files (tickets, docs, JSON), read them **fully** first
using the Read tool WITHOUT limit/offset parameters. Do this before spawning any sub-tasks.

## Step 2: Decompose the Research Question

Break down the query into composable research areas. Identify:
- Specific components, patterns, or concepts to investigate
- Which directories, files, or architectural patterns are relevant
- Cross-component connections and architectural implications

## Step 3: Spawn Parallel Sub-Agent Tasks

Create multiple Task agents to research different aspects concurrently.

Strategy:
- Start with **codebase-locator** agents to find what exists
- Follow up with **codebase-analyzer** agents on the most promising findings
- Run multiple agents in parallel when searching for different things
- Tell agents **what** to look for, not **how** to search

## Step 4: Synthesize Findings

Wait for **all** sub-agents to complete before proceeding.

- Prioritize live codebase findings as primary source of truth
- Use `.hive/` as supplementary historical context
- Connect findings across components
- Include specific file paths and line numbers
- Highlight patterns, connections, and architectural decisions

## Step 5: Gather Metadata

Before writing the document, gather:

```bash
git branch --show-current
git rev-parse HEAD
date -u +"%Y-%m-%dT%H:%M:%SZ"
```

## Step 6: Write the Research Document

**Filename format:** `.hive/research/YYYY-MM-DD-ENG-XXXX-description.md`
- With ticket: `2025-01-08-ENG-1478-parent-child-tracking.md`
- Without ticket: `2025-01-08-authentication-flow.md`

```markdown
---
date: [ISO datetime with timezone]
researcher: [git config user.name]
git_commit: [current commit hash]
branch: [current branch]
repository: [repo name]
topic: "[Research question/topic]"
tags: [research, codebase, relevant-component-names]
status: complete
last_updated: [YYYY-MM-DD]
last_updated_by: [researcher name]
---

# Research: [Topic]

**Date**: [datetime]
**Researcher**: [name]
**Git Commit**: [hash]
**Branch**: [branch]
**Repository**: [repo]

## Research Question

[Original query]

## Summary

[High-level findings answering the question]

## Detailed Findings

### [Component/Area 1]

- Finding with reference (file.ext:line)
- Connection to other components
- Implementation details

### [Component/Area 2]

...

## Code References

- `path/to/file.py:123` - Description
- `another/file.ts:45-67` - Description

## Architecture Insights

[Patterns, conventions, and design decisions discovered]

## Historical Context

[Relevant insights from `.hive/` directory]

## Related Research

[Links to other research documents in `.hive/research/`]

## Open Questions

[Areas needing further investigation]
```

## Step 6a: Create Review Todo

After writing the research document, create a todo for human review:

```bash
hive todo add \
  --title "Review research: <topic-slug>" \
  --uri "review://.hive/research/<filename>"
```

Use the actual filename from Step 6.

## Step 7: Add GitHub Permalinks (if applicable)

If on main or a pushed branch:
```bash
git branch --show-current
gh repo view --json owner,name
```

Replace local file references with permalinks:
`https://github.com/{owner}/{repo}/blob/{commit}/{file}#L{line}`

## Step 8: Present Findings

Summarize key findings concisely to the user. Include file references for navigation.
Ask if follow-up questions or clarification is needed.

## Step 9: Handle Follow-up Questions

Append follow-up research to the same document:
- Update `last_updated` and `last_updated_by` in frontmatter
- Add `last_updated_note: "Added follow-up research for [brief description]"`
- Add a new section: `## Follow-up Research [timestamp]`
- Spawn new sub-agents as needed

## Important Notes

- Always run fresh codebase research — never rely solely on existing research documents
- Read mentioned files FULLY before spawning sub-tasks
- Wait for ALL sub-agents to complete before synthesizing
- Gather metadata before writing (never use placeholder values)
- Keep the main agent focused on synthesis, not deep file reading
- Encourage sub-agents to find examples and usage patterns, not just definitions
- Check `.hive/` for existing research and context
