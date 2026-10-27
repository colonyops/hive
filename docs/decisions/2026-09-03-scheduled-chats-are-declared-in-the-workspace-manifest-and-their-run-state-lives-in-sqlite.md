# Scheduled chats are declared in the workspace manifest and their run state lives in SQLite

- **Status:** accepted
- **Date:** 2026-09-03

## Context

A workspace's agent should run recurring work on its own, such as a chat
every Friday at 09:00 that summarizes the week. A launch missed while the app
was closed still has to happen the next time it runs. "Job" already names
live action runs (`cmd/desktop/internal/app/jobs`), so the definition is a
**schedule** and one execution is a **run**.

## Decision

**The schedule is authored config; its run state is app-local data.** A
schedule is an entry under `schedules:` in `agent-workspace.yaml`, beside
`mcps:` and `skills:`, so it travels with the workspace and the manifest
watcher covers it. A manifest write reconciles the list to exactly what the
request carries. How far each schedule has been evaluated and what its runs
did live in `desktop-pipeline.db`, in `schedule_cursor` and `schedule_run`.

**The scheduling logic is a leaf package, `cmd/desktop/internal/app/schedule`,**
with consumer-defined `Source`, `Store` and `Launcher` ports that `App`
satisfies (`schedule_adapters.go`). One `Scheduler` goroutine runs on the
App-owned lifecycle.

**A cursor never lets a schedule fire for the past.** A schedule with no
cursor, or whose cursor was evaluated against a different `cron`, gets its
cursor set to now with no run, so an edit to the timing never surfaces
catch-up runs.

**A missed window is one run, not one per occurrence.** Every occurrence
since the cursor coalesces into one run with reason `catch_up` that records
how many it folded in; `on_missed: skip` records a skipped run instead. The
cursor advances past the window either way.

**A run is skipped, not queued, while the previous one is still live,** and
a launch failure is recorded and never retried. The run history is where a
user finds out.

**The loop sleeps in steps of at most one minute** (`maxSleep`), because
darwin stops the monotonic clock while the machine sleeps and a long timer
would fire late by the whole nap.

**The prompt is `text/template` with `missingkey=error`,** rendered against a
fixed data set (`Now`, `ScheduledFor`, `LastRun`, `Reason`, `Missed`, and the
schedule's and workspace's fields), so a typo fails the schedule instead of
leaking `<no value>` into the agent's opening message.

**A scheduled launch is an ordinary session launch, detached.** It
regenerates the workspace first, as `Open` does, and passes the prompt
through the workspace command template (ADR the-workspace-command-is-a-template).
A workspace whose command does not take a prompt cannot declare schedules.

## Consequences

- An edit to `schedules:` takes effect without a restart.
- Times are local: a cron expression carries no timezone.
- Cursor and run rows are app-local state, not part of the workspace folder,
  so a synced workspace on a second machine starts its own cursors.
