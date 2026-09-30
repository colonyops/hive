# Actions declare their inputs on the envelope, collected by one generic form

- **Status:** accepted
- **Date:** 2026-07-30

## Context

Action templates render over `dispatch.OutputData` — `.Key`, `.Payload`,
`.Raw` — every field of which comes from the triggering item. There was no way
for an action to ask the user for anything, so "silence this alert *for a
reason*" could not be expressed: the reason exists only at click time.

One action type already collected input before running. An interactive
`launch-session` action (no `repo_template`) short-circuits invocation to a
dedicated dialog, and its values travel on `ActionInvocationInput.Session`.
That is a bespoke path for one type's fields, not a mechanism.

## Decision

**An action declares its inputs on the envelope, and one generic form collects
them for every action type.**

- **Envelope, not per-type config.** `inputs` sits beside `id`/`label`/`type`
  in `actions.yml` because every action type renders over the same
  `OutputData`. A new action type inherits the invocation form without wiring
  anything — unlike the per-type extension-point checklist (registry, executor,
  writer branch, editable branch), inputs cost a new type nothing.
- **Values are strings.** `{name, label, type, required, default, placeholder,
  options}` where `type` selects a control — `text`, `multiline`, `select` —
  not a value type. Templates interpolate text; a typed value would have to be
  rendered back to a string at the only point it is used.
- **`name` is a template identifier**, not the kebab-case slug ids use, because
  it is read as `{{ .Inputs.<name> }}` and Go's parser accepts only an
  identifier after a field selector.
- **`Action.ResolveInputs` is the single gate.** It fills blanks from defaults,
  rejects a blank required value, rejects a select value outside its options,
  and rejects a name the catalog does not declare. `InboxService.InvokeAction`
  runs it as a preflight so an incomplete form never becomes a durable failed
  command, and `Worker.execute` runs it again as the thing that actually
  produces `OutputData.Inputs` — which is what makes the automatic path work,
  since a flow-fired command carries no collected values and needs its defaults
  filled in.
- **A required input with no default makes an action detail-pane only.**
  `HeadlessCapable()` returns false, so `flow`'s action-node validator refuses
  the reference — the same gate that already keeps interactive launch-session
  actions and clipboard actions out of flows. The alternative, running
  headlessly with defaults, would let a required value be silently absent.
- **Declared inputs compose with the session dialog** rather than stacking a
  second modal: an interactive `launch-session` action renders its declared
  inputs inside `CreateSessionDialog`, and both halves reach the invocation.
  `session` stays its own field on `ActionInvocationInput` — it is the launcher's
  contract (repository, session name, agent), not free-form template data.

## Consequences

- Collected values are not persisted with the `output_command` row. A rerun
  re-sends what the dialog holds, matching how `session` input already behaves;
  a retried automatic command re-resolves from defaults. Persisting them would
  make the durable command a second source of truth for values the catalog can
  redeclare between runs.
- `.Inputs.<name>` for a name the action does not declare is a render error,
  not a blank — the renderer runs with `missingkey=error` and resolution only
  ever populates declared names. Referencing an input you forgot to declare
  fails loudly.
- `RenderClipboardText` and `RenderRepoTarget` both take the resolved map, and
  `Action.ResolveInputs` is the only producer of that map: the detail pane's
  applicability probe resolves with nothing supplied, so it renders
  `repo_template` over declared defaults — the same values a headless run
  resolves to — rather than over blanks.
- Adding an input to an action a flow references is refused when it turns the
  action interactive, through the existing headless-to-interactive check in
  `ActionStore.Update`.
