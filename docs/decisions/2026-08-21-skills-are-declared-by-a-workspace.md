# Skills are declared by a workspace

- **Status:** accepted
- **Date:** 2026-08-21

## Context

Writing Hive's skills into `~/.claude/skills`, `~/.codex/skills`,
`~/.pi/agent/skills`, and `~/.agents/skills`, directories Hive does not own,
needs a registry, an install index, a drift model, and
a settings section to keep honest. The workspace generator does the same job
inside a directory Hive owns: a workspace enables skill packages
(ADR skill-packages-are-the-unit-a-workspace-enables) and `agentws.Generate`
reconciles them into its disposable `.claude/skills/` and `.agents/skills/`
(ADR workspace-directories-are-generated-and-disposable).

## Decision

A workspace is the only thing that declares skills, and the workspace generator
is the only thing that installs them. The global installer is gone.

- `cmd/desktop/internal/app/skills` keeps only the Agent Skills format: the
  `SKILL.md` template and `ValidateSkill`'s naming rules.
- `app.SkillsService` is a renderer: `ShippedSkills` names what this build
  offers to package patterns, and `RenderSkill` renders one `SKILL.md`.
- `settings.yaml` has no `skills:` section; the v3 migration step
  (`dropSkillsSection`) removes it, since the decoder is strict.
- Files the old installer wrote stay where they are, frozen. The app removes
  the old install index, `<StateDir>/skills.json`, at startup.

## Consequences

- Nothing Hive writes lands outside its own directories, so no
  never-clobber-a-user-edit rule is needed.
- Skills are not available to an agent in an ordinary repository, only inside a
  workspace. A user who wants one elsewhere copies the file, and it will not
  update itself.
- Upgraded users may carry stale `hive-*` skills in those home directories with
  no in-app way to remove them; deleting the directory is the uninstall.
