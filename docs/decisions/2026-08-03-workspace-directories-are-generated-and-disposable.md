# Workspace directories are generated and disposable

- **Status:** accepted
- **Date:** 2026-08-03

## Context

An agent workspace directory mixes files a user authors
(`agent-workspace.yaml`, `AGENTS.md`, `docs/`) with files `agentws.Generate`
writes on every open (`CLAUDE.md`, the MCP configs such as `.mcp.json` and
`.codex/config.toml`, the skills under `.claude/skills/` and `.agents/skills/`,
an empty `docs/`). The workspace root is user-configurable and iCloud Drive is
an expected destination, so the model has to survive syncing, eviction, and two
machines opening the same workspace.

## Decision

1. **Generated is disposable.** Everything `Generate` writes is a pure
   projection of `GenerateInput` (the manifest, `AGENTS.md`, the resolved MCP
   set, the rendered skills) and is reconciled on every open. It is never
   drift-tracked or merged with a hand edit; a hand edit to a generated file is
   replaced on the next open.
2. **Hive owns the generated files outright.** It never writes into
   `~/.claude` or `~/.codex`, directories it shares with everything else. The one exception is the skills trees: Hive
   reconciles only the skills it installed and leaves an agent's own skills
   alone (ADR agents-write-their-own-skills-into-a-workspace).
3. **Prose lives in `AGENTS.md`, not in YAML.** The manifest has no
   `system_prompt` field. `AGENTS.md` is the file users and agents reach for.
   `CLAUDE.md` is a generated copy of it, never a symlink, because symlinks do
   not sync reliably in iCloud Drive.
4. **An evicted `.AGENTS.md.icloud` stub is reported as a problem**, not
   treated as a missing `AGENTS.md`, so a cold file never empties `CLAUDE.md`.
   An evicted generated file is harmless; the next open rewrites it.
5. **Generation is deterministic byte-for-byte** (stable key order, no
   timestamps), and `writeIfDifferent` writes a file only when its bytes
   change. Two machines regenerating from the same synced input then agree and
   cause no extra sync traffic. Concurrent generation is last-writer-wins per
   file, which is safe because every writer computes the same bytes from the
   same inputs.
6. **codex configuration lives in the workspace tree.** Hive writes nothing to
   `~/.codex/config.toml`. codex asks for directory trust itself, keyed on the
   absolute path, so a synced workspace on a second machine prompts again.

## Consequences

- A hand edit to a generated file is lost on the next open. That is not a bug.
- A new generated file needs no drift index or migration; the reconcile pass
  writes and prunes it like the rest.
- Anything added to the generator must stay a pure function of
  `GenerateInput`. The clock, the environment, and the filesystem outside it
  must not leak in.
- Determinism holds only for identical inputs. Shipped hive skills render
  per-install values (the settings, actions, and flows paths and the MCP
  endpoint), so a workspace synced across machines rewrites those skill files
  on each machine's open. For the same reason the generated subtree does not
  belong in version control.
