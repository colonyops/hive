# Action runs keep a line log written through the execution context

- **Status:** accepted
- **Date:** 2026-10-07

## Context

Debugging an action run, from the app or by an agent, needs the whole attempt
in order, while it runs: both streams interleaved as they happened, and the
steps of actions that run no process, such as a session launch.

## Decision

Each attempt of an `actions.yml` action writes an ordered line log to
`action_run_log`. Every row names its command, attempt, stream (`stdout`,
`stderr`, or `system`), and time. The worker creates one `RunLog` per attempt,
attaches it to the execution context, and flushes buffered lines in batches
every 250 ms. Before the terminal status transition it writes the outcome line
and flushes, so a reader that sees a terminal status sees the complete log.

Executors write through `RunLogFrom(ctx)`. A shell command tees both streams
into it and adds system lines for the command line and the exit status. An
executor with no process, such as launch-session or publish-message, writes
system lines that describe its steps. A nil `RunLog` discards everything, so
an execution with no durable command needs no special case.

Only catalog actions get a log. Notify and launch nodes enqueue commands under
ids that contain `:`; those write no log and do not appear in the run list.

The log is the only record of a run's output, kept whole up to 8 MiB per
attempt, and it lives exactly as long as its command row. Retention bounds
finished `actions.yml` runs on their own, at `retention.action_runs` in
`settings.yaml` (100 by default), apart from node commands, so notification
volume cannot remove run history.

## Consequences

The run viewer and the `list_action_runs` and `get_action_run` MCP tools read
the same rows, and follow a running command by polling from the last line id.
Disk use is bounded by the run limit times the per-attempt cap.

Output is line-buffered, so output that ends in no newline appears when the
attempt ends or after 16 KiB. Terminal-target actions stay non-durable and have
no log.
