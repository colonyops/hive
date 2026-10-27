# Hive Desktop observes shared hive.db state through one core poller with registered probes

- **Status:** accepted
- **Date:** 2026-10-02

## Context

The hive CLI and the agents it runs write sessions and hc tasks to the
`hive.db` the desktop reads, from other processes. Nothing signals such a
write across processes: no triggers, no change table, and the hive event bus
is in-process. A filesystem watch on `hive.db` cannot tell tables apart,
because every table shares the file and its WAL.

## Decision

1. **One core `hivewatch.Watcher` polls shared `hive.db` state** on one
   App-owned goroutine and ticker (`hiveWatchInterval`, 2s). A consumer
   registers a probe: a cheap `Read`, an `Equal`, and an `OnChange` that
   publishes a core event. The watcher owns the baseline, failed-read, and
   lifecycle rules for every probe. The probes live in `hive_probes.go`.

2. **A probe publishes a typed event.** The sessions probe publishes
   `SessionsUpdated` with the changed ids; the tasks probe publishes
   `TasksUpdated`. `wailsui` degrades both to wake-ups.

3. **The tasks probe reads a fingerprint, not the items** (`HCFingerprint`).
   hc comments do not touch their item's `updated_at`, so an item diff misses
   them. The fingerprint is the row count and newest timestamp of `hc_items`
   and `hc_comments`, plus the `hc_task_blockers` row count. Replacing one
   blocker with another within one tick is not detected.

4. **The frontend does not poll shared `hive.db` state.** It reloads on the
   wake-up.

## Consequences

New shared `hive.db` state the desktop shows gets a probe, not a new loop.
Every probe runs every tick for the life of the app, so `Read` must stay
cheap. If idle cost matters, a `PRAGMA data_version` check on a pinned
connection can gate the probes without changing them.
