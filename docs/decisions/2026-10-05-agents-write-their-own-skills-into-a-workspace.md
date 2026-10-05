# Agents write their own skills into a workspace

- **Status:** proposed
- **Date:** 2026-10-05

## Context

A workspace's `.claude/skills/` and `.agents/skills/` were wholly Hive-owned
(ADR workspace-directories-are-generated-and-disposable): every open
reconciled them to the skills the enabled packages select and deleted
anything else. A skill only one workspace needs had to go into the shared
library and a package first. An agent that wrote a skill for its own
workspace, where its agent CLI already looks for skills, lost it on the next
open (#508).

## Decision

Hive owns only the skill directories it installed. Each skills tree holds a
`.hive-installed` list of the slugs the last generation wrote. Generation
removes a listed skill that is no longer selected, reconciles each selected
skill's directory as before, and leaves every other directory in the tree
alone.

- **No new surface.** An agent writes a skill with its own file tools into
  the tree its CLI reads. Hive adds no directory, no tool, and no editor
  section for it.
- **A tree with no list removes nothing.** It cannot tell its own skills from
  an agent's, so it keeps everything. After this change, a skill dropped from
  a package before the first open of an existing workspace stays until it is
  deleted by hand.
- **A package skill wins a name both claim.** A selected slug is written over
  whatever is there and joins the list.

## Consequences

- The skills trees are partly authored. The rest of the generated output
  (`CLAUDE.md`, `.mcp.json`, `.codex/`) stays wholly Hive-owned.
- A workspace-only skill exists for one agent's tree at a time: one written to
  `.claude/skills/` is not copied to `.agents/skills/`.
- Hive does not validate, list, or manage agent-written skills.

## Reference

Amends ADR workspace-directories-are-generated-and-disposable for the two
skills trees. Related: ADR skill-packages-are-the-unit-a-workspace-enables.
