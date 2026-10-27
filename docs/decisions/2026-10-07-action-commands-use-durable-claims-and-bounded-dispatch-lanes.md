# Action commands use durable claims and bounded dispatch lanes

- **Status:** accepted
- **Date:** 2026-10-07

## Context

A shell action can stay alive for a long time, for example during an
interactive review. It must not block later manual invocations or the
automatic action queue, and two schedulers must never execute the same
pending row. A successful process start is not successful completion: a shell
action stays tracked until exit and stops with the app. Manual inputs are
process-local, so an interrupted manual run must not replay after restart.

## Decision

The dispatch worker has separate bounded capacity for manual and automatic
item actions. A manual invocation returns its command id and `running` state
after admission, and execution continues under the worker's supervision. A
full manual lane rejects the invocation before it creates a command. Automatic
work stays `pending` while its lane is full.

Every attempt atomically changes one `output_command` from `pending` to
`running`, increments its attempt count, and records a unique `claim_token`.
Completion, failure, cancellation, and requeue updates require both the
running state and that token. A partial unique index on `(action_id, key)`
over `pending` and `running` rows prevents concurrent active reruns of the
same action and key.

Manual failures and failures after an attempted side effect are terminal. An
automatic failure before a side effect returns to `pending` with a delay and
an attempt limit. Startup marks residual `running` commands failed instead of
replaying them. Shutdown stops admission, cancels active work, waits within a
deadline, and only then closes SQLite. Shell cancellation targets the owned
process group.

## Consequences

The jobs and action-run UI treat `pending` and `running` as active states,
refresh results from job events, and offer cancellation.

Completion order can differ from claim order. Manual and automatic work cannot
consume each other's capacity. SQLite claims prevent duplicate local
ownership, not exactly-once external side effects after a crash.
Terminal-target actions stay on their separate repeatable, non-durable path.
