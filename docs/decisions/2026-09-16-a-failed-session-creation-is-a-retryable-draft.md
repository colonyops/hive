# A failed session creation is a retryable draft

- **Status:** accepted
- **Date:** 2026-09-16

## Context

`CreateSession` validates the New Session form and hands the rest to a
background job, because a clone takes as long as it takes. The form closes
on submit, so a failure minutes later has nowhere to land but a job row, and
what was typed is lost. A failure also has to say which step failed, where
the checkout is, and what the child process printed; a bare
`exit status 1` from a failing git hook is not enough to act on.

## Decision

1. **The last failed attempt is a `dispatch.SessionDraft` the form
   restores**, in one in-memory slot on `SessionsService`. A non-nil
   `Failure` is what says an attempt is waiting. A duplicate name is not a
   failure: the form is still open and the field is what is wrong.

2. **The durable half is an activity row.** A failure records a session
   error event (`activity.SessionCreateFailed`) whose metadata carries the
   submitted form, and the Activity view offers Retry on any row that
   carries it. The slot dies with the process; the row survives a restart,
   which matters when the fix is to repair a git hook first. The metadata
   column already exists, so this needs no migration. A retry lives exactly
   as long as its audit row, and a prompt is written into the activity log;
   both are accepted. The frontend forwards the metadata untouched, so the
   encoding has one owner (`SessionDraftMetadata` /
   `SessionDraftFromMetadata`).

3. **The failure detail comes from the engine's typed errors, not from text.**
   `session.Service.CreateFromRequest` returns a `*session.LaunchError`
   carrying the step, destination, clone strategy, whether a leftover
   checkout is on disk, and the progress tail. `CreateSessionError.Operation`
   is the authority on the failed step; the last progress line is the
   fallback for a failure before a destination was resolved. Child output
   travels in `executil.CommandError`. Nothing matches on error text.

4. **The failure is pushed, not filed.** `events.SessionCreateFailed` wakes
   the frontend, which re-reads the draft and raises an error toast that
   does not expire.

## Consequences

- ⌘N is the retry: `openBlank` restores the pending attempt ahead of the
  current repository and the default, so there is no separate command.
- One structured ERR log line per failure carries every field, present even
  when empty, so log queries can rely on its shape.
