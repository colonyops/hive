# Actions declare their inputs on the envelope, collected by one generic form

- **Status:** accepted
- **Date:** 2026-07-30

## Context

Action templates render over `dispatch.OutputData`, and every field of it came
from the triggering item. An action could not ask the user for anything, so
"silence this alert for a reason" could not be expressed: the reason exists
only at click time.

## Decision

**An action declares its inputs on the envelope, and one generic form collects
them for every action type.**

- **Envelope, not per-type config.** `inputs` sits beside `id`, `label` and
  `type` in `actions.yml`, because every type renders over the same
  `OutputData`. A new action type gets the form with no wiring.
- **Values are strings.** An `InputSpec` is `{name, label, type, required,
  default, placeholder, options}`, where `type` (`text`, `multiline`,
  `select`) picks a control, not a value type. Templates interpolate text.
- **`name` is a template identifier**, read as `{{ .Inputs.<name> }}`, not a
  kebab-case slug, because Go's parser accepts only an identifier after a
  field selector.
- **`Action.ResolveInputs` is the single gate.** It fills blanks from
  defaults and rejects a blank required value, a select value outside its
  options, and an undeclared name. `InboxService.InvokeAction` runs it as a
  preflight so an incomplete form never becomes a durable failed command, and
  `Worker.execute` runs it again to produce `OutputData.Inputs`, which fills
  defaults for a flow-fired command that carries no collected values.
- **A required input with no default makes an action detail-pane only.**
  `HeadlessCapable()` returns false, so a flow action node cannot reference
  it. Running headlessly would let a required value be silently absent.
- **Declared inputs compose with the session dialog.** An interactive
  `launch-session` action renders its inputs inside `SessionLaunchDialog.vue`
  rather than stacking a second modal. `ActionInvocationInput.Session` stays
  its own field: it is the launcher's contract (repository, session name,
  agent), not template data.

## Consequences

- Collected values are not persisted on the `output_command` row. A rerun
  re-sends what the dialog holds, and a retried automatic command re-resolves
  from defaults.
- `.Inputs.<name>` for an undeclared name is a render error, not a blank
  (`pkg/tmpl` renders with `missingkey=error`).
- `ResolveInputs` is the only producer of the resolved map that
  `RenderClipboardText` and `RenderRepoTarget` take. The detail pane's
  applicability probe resolves with nothing supplied, so it renders over
  declared defaults, the same values a headless run gets.
- `ActionStore.Update` refuses an edit that turns an action a flow references
  from headless-capable into interactive-only.
