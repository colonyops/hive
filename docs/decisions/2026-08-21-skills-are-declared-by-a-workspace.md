# Skills are declared by a workspace

- **Status:** accepted
- **Date:** 2026-08-21

## Context

ADR skill-installer wrote the paste-ready prompts into `~/.claude/skills`,
`~/.codex/skills`, `~/.pi/agent/skills` and `~/.agents/skills` — four
directories Hive does not own, affecting every project on the machine whether
or not Hive is involved. Keeping those files honest needed a whole apparatus:
a per-target registry, an install index beside the state dir, a three-hash
drift model that distinguishes an app-side change from a user edit, an
on-start maintenance sync, and a settings section holding directory overrides
and an auto-update toggle.

Since then the workspace generator learned to do the same job properly. A
workspace declares the skill packages it enables, `agentws.Generate`
reconciles the selected skills into its own `.claude/skills/` and
`.agents/skills/`, and that output is disposable by construction
(ADR workspace-directories-are-generated-and-disposable) — nothing to
drift-track, because Hive owns the whole subtree. Packages made the
declaration ergonomic on top of that (ADR skill-packages-are-the-unit-a-workspace-enables).

Two installers for one thing, and the one with the smaller blast radius is
the better one.

## Decision

The global installer is deleted. A workspace is the only thing that declares
skills, and the workspace generator is the only thing that installs them.

- **`internal/app/skills` keeps the format, loses the installation.** What
  survives is the Agent Skills contract: the `SKILL.md` frontmatter template
  and `ValidateSkill`'s naming rules. The target registry, the index, the
  drift states, and the sync semantics are gone, and with them the **Skill
  target** extension point — a "target agent" was only ever a directory to
  write into, and there is no longer a directory to write into.
- **`app.SkillsService` is a renderer.** `ShippedSkills` names what this build
  offers a package's patterns; `RenderSkill` renders one as a `SKILL.md`. No
  settings, no installer, no state.
- **Settings ▸ Skills is removed**, along with `SkillsService`'s Wails
  binding and the `skills:` section of `settings.yaml`. The decoder is
  strict, so dropping the field needs the `settings.yaml` v3 migration step,
  not just a struct edit.
- **Files already installed stay where they are.** They are valid skills, and
  deleting from four home directories on upgrade is a bigger intervention
  than leaving what the user asked for. They are frozen: nothing updates them
  and nothing in the app manages them any more. The install index
  (`<StateDir>/skills.json`) is removed on the first run of a build carrying
  this change, because nothing reads it.
- **`hive-keybindings` stops shipping.** Its body needs the frontend's
  bindable-command catalog, which only the Skills tab supplied; a headless
  `Catalog` already omitted it, so it was already absent from every
  workspace. The prompt and its template are deleted rather than kept as a
  surface nothing reaches, and `prompts.Input.Commands` goes with them.

## Consequences

- One installer, one place a skill can come from, and a scope the user chose:
  a skill reaches an agent because a workspace they opened enables a package
  that selects it.
- Nothing Hive writes lands outside its own directories any more. The
  conservative-uninstall and never-clobber-a-user-edit rules exist because
  the old model wrote into `~`; they are not replaced, because the problem
  they solved is gone.
- Skills are no longer available to an agent working in an ordinary
  repository — only inside a workspace. That is the point of the change, and
  the cost of it: a user who wants Hive's flow-authoring skill in their own
  project copies the file, and it will not self-update.
- Users upgrading past this carry stale `hive-*` skills in up to four
  directories with no in-app way to remove them. Deleting the directory is
  the whole uninstall.
- Rebinding keyboard shortcuts loses its authoring prompt. `settings.yaml`'s
  `keybindings:` section is still documented by the app-settings skill, minus
  the command-id table, which only the frontend could ever produce.

## Reference

Supersedes ADR skill-installer. Related: ADR skill-packages-are-the-unit-a-workspace-enables
(what a workspace declares), ADR workspace-directories-are-generated-and-disposable
(why the generated tree needs no drift model), ADR yaml-config-migration (the
runner the `settings.yaml` v3 step rides).
