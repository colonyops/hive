# Skill packages are the unit a workspace enables

- **Status:** accepted
- **Date:** 2026-08-11

## Context

A workspace declares two kinds of capability: the MCP servers its agent can
call and the skills its agent can read. Only MCPs had a management surface.
`mcps.yaml` is a library — declaring a server there enables it nowhere — and a
workspace's `mcps:` list is what enables one.

Skills worked the other way: `agentws.Generate` scanned `<root>/.shared/skills`
and merged every directory it found into every workspace's tree. A skill was
therefore in all workspaces or none, nothing in the app said which skills a
workspace carried, and the only slugs a workspace could name were the `hive-*`
ones the build ships.

Making individual skills the enablement unit fixes the scoping but produces a
list that grows without bound — seven shipped skills before a user authors
one — and it makes every new skill a manifest edit in every workspace that
wants it. The shipped set is already a de-facto bucket, and the startup rewrite
that kept the seeded `hive` workspace's list in step with it
(`SyncHiveWorkspaceSkills`) was that missing concept showing through as a
workaround.

## Decision

`skills.yml` sits beside `mcps.yaml` and defines **packages**: a named set of
glob patterns over skill names. A workspace's `skills:` list names packages,
never individual skills, and generation installs the union of what the enabled
packages select.

```yaml
version: 1
packages:
  hive:
    title: Hive
    include: ["hive-*"]
  infra:
    include: ["terraform-*", "k8s-*", "runbook"]
    exclude: ["terraform-experimental"]
```

- **Patterns, not membership lists.** A package holds no copy of a skill, so
  one skill can belong to several packages and a newly authored skill joins
  every workspace whose package already matches its name — no manifest edited
  anywhere. `Include` admits, `Exclude` carves out, and a pattern with no
  wildcard is an exact name, so naming one skill needs no separate syntax.
- **One name-space, two sources.** Names come from the skills this build ships
  (rendered per install by the prompt engine) and from
  `.shared/skills/<name>/SKILL.md`. Patterns match across both, which is what
  lets `hive-*` select the shipped set. A shared file of the same name as a
  shipped skill wins: a slug is one skill, and the authored copy is the one the
  user can see and edit. The shipped half arrives in `agentws` as data
  (`ShippedSkill`) because `internal/app/prompts` imports `agentws` and the
  dependency cannot run the other way.
- **The union needs no collision rule.** Two packages selecting the same skill
  select the same file; the identity is the name, so the second occurrence is
  dropped and nothing has to arbitrate. This is a direct consequence of
  packages being patterns over a flat name-space rather than directories that
  own their contents.
- **The generator stays pure.** `Generate` expands no pattern and reads no
  directory: resolution happens in the service and it installs exactly the
  skills it is handed (spec §4.4). `GenerateInput.Shared` is gone.
- **A package that does not resolve is reported, not fatal.** An enabled name
  `skills.yml` does not define lands in `OpenResult.MissingPackages` and the
  workspace still opens, the way a missing MCP id already behaved. A package
  that resolves to zero skills lists with no members rather than being hidden —
  an empty package is a pattern to fix.
- **Validation is at load.** A package with no `include`, a pattern
  `path.Match` cannot compile, or a name that is not a path-safe segment fails
  the file rather than silently selecting nothing. A broken `skills.yml` keeps
  the last-good set, like `mcps.yaml`.

Authoring stays file work: the editor links to `skills.yml` and to the shared
skills directory. A skill is a `SKILL.md` in the format every target agent
already reads, so it installs verbatim and needs no in-app author flow.

## Consequences

- `SyncHiveWorkspaceSkills` is deleted. The seeded workspace declares
  `skills: [hive]` and the package's `hive-*` pattern tracks the shipped set,
  so a release that adds or removes a skill needs no startup rewrite — and a
  package switched off there stays off, which the rewrite made impossible.
- An install whose `.shared/skills/` entries relied on the implicit merge loses
  them until a package selects them and a workspace enables it. This is a
  deliberate, unmigrated behaviour change: the merge and the enablement list
  cannot both be true.
- An existing `hive` workspace whose manifest still enumerates skill slugs
  (`hive-mcp`, …) reports them as missing packages until its `skills:` list is
  changed to `[hive]`. Startup seeds `skills.yml`, so the package exists and
  the editor's toggle is the fix; no migration code is written for this.
- Grouping is no longer a presentation concern. An earlier iteration grouped
  individually-enabled skills by slug prefix in the editor; packages make that
  redundant and it is removed.
- The two capability halves stay symmetric in shape — a library file at the
  root, a per-workspace list naming things in it — but not in expressive power:
  `mcps.yaml` has no equivalent of a pattern. If server sets ever want the same
  treatment, this is the precedent to follow.
- What packages still do not do is *distribution*: there is no source to
  install a package from and no update path. Adding one is a separate decision
  and would want versioning and drift reporting, not more grouping.

## Reference

Related decisions: ADR workspace-directories-are-generated-and-disposable (the
generated/authored split the skills tree lives in), ADR
a-workspace-declares-its-own-authority (the capability declaration this is the
other half of), ADR skill-installer (the drift-tracked installer for skills
written into `~/.claude`, which this is deliberately not).
