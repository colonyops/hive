# Skill installer: install the paste-ready prompts as agent skills, kept in sync by content hash

- **Status:** superseded by ADR skills-are-declared-by-a-workspace
- **Date:** 2026-07-28

## Context

Hive Desktop's configuration is plain text meant to be written by a coding agent,
so the app ships paste-ready prompts (`internal/app/prompts`, ADR go-owned-llm-prompts) — one per
configurable surface (flows, actions, webhook sources, the agent HTTP API,
keyboard shortcuts, app settings). Settings ▸ LLM prompts rendered each one against
this install and offered a copy button.

Copy-paste has two costs. A prompt is only current at the moment it is copied: a
new node or action type, a moved config path, or a rebound port silently makes the
pasted copy stale. And the text lives nowhere the agent can find on its own — the
user has to know it exists and re-paste it into every agent, in every project.

Meanwhile the coding-agent ecosystem converged on a portable format for exactly
this. The **Agent Skills** standard (agentskills.io, adopted by ~40 tools) is a
directory containing a `SKILL.md` (YAML frontmatter — `name`, `description` — plus
Markdown body) that an agent discovers and loads on demand. Claude Code, OpenAI
Codex (which **deprecated** its older custom-prompt mechanism in favour of skills),
the Pi Coding Agent, Gemini CLI, and Cursor all read it; they differ only in which
directory they scan.

## Decision

`internal/app/skills` installs each listed prompt as a `SKILL.md` into the
directories agents scan, and keeps installed files in sync as the app changes what
it would render.

- **One format, four target directories.** A target registry (`targets.go`) holds
  Claude Code (`~/.claude/skills`), Codex (`~/.codex/skills`), pi
  (`~/.pi/agent/skills`), and the portable Agent Skills location
  (`~/.agents/skills`). Each is a **new extension point** in the architecture.md
  sense: a registry entry carrying a **Go path template** and a **Go body
  template**, so a file's location and every frontmatter value are data, not code.
  All four currently share one `SKILL.md` template; a future agent whose file shape
  diverges is one entry plus a template, nothing else. Each target's install
  directory is overridable (a leading `~` is expanded at write time; an unset
  override uses the target's default).

- **The prompt is the body; the slug is the identity.** A skill's `name` is the
  namespaced slug `hive-<id>` (`hive-flows`, …), which — per the standard — is also
  the directory name; the `description` is the prompt's own description plus where
  it applies; the body is the rendered prompt text verbatim. `internal/app/prompts`
  stays the single content engine (it still serves the two context-scoped surfaces,
  a webhook node's transform prompt and the flows-view copy button); `skills` is the
  installer layered on top, and `app.SkillsService` composes the two.

- **Drift is resolved by content hash, and a user edit is never clobbered.** An
  index beside the state dir (`<StateDir>/skills.json`, the `credentials.json`
  precedent) records each installed file's path and the hash of what we last wrote.
  Sync re-renders each indexed skill and compares three hashes: if the file still
  matches what we wrote but the app would now render something different, it is
  **outdated** and rewritten; if the file no longer matches what we wrote, it was
  **edited by the user** and is left untouched (only an explicit re-install
  overwrites it); a **missing** file is recreated. Uninstall deletes only a file
  whose hash we still recognise.

- **All-or-nothing per agent; state is derived, not stored.** The unit of
  management is the agent: turning one on installs *every* skill to it, turning it
  off removes them all (a file the user edited is left in place and reported).
  Whether an agent is "on" is read from the index — does it have any skills
  installed — not from a settings flag, so the toggle can never disagree with the
  files on disk. `settings.yaml`'s `skills` section holds only per-target directory
  overrides and an `auto_update` toggle (default on); there is no per-skill opt-in.
  The Skills tab lists what gets installed with preview + copy per skill (the
  fallback for an agent with no known folder) but has no per-skill install control.
- **Sync provisions on agents; on start it only maintains.** **Sync** (the button)
  reconciles every on agent — installs any skill it is missing (a new node type)
  and updates any that drifted, never clobbering a user edit — and reports what it
  did. On start, non-mock runs **maintain** already-installed skills only — update
  drift and restore a deleted file, but never install a new skill and never touch an
  agent with nothing installed; mock and e2e runs skip it so a fixture launch never
  writes into the real `~`.

## Consequences

- One place to add a skill (a prompt template, as before) and one to add a target
  agent (a registry entry). Neither the settings tab nor the installer branches per
  type.
- Installed skills self-heal: adding a node type or moving a config path re-renders
  every installed copy on the next start, with no user action.
- The keyboard-shortcuts skill's body needs the frontend-owned command catalog, so
  the headless on-start sync leaves that one skill untouched until a sync runs from
  the tab (which supplies the catalog). This is the same reason its prompt is
  omitted from a headless `PromptsService.Catalog`.
- Writing into `~/.claude`, `~/.codex`, `~/.pi`, and `~/.agents` is now a thing the
  app does — but only for a skill the user installed, and never over a file the user
  edited. Uninstall is conservative for the same reason.
- The `settings.yaml` schema gains a `skills` section; a config synced from a build
  that knows more targets still loads, because an unknown target key is ignored by
  the installer rather than rejected by the schema.
