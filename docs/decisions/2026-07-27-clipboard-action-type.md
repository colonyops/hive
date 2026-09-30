# Clipboard action type with a render-only invocation path

- **Status:** accepted
- **Date:** 2026-07-27

## Context

"Copy a ready-to-paste command" was faked as a `shell` action piping to
`pbcopy` (the seeded `copy-checkout`). That is macOS-only, invisible to the
action editor, and overloads the shell type — which exists to run a local
command — for what is really a distinct intent: render text and put it on the
clipboard.

A `clipboard` action type is the fix, and it fits the existing action
extension point (registry line, config + `Validate`, `actions/docs/<type>.md`,
editable-catalog and YAML-writer branches, an `Executor`). The one part that
does not fit is *invocation*. Every other detail-pane action runs through
`Worker.Confirm`, which writes a durable `output_command` row whose
`UNIQUE (action_id, key)` index is load-bearing: it is what stops an already-run
action from re-firing, and a second confirm of the same row returns
`ConfirmationRequired` so the frontend prompts for a rerun.

That behaviour is exactly wrong for a clipboard action. Copying the same item
twice must just re-copy, with no prompt and no accumulating command history — a
clipboard action has no side effect the app must remember. And copying is a GUI
act: the text lands on the clipboard through the native Wails `Clipboard.SetText`
(chosen over `navigator.clipboard`, which no-ops when the WKWebView is
unfocused), which the core cannot and should not perform.

## Decision

**A clipboard action is detail-pane only, non-durable, and split across the
boundary: the core renders the text, the desktop adapter writes the clipboard.**

- **Config is `{ text_template }`** — a Go `text/template` rendered over the
  same `OutputData` context a shell action's `command_template` renders over,
  with the `shq` helper available. `dispatch.RenderClipboardText` is the single
  renderer; `ClipboardExecutor` and the render-only service method both call it,
  so the copied text can never drift from what a dispatch would produce.
- **`RenderClipboardAction` is a render-only service method**, not a durable
  dispatch. `InboxService.RenderClipboardAction` runs the same authorization
  chain as `InvokeAction` (item decodes, action exists, is a clipboard action
  shown in the detail pane, applies to the item's kind, item has an id) and
  returns the rendered text. It never touches `Worker.Confirm`, so no
  `output_command` row exists and re-copying is free of any rerun prompt. The
  Wails `PipelineService.RenderClipboardAction` returns the text; the frontend
  writes it via the existing `useClipboard` path and shows a "Copied" toast.
- **`InvokeAction` refuses a clipboard action.** The durable path returns a
  typed `KindInvalid` error for a clipboard type, so a clipboard action can
  never enqueue a command even if a caller bypasses the render path.
- **A clipboard action is never `HeadlessCapable`.** A headless flow `action`
  node has no clipboard target, so `Action.HeadlessCapable()` returns false for
  a clipboard config and `flow`'s action-node validator rejects a reference to
  one — the same gate that already keeps interactive launch-session actions out
  of flows.
- **`ClipboardExecutor` still exists** and renders the text into a
  `ClipboardExecutionOutcome`. A clipboard action is never dispatched through
  the worker in practice (detail-pane only, non-headless, and `InvokeAction`
  refuses it), but the executor keeps the dispatch contract complete — every
  catalog action type has one, enforced by
  `TestOutputExecutorsCoverEveryActionType` — and would render identically were
  a durable path ever to enqueue it.

## Consequences

- The clipboard/durable split is now explicit: durability lives in
  `Worker.Confirm`, and the render-only path deliberately routes around it. A
  future action type that is likewise "produce a value, no lasting side effect"
  has a worked precedent to follow rather than forcing an `output_command`.
- The seeded `copy-checkout` becomes a real `clipboard` action; a `shell`
  starter (`open-on-github`, `gh browse`) keeps the shell type demonstrated, as
  the starter-catalog test requires one example per registered type.
- Two invocation paths for detail-pane actions now exist — `InvokeAction` for
  durable actions and `RenderClipboardAction` for clipboard — and the frontend
  branches on the action type. Reversing this to a single path means either
  giving clipboard a durable row (and its rerun prompt) or teaching the core to
  write the clipboard, both of which this decision rejects.
