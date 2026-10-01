---
name: desktop-shipped-skills
description: Change the agent skills Hive Desktop ships into an agent workspace — the hive-flows, hive-actions, hive-settings, hive-webhook-sources, hive-mcp, hive-agent-workspaces SKILL.md files. Use when editing what one of those says or adding a new one.
compatibility: Requires Go and mise. Editing a template changes generated output — never edit a rendered SKILL.md inside a workspace directory.
---

# Change a shipped skill

A shipped skill is a **rendered prompt**, not a file. `cmd/desktop/internal/app/prompts`
owns the text (ADR go-owned-llm-prompts) and `cmd/desktop/internal/app/skills` wraps it in
the Agent Skills `SKILL.md` format. `app.SkillsService` composes the two: slug
is `hive-<prompt id>` and the frontmatter description is the prompt's
description plus `Use when configuring this in Hive Desktop (<target>).`

Nothing installs a skill globally. A workspace enables a **package**, the
package's glob patterns select names out of the shipped set plus
`.shared/skills/`, and `agentws.Generate` renders the selection into that
workspace's `.claude/skills/` and `.agents/skills/`
(ADR skills-are-declared-by-a-workspace, ADR skill-packages-are-the-unit-a-workspace-enables).

The `.agents/skills/` files in *this repo* are the project's own dev skills and
have nothing to do with these — do not confuse the two.

## Edit what a skill says

Prompt text lives in `cmd/desktop/internal/app/prompts/templates/*.tmpl`, never in a Go
string literal or a frontend component. Three rules decide which file to open:

- Wording shared by more than one prompt goes in `fragments.tmpl` and is
  composed with `{{template}}` — write a change once.
- What a prompt says about a **type** comes from that type's own documentation
  — `flow.NodeDoc` for node types, `actions.Doc` for action types,
  `mcpcatalog.Doc` for MCP entries — spliced in by the `section` function. Fix
  the type's doc; do not restate it in the template.
- Install-specific paths and URLs come from `.Env` (`FlowsDir`, `ActionsPath`,
  `SettingsPath`, `WebhookBaseURL`, `MCPEndpoint`, `AgentWorkspacesDir`, and
  the `*Enabled` flags), so a rendered skill names the real file on that
  machine instead of a placeholder.

## Add a skill

A template in `templates/` plus one entry in `definitions`
(`cmd/desktop/internal/app/prompts/prompts.go`) with `listed: true`. Nothing else changes:
`SkillsService.ShippedSkills` reports whatever the catalog holds, so the
seeded `hive` package's `hive-*` pattern picks the new slug up and every
workspace that enabled that package carries it on its next open — no manifest
edited anywhere.

`listed: false` marks a context-scoped prompt that needs instance data only its
own editor has (`webhook-transform`) — it is rendered on demand and never
offered as a skill.

`ValidateSkill` enforces the Agent Skills naming rules: the slug is 1–64 chars
of lowercase letters, digits, and single hyphens, must not contain a reserved
word, and the description caps at 1024 characters — which the `hive-` prefix
and the appended target sentence must fit inside.

## Validate

```bash
go test ./cmd/desktop/internal/app/...   # prompts, the SKILL.md renderer, and SkillsService
mise run check                   # the pre-push gate: generate, tidy, lint, test
```

`cmd/desktop/internal/app/prompts/prompts_test.go` is the real specification and asserts
against the registries, not fixed lists: every definition renders with no
unresolved template field, the flows and actions prompts cover every registered
node/action type, each prompt carries this install's paths, and rendering is
deterministic. Adding a node or action type without extending its doc fails
there. `TestEveryShippedSkillRenders` (`cmd/desktop/internal/app/skills_service_test.go`)
is the other half: a slug the catalogue advertises but `RenderSkill` refuses
would fail the open of every workspace whose package selects it.

## Guardrails

- **A rendered `SKILL.md` is generated output.** Editing one under a
  workspace's `.claude/skills/` is silently replaced on the next open
  (ADR workspace-directories-are-generated-and-disposable). Change the
  template.
- **Do not write install code paths for a new skill.** If a change needs a
  branch outside the prompt registry, the skill is being modelled wrong — it
  should be a template plus a registry entry.
- **A prompt that cannot render headlessly cannot ship.** `ShippedSkills`
  calls `Catalog(Input{})`, which silently omits a definition whose `data`
  errors, so such a prompt is simply absent from every workspace. That is what
  retired `hive-keybindings`, whose body needed the frontend's command
  catalog.
- A change to what these prompts say reaches a workspace on its next open.
  Keep them accurate about the current schema — the agent-workspaces prompt in
  particular, since it teaches the manifest shape.
