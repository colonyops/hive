# Hive Desktop observes shared hive.db state through one core poller with registered probes

- **Status:** accepted
- **Date:** 2026-10-02

## Context

The hive CLI and the agents it runs write sessions and hc tasks to the
`hive.db` the desktop reads. They run in other processes, and nothing in the
shared packages signals a write across processes: no triggers, no change
table, and the hive event bus is in-process. The desktop surfaced these writes
two ways: a core session poller, and a frontend timer in the Tasks store that
re-read the list only while the view was mounted.

A filesystem watch on `hive.db` cannot tell the tables apart. Every table
shares the file and its WAL, so a message or status write would wake every
consumer.

## Decision

1. **One core `hivewatch.Watcher` polls shared `hive.db` state.** It runs on
   one App-owned goroutine and ticker. A consumer registers a probe: a cheap
   read, an equality check, and an `OnChange` that publishes a core event. The
   watcher owns the baseline, failed-read, and lifecycle rules for every
   probe.

2. **A probe reads through the seam and publishes a typed event.** Sessions
   publish `SessionsUpdated` with the changed ids. Tasks publish
   `TasksUpdated`. `wailsui` degrades both to wake-ups.

3. **The tasks probe reads a fingerprint, not the items.** hc comments do not
   touch their item's `updated_at`, so an item diff misses them. The
   fingerprint is the row count and newest timestamp of `hc_items` and
   `hc_comments`, plus the `hc_task_blockers` row count. Replacing one blocker
   with another within one tick is not detected.

4. **The frontend does not poll shared `hive.db` state.** It reloads on the
   wake-up.

## Consequences

New shared `hive.db` state the desktop shows gets a probe, not a new loop.
Every probe runs every tick for the life of the app, so `Read` must stay
cheap. If idle cost matters later, a `PRAGMA data_version` check on a pinned
connection can gate the probes without changing them.
