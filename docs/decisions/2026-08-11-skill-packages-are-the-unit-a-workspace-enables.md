# Skill packages are the unit a workspace enables

- **Status:** accepted
- **Date:** 2026-08-11

## Context

A workspace declares the MCP servers and the skills its agent can use.
`mcps.yaml` is a library and a workspace's `mcps:` list enables entries from
it. Skills need the same scoping, but enabling individual skills gives a list
that grows without bound and makes every new skill a manifest edit in every
workspace that wants it.

## Decision

`skills.yml` sits beside `mcps.yaml` at the workspace root and defines
**packages**: a named set of glob patterns over skill names. A workspace's
`skills:` list names packages, never individual skills, and generation installs
the union of what the enabled packages select.

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

- **Patterns, not membership lists.** One skill can belong to several packages,
  and a new skill joins every workspace whose package already matches its name.
  `include` admits, `exclude` carves out, and a pattern with no wildcard is an
  exact name.
- **One name-space, two sources:** the skills this build ships and
  `.shared/skills/<name>/SKILL.md` (`agentws.SkillNames`). A shared skill with a
  shipped skill's name wins, since the authored copy is the one the user can
  see and edit. The shipped half arrives as data (`ShippedSkill`) because
  `cmd/desktop/internal/app/prompts` imports `agentws`.
- **The union needs no collision rule.** Identity is the name, so a skill two
  packages select is installed once.
- **The generator stays pure.** `agentws.Generate` expands no pattern and reads
  no directory; the service resolves packages and passes
  `GenerateInput.Skills`.
- **An undefined package is reported, not fatal.** It lands in
  `OpenResult.MissingPackages` and the workspace still opens, like a missing
  MCP id. A package that selects nothing lists with no members.
- **Validation is at load.** A package with no `include`, a pattern `path.Match`
  cannot compile, or a name that is not a path-safe segment fails the file. A
  broken `skills.yml` keeps the last-good set.

Authoring stays file work: a skill is a `SKILL.md` that every target agent
already reads, installed verbatim.

## Consequences

- The seeded `hive` workspace declares `skills: [hive]`, and the seeded
  `skills.yml` defines `hive` as `hive-*`, so a release that adds or removes a
  shipped skill needs no startup rewrite.
- A manifest that still lists skill slugs reports them as missing packages.
  No migration rewrites it.
- `mcps.yaml` has no equivalent of a pattern. If server sets ever want one,
  this is the precedent.
- Packages do not cover distribution (installing a package from a source, or
  updating one). That is a separate decision.
- A workspace is the only place skills are installed
  (ADR skills-are-declared-by-a-workspace).
