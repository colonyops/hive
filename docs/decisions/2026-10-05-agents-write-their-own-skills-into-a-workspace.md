# Agents write their own skills into a workspace

- **Status:** accepted
- **Date:** 2026-10-05

## Context

A workspace's `.claude/skills/` and `.agents/skills/` are generated
(ADR workspace-directories-are-generated-and-disposable). If every open
reconciled them to the selected package skills and deleted anything else, a
skill an agent wrote for its own workspace, where its CLI already looks,
would vanish on the next open.

## Decision

Hive owns only the skill directories it installed. Each skills tree holds a
`.hive-installed` list of the slugs the last generation wrote. Generation
removes a listed skill that is no longer selected, reconciles each selected
skill's directory, and leaves every other directory in the tree alone.

- **No new surface.** An agent writes a skill with its own file tools. Hive
  adds no directory, tool, or editor section for it.
- **A tree with no list removes nothing**, because it cannot tell its own
  skills from an agent's.
- **A package skill wins a name both claim.** A selected slug is written over
  whatever is there and joins the list.

## Consequences

- The skills trees are partly authored. The rest of the generated output
  (`CLAUDE.md`, `.mcp.json`, `.codex/`) stays wholly Hive-owned.
- An agent-written skill lives in one tree: one written to `.claude/skills/`
  is not copied to `.agents/skills/`.
- Hive does not validate, list, or manage agent-written skills
  (ADR skill-packages-are-the-unit-a-workspace-enables).
