# hive.log rotates by size under a file lock shared by every hive process

- **Status:** accepted
- **Date:** 2026-10-10

## Context

`hive.log` had no size cap. Every `hive` CLI invocation, the TUI, agents
calling `hive hc`, and Hive Desktop append to the same file at the same time,
and the desktop holds it open for days. A single-process rotator such as
lumberjack counts its own writes and renames the file under the others, so
the processes that still hold the old file keep writing to a backup.

## Decision

`logutils.NewRoot` writes the file through a rotating writer:

1. **Size is read from disk, not counted.** A process checks when it opens
   the log, then on the first write after five minutes. It stats the path and
   its open file; when they are the same file and the file has reached
   10 MiB, it rotates. Five minutes is enough: hive writes on the order of
   150 KB a day, and a late rotation costs only some overshoot.
2. **Rotation holds an exclusive `flock` on `hive.log.lock`** and re-checks
   the size under it, so concurrent writers rotate once. It renames
   `hive.log` to `hive.log.1` and shifts older backups up, keeping three.
3. **A writer whose path no longer names its open file reopens it.** That is
   how a long-running process follows a rotation another process did.

## Consequences

- Retained log is about 40 MiB. A file can overshoot 10 MiB by what is
  written in five minutes.
- A long-running writer keeps appending to a rotated backup for up to five
  minutes after another process rotates. Short-lived CLI processes check only
  on open.
- Diagnostics and the problem report read only `hive.log`, so right after a
  rotation they show little history.
- Windows cannot rename a file another process holds open, so there the
  rotation fails and the file keeps growing.
