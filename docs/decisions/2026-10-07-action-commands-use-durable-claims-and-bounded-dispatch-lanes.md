# Action commands use durable claims and bounded dispatch lanes

- **Status:** accepted
- **Date:** 2026-10-07

## Context

An item action held the output worker's scheduling mutex until its executor returned. A shell command that stayed alive for an interactive review therefore blocked later manual invocations and the automatic action queue. Removing the mutex alone would let two schedulers execute the same pending row because selection did not claim it atomically.

A successful process start is not successful action completion. Shell actions must remain tracked until exit, retain bounded diagnostics, and stop with the app. Manual inputs intentionally remain process-local, so an interrupted manual run must not replay after restart.

## Decision

The App owns one dispatch coordinator with separate bounded capacity for manual and automatic item actions. A manual invocation returns its command id and `running` state after admission; execution continues under App-owned supervision. Reaching the manual limit rejects the invocation before it creates a command. Automatic work remains pending when its lane is full.

Every attempt atomically changes one `output_command` from `pending` to `running`, increments its attempt count, and records a unique claim token. Completion, failure, cancellation, and requeue updates require both the running state and that token. The existing non-rerun deduplication index remains, and a second partial unique index prevents concurrent active reruns for the same action and key.

Manual and attempted side-effect failures are terminal. An automatic failure before a side effect can return to `pending` with a bounded delay and attempt limit. Startup marks residual `running` commands failed rather than replaying them. App shutdown stops admission, cancels active work, waits within a deadline, and only then closes SQLite. Shell cancellation targets the owned process group.

## Consequences

Long-running reviews no longer serialize unrelated item actions. The jobs and action-run UI must treat `pending` and `running` as active states, refresh terminal results from job events, and offer cancellation.

Completion order can differ from claim order. A full lane still limits progress, but manual and automatic work cannot consume each other's reserved capacity. SQLite claims prevent duplicate local ownership, not exactly-once external side effects after a process crash. Terminal-target actions remain on their separate repeatable, non-durable path.
