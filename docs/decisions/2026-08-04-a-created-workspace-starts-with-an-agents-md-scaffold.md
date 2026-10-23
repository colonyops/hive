# A created workspace starts with an AGENTS.md scaffold

- **Status:** accepted
- **Date:** 2026-08-04

## Context

`AGENTS.md` is the authored home for a workspace's prose
(ADR workspace-directories-are-generated-and-disposable). A workspace created
in the app needs a default system prompt the user can override.

## Decision

`agentws.CreateWorkspace` writes an `AGENTS.md` scaffold beside the first
manifest: a short framing of what an agent workspace is, plus a Purpose section
to fill in. It is written once, at creation, and is authored from then on.
Nothing regenerates it; overriding it is editing the file, and deleting it
deletes it. The seeded hive workspace's `AGENTS.md` follows the same rule.

A generated preamble that tracks the manifest is rejected: codex reads
`AGENTS.md` directly, so it must never be generated over, and a generated block
inside an authored file is the merge model the disposable-directory ADR
declines.

Hand-authored workspaces get no scaffold. Writing one on open-if-missing would
resurrect a deliberately deleted file.

## Consequences

- Template changes reach new workspaces only.
- The scaffold must stay generic. It does not list the enabled MCP servers or
  skills, which would go stale the first time the manifest changes.
