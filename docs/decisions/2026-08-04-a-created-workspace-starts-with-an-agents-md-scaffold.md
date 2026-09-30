# A created workspace starts with an AGENTS.md scaffold

- **Status:** accepted
- **Date:** 2026-08-04

## Context

ADR workspace-directories-are-generated-and-disposable split a workspace into authored files (`agent-workspace.yaml`,
`AGENTS.md`, `docs/`) and generated ones (`CLAUDE.md`, MCP configs, skills
trees), with `AGENTS.md` as the home for prose. Nothing wrote one for a
user-created workspace: creation was a directory plus a manifest, so a new
workspace opened with no instructions and no `CLAUDE.md` — only the seeded
`hive/` workspace started with any framing. The goal is a default system
prompt — "you are in a user-defined workspace pre-configured for a purpose" —
that the user can override.

## Decision

`agentws.CreateWorkspace` writes an `AGENTS.md` scaffold alongside the first
manifest: a short framing of what an agent workspace is, plus a Purpose
section to fill in. It is written exactly once, at creation, and is an
authored file from that moment: nothing regenerates it, overriding it is
editing the file, and deleting it deletes it — the same rule the seeded hive
workspace's `AGENTS.md` follows.

A generated preamble tracking the manifest was considered and rejected: it
could only reach claude (codex reads `AGENTS.md` itself, which must never be
generated over), and a generated block inside an authored file is the merge
model ADR workspace-directories-are-generated-and-disposable explicitly declines. The scaffold freezing at creation is the
accepted cost — template improvements reach new workspaces only. The scaffold
deliberately does not enumerate the enabled MCP set, which the agent already
sees through its own wiring, so nothing in the prose goes stale when the
manifest changes.

Hand-authored workspaces (mkdir + YAML, or an agent writing through the
`hive-agent-workspaces` skill) get no scaffold: writing on open-if-missing
would resurrect a deliberately deleted file.

## Consequences

- A fresh UI-created workspace opens with a `CLAUDE.md` (the generated copy of
  the scaffold) instead of nothing; editing `AGENTS.md` is the override path,
  with no new mechanism to learn.
- The scaffold must stay generic enough to survive manifest edits: anything in
  it that names the enabled tool set would be stale the first time the list
  changes.

## Reference

Related decisions: ADR workspace-directories-are-generated-and-disposable (the authored/generated split and why prose lives
in `AGENTS.md`), ADR a-workspace-declares-its-own-authority (the authority model the scaffolded workspace runs
under).
