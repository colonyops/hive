# A failed operation the user must act on raises a shared error dialog, not an inline message

- **Status:** accepted
- **Date:** 2026-08-07

## Context

A flow deploy failure was a truncated label in the canvas status strip, next to
the file path and node counts. The one state that means "nothing shipped"
looked like the states that mean "all fine", and the author walked away
believing the flow ran. Toasts do not fix this: they self-dismiss, are the wrong
size for a Go error chain, and are app-shell state the flows canvas does not
reach. Nothing shared existed for "an operation failed and the user needs the
details".

## Decision

1. **A failure the user must act on is raised, not rendered in place.** A
   caller passes `ErrorDetails` (title, one-line summary, the error text
   verbatim, identifiers worth reporting) to `useErrorDialog().showError(...)`.
   The state is module-scoped; `AppDialogs.vue` mounts the single
   `ErrorDialog`.
2. **The detail is never trimmed.** It is selectable, scrolls inside the
   dialog, and is what Copy writes. Callers read it with `errorText`
   (`@hive/ui`), which prefers the Go core's message over the Wails runtime's.
3. **Report a problem is one click** and opens the issue flow
   (ADR problem-reports-are-github-issues). The error text is not prefilled,
   because it names flows, nodes, and repositories; the user pastes it.
4. **The backdrop does not dismiss it.** Escape and Close do.

Flow deploy (`usePipelineEditor`) is the first caller. Inline error text stays
where a surface already has it, as the record rather than the interrupt.

## Consequences

- The bar is "this could be mistaken for success", not "this is an error". A
  background poll that fails and retries stays a log line.
- `showError` has no queue: a second failure replaces the first.
- `AppDialogs.vue` renders `UnsavedFlowChangesModal` only while no error is
  raised, so deploying from that modal swaps it for the error and dismissing
  brings it back. Another caller that raises from inside its own modal needs
  the same treatment.
