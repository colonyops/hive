# A failed operation the user must act on raises a shared error dialog, not an inline message

- **Status:** accepted
- **Date:** 2026-08-07

## Context

Deploy wrote its failure to a 240px truncated label in the flows canvas status
strip. The strip also carries the file path and the node status counts, so the
one state that means "nothing shipped" looked like the states that mean
"everything is fine" — an author reads `Deploy`, sees the canvas unchanged, and
walks away believing the flow is running. The text was truncated, unselectable
in practice, and gone as soon as the next load cleared `error`.

Toasts do not fix this: they self-dismiss on a timer, they are the wrong size
for a Go error chain, and `useToasts` is deliberately app-shell state the flows
canvas does not reach (FlowsView says so in its own module docs).

Nothing shared existed for "an operation failed and the user needs the
details", so every surface that wanted more than a red line would have grown
its own.

## Decision

1. **A failure the user must act on is raised, not rendered in place.** A
   caller passes `ErrorDetails` to `useErrorDialog().showError(...)` — title,
   one-line summary, the error text verbatim, and identifiers worth carrying
   into a report. The state is module-scoped, so a surface raises without
   knowing where the dialog is mounted; App.vue mounts the single
   `ErrorDialog` that renders it.

2. **The detail is never trimmed to fit.** It is shown selectable, scrolls
   inside the dialog, and is what Copy writes and a report carries. Callers
   read it with `errorText`, which prefers the message the Go core wrote over
   the one the Wails runtime threw.

3. **Reporting is one click and attaches everything the manual reporter
   offers** — logs, settings, flows, actions, all redacted Go-side (ADR
   [in-app-problem-reporting](2026-07-27-in-app-problem-reporting.md)). Opening
   the reporter's form pre-filled was rejected: the person who just lost a
   deploy did not set out to file a bug, and a form is where that intent dies.
   The dialog states what a report attaches, and the button disappears into an
   explanation in a build with no report endpoint.

4. **The backdrop does not dismiss it.** Escape and Close do. A failure the app
   decided to interrupt for should not close on a stray click before it has
   been read.

Deploy is the first caller. Inline error text stays where a surface already has
it — it is the record, not the interrupt.

## Consequences

- `BaseModal` closes on any Escape with no stacking discipline, so two overlays
  would both consume one keypress. App.vue therefore renders
  `UnsavedFlowChangesModal` only while no error is raised: deploying from that
  modal replaces it with the error and dismissing brings it back. A second
  caller that can raise from inside its own modal needs the same treatment, or
  `useEscapeToClose` needs a real modal stack.
- A caller that raises for something the user cannot act on turns a shrug into
  a modal. The bar is "this could be mistaken for success", not "this is an
  error" — a background poll that fails and retries stays a log line.
- `showError` is fire-and-forget with no queue: a second failure replaces the
  first. Two independent failures at once has not happened, and a queue would
  have to answer which one Escape dismisses.
