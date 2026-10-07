# Action runs keep a line log written through the execution context

- **Status:** accepted
- **Date:** 2026-10-07

## Context

An `output_command` row kept only the first 64 KiB of stdout and stderr, as two separate columns written when the attempt ended. That cannot show a running command, loses the order in which the two streams interleaved, and says nothing about actions that run no process, such as a session launch. Debugging a run, from the app or by an agent, needs the whole attempt in order, while it runs.

## Decision

Each attempt of an `actions.yml` action writes an ordered line log to `action_run_log`. Every row names its command, attempt, stream (`stdout`, `stderr`, or `system`), and time. The worker creates one `RunLog` per attempt, attaches it to the execution context, and flushes buffered lines in batches every 250 ms. Before the terminal status transition, it writes the outcome line and flushes, so a reader that sees a terminal status sees the complete log.

Executors write through `RunLogFrom(ctx)`. A shell command tees both streams into it and adds system lines for the command line and the exit status. An executor with no process, such as launch-session or publish-message, writes system lines that describe its steps. A nil `RunLog` discards everything, so an execution with no durable command needs no special case.

Only catalog actions get a log. Notify and launch nodes enqueue commands under ids that contain `:`, and those write no log and do not appear in the run list, so their volume cannot push action logs out of retention.

The log replaces the bounded `stdout` and `stderr` columns, which the migration copies into it and drops, so a run's output is written once. A log is kept whole, up to 8 MiB per attempt as a guard against a runaway command, and it lives exactly as long as its command row. Retention bounds finished `actions.yml` runs on their own, at `retention.action_runs` in `settings.yaml` (100 by default), apart from notify and launch node commands, so notification volume cannot remove run history.

## Consequences

The run viewer and the `list_action_runs` and `get_action_run` MCP tools read the same rows. Both follow a running command by polling from the last line id. The size of the log table is bounded by the run limit times the attempt cap, which is large in the worst case and small in practice; it trades disk for never cutting off the output that explains a failure.

Output is line-buffered, so output that ends in no newline appears when the attempt ends or after 16 KiB. Terminal-target actions stay non-durable and have no log.
