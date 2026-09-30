# Clipboard

A **clipboard** action renders a template over whatever it was invoked against
and puts the result on the clipboard. Use it for "get me a ready-to-paste
command" — a `gh pr checkout`, a session's checkout path, a branch name, a
link — without shelling out to `pbcopy`.

## Fields

- `text_template` (required) — the text placed on the clipboard.

## Templates

`text_template` is a Go `text/template` rendered over the target's data (see
"Template data" above), the same context a shell action's `command_template`
renders over. The `shq` helper is available, though clipboard text is not run
through a shell, so quoting is rarely needed:

```
text_template: "gh pr checkout {{ .Payload.num }} -R {{ .Payload.repo }}"
```

## Copied, never run

A clipboard action is offered wherever it declares a target, but it is not
runnable from a flow, which has no clipboard to write to. It leaves no durable
command record either: copying again just re-renders, with no rerun prompt.
