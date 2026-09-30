# LLM prompts owned by Go, node docs live with the schema

- **Status:** accepted
- **Date:** 2026-07-24

## Context

Hive Desktop's configuration — `flows/*.yaml`, `actions.yml`, `settings.yaml` — is plain text meant to be written by a coding agent rather than by hand, so the app ships paste-ready prompts. Those prompts had grown wherever they were first needed: `pipeline/lib/flowPrompt.ts` built the flows prompt from the frontend node registry (with a `WORKED_EXAMPLE` hand-transcribed from `flow`'s loader test), `nodes/webhook-source/prompt.ts` built a transform prompt, and each was reachable only from the surface that happened to own it. There was no place that answered "what can I hand to an agent to configure this app?", nothing shared a preamble, and the hand-copied example had already drifted from the fixture it was copied from (`msg.payload` vs `msg.Payload`).

The split of ownership was the real question. The schemas being described are defined in Go (`flow`'s and `actions`' type registries), and only Go knows this install's real config paths and webhook port. But per-node-type prose lived in the frontend as `nodes/*/help.md`, which the node drawer and palette render.

## Decision

`internal/app/prompts` owns all prompt text and assembly: `//go:embed templates/*.tmpl` plus `text/template`, with shared fragments (`app`, `yaml-strict`, `template-data`, `task`) composed by each prompt so common wording is written once. Prompts render against an `Env` of the install's real paths and webhook URL, exposed over `PromptsService`. Adding a prompt is a template plus a registry entry; the settings page renders whatever the registry reports.

Per-type documentation moved to the Go tree next to the schema that validates it — `pipeline/flow/docs/<type>.md` and `pipeline/actions/docs/<type>.md` — and the frontend imports the *same files* through a `@nodedocs` Vite alias rather than keeping a second copy. Tests assert a registry↔docs bijection, so adding a node or action type extends the prompt with no prose edit anywhere.

Worked examples are embedded fixtures (`flow.WorkedExampleYAML`, `actions.ExampleYAML()`) that the loader tests parse and validate, so a prompt can never show an example that no longer loads.

The one fact the frontend still owns is the bindable command catalog (`keybindings/catalog.ts` carries icons and palette grouping); it is passed into the keybindings prompt as render input. To make that prompt actionable, keybinding overrides moved from webview `localStorage` to a `keybindings` section in `settings.yaml`, stored opaquely by Go for the same reason `Appearance.Theme` is.

## Consequences

- One place to change prompt wording, and one place to add a prompt. No component builds prompt strings.
- Prompts name this machine's actual paths and port instead of placeholders, which is only possible server-side.
- The frontend build reaches outside its root for node docs, so `vite.config.ts` and `vitest.config.ts` both carry the `@nodedocs` alias and a matching `server.fs.allow` entry. A third config that resolves frontend modules would need the same.
- Node docs are now read by both a markdown renderer in the drawer and an LLM, so they must stay free of UI-only references ("the row below").
- Keybindings are dotfiles-managable and agent-editable, but a rebind now depends on a settings write; a failed write is logged and left live for the session rather than reverted.
