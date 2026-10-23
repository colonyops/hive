# LLM prompts owned by Go, node docs live with the schema

- **Status:** accepted
- **Date:** 2026-07-24

## Context

Hive Desktop's configuration (`flows/*.yaml`, `actions.yml`, `settings.yaml`) is meant to be written by a
coding agent, so the app ships paste-ready prompts. The schemas those prompts describe are defined in Go,
and only Go knows this install's real config paths and webhook URL. Prompts built ad hoc in the frontend
duplicated wording and drifted from the fixtures they copied.

## Decision

`cmd/desktop/internal/app/prompts` owns all prompt text and assembly: `//go:embed templates/*.tmpl` with
`text/template`, and shared fragments (`app`, `yaml-strict`, `template-data`, `task`) composed by each
prompt. Prompts render against an `Env` of the install's real paths and endpoints, exposed through
`PromptsService`. Adding a prompt is a template plus a registry entry; the settings page renders whatever
the registry reports.

Per-type documentation lives next to the schema that validates it (`flow/docs/<type>.md`,
`actions/docs/<type>.md`), embedded in Go. The frontend imports the same flow node docs through the
`@nodedocs` Vite alias instead of keeping a copy. Tests assert every registered type has a doc.

Worked examples are embedded fixtures (`flow.WorkedExampleYAML`, `actions.ExampleYAML()`) that the loader
tests parse, so a prompt cannot show an example that no longer loads.

Keybinding overrides live in the `keybindings` section of `settings.yaml`, so an agent can edit them.

## Consequences

- No component builds prompt strings.
- The frontend build reaches outside its root for node docs, so `vite.config.ts` and `vitest.config.ts`
  both carry the `@nodedocs` alias, and `vite.config.ts` a matching `server.fs.allow` entry. Any other
  config that resolves frontend modules needs the same.
- Node docs are read by both the drawer's markdown renderer and an LLM, so they must stay free of UI-only
  references ("the row below").
