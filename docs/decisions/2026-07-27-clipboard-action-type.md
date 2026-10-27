# Clipboard action type with a render-only invocation path

- **Status:** accepted
- **Date:** 2026-07-27

## Context

"Copy a ready-to-paste command" needs its own action type: a `shell` action
piping to `pbcopy` is macOS-only and overloads a type meant to run a command.
A `clipboard` type fits the action extension point, except for invocation.
Every other detail-pane action runs through `Worker.Confirm`, which writes a
durable `output_command` row whose `UNIQUE (action_id, key)` index stops a run
action from firing again and makes a second confirm prompt for a rerun. A copy
must be repeatable with no prompt and no history, and writing the clipboard is
a GUI act the core cannot perform.

## Decision

**A clipboard action is detail-pane only, non-durable, and split across the
boundary: the core renders the text, the desktop writes the clipboard.**

- Config is `{ text_template }`, a Go `text/template` over the same data a
  shell action's `command_template` sees, with `shq`.
  `dispatch.RenderClipboardText` is the one renderer.
- `InboxService.RenderClipboardAction` is render-only. It runs the same checks
  as `InvokeAction` (action exists, is a clipboard action shown in the detail
  pane, applies to each item, item has an id), renders once per item, and
  joins the results. It never calls `Worker.Confirm`. The Wails
  `PipelineService.RenderClipboardAction` returns the text and the frontend
  writes it with the native Wails `Clipboard.SetText` (`navigator.clipboard`
  no-ops when the WKWebView is unfocused).
- `InvokeAction` refuses a clipboard action with `KindInvalid`, so a
  clipboard action never enqueues a command.
- `Action.HeadlessCapable()` is false for a clipboard action, so a flow
  `action` node cannot reference one.
- `ClipboardExecutor` still exists so every action type has an executor
  (`TestOutputExecutorsCoverEveryActionType`), though no path dispatches a
  clipboard action through the worker.

## Consequences

- Detail-pane actions have two invocation paths, `InvokeAction` (durable) and
  `RenderClipboardAction` (clipboard), and the frontend branches on type.
  Merging them means giving clipboard a durable row and its rerun prompt, or
  teaching the core to write the clipboard; both are rejected.
- A future "produce a value, no lasting side effect" action type follows this
  precedent instead of forcing an `output_command`.
