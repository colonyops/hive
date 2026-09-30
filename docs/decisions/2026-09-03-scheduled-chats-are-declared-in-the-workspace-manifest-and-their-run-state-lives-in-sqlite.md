# Scheduled chats are declared in the workspace manifest and their run state lives in SQLite

- **Status:** accepted
- **Date:** 2026-09-03

## Context

A workspace's agent should run recurring work on its own: every Friday at
09:00, start a chat in the "Product" workspace with a prompt that asks the
agent to summarize the week. A launch the app missed because it was closed
still has to happen the next time the app runs. Nothing like this exists:
there is no cron parser, no scheduler, and no way to start a workspace
session with an initial prompt. "Job" is taken (`internal/app/jobs` is live
action runs), so this is **schedules** for the definition and **runs** for
one execution.

## Decision

**The schedule is authored config; its run state is app-local data.** A
schedule is an entry under `schedules:` in `agent-workspace.yaml`, beside
`mcps:` and `skills:`: it is user- and agent-authored, travels with the
workspace's dotfiles, and the manifest watcher and the node-tree writer
already cover it. The editor writes it with the rest of the manifest, and the
write reconciles the list to exactly what the request carries, so there is no
half-saved manifest. How far each schedule has been evaluated and what its
runs did are not authored, so they live in `desktop-pipeline.db`, in
`schedule_cursor` and `schedule_run`.

**The scheduling logic is a leaf package, `internal/app/schedule`,** with
consumer-defined `Source`, `Store` and `Launcher` ports that `App` satisfies
over `agentws` and `data/stores`. One `Scheduler` goroutine runs on the App-owned
lifecycle, started after the agent-workspace watcher and stopped before
`terminals.Stop`.

**A cursor never lets a schedule fire for the past.** A schedule with no
cursor, or whose stored cursor was evaluated against a different `cron`, gets
its cursor set to now with no run. An edit to the timing must not surface a
pile of catch-up runs nobody asked for.

**A missed window is one run, not one per occurrence.** Every occurrence
since the cursor coalesces into a single run tagged `catch_up` that records
how many were folded in; `on_missed: skip` records a `skipped` run instead.
The cursor advances past the window either way.

**A run is skipped, not queued, while the previous one is still live,** and a
launch failure is recorded and never retried. The run history is where a user
finds out.

**The loop sleeps in bounded steps, capped at one minute,** because darwin
stops the monotonic clock while the machine sleeps, and a long timer would
fire late by the whole nap.

**The prompt is `text/template` with `missingkey=error`,** rendered against a
fixed data set (`Now`, `ScheduledFor`, `LastRun`, `Reason`, `Missed`, the
schedule's and the workspace's fields), so a typo fails the schedule instead
of leaking `<no value>` into the agent's opening message.

**A scheduled launch is an ordinary session launch, detached.** It
regenerates the workspace first, as `Open` does, and appends the rendered
prompt behind `--` so a prompt that opens with a hyphen is not read as an
option.

## Consequences

- A schedule never fires for a time before it, or its current `cron`,
  existed. A launch missed while the app was closed happens once, the next
  time it runs, unless `on_missed: skip`. An edit to `schedules:` takes
  effect without a restart.
- No automatic retry of a failed launch; local time only, since a cron
  expression carries no timezone; no second chat on top of one still running
  for the same schedule.
- `desktop-pipeline.db` gains two tables in `0008_schedules.up.sql`; their
  rows are app-local state, not part of the workspace folder.
