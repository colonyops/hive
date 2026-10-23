# A session rename renames its tmux session, keeping slug and tmux name in step

- **Status:** accepted
- **Date:** 2026-07-29

## Context

Renaming a hive session (#144) recomputes its slug from the new name. The
slug is also the tmux session name: the spawner creates the tmux session as
`slug`, and every attach path targets it (the CLI TUI, `tmux attach -t
<slug>`, and the desktop's control clients, terminal routes, WebSocket URL,
`/terminal/:slug` route and resume snapshot). A rename that leaves tmux alone
makes the slug name a tmux session that does not exist, and attach silently
fails.

## Decision

**The invariant is that the session's slug is its live tmux session name,
and `MetaTmuxSession` records the actual tmux target.** The shared
`internal/hive/session` service renames through its injected multiplexer
lifecycle and writes `MetaTmuxSession`; when the tmux rename fails it keeps
the old target there so lifecycle operations still find it.

Desktop's `app.SessionsService.RenameSession` adds a preflight, in this order:

1. Validate and slugify the name. A slug equal to the current one skips tmux.
2. Reject a slug another session holds. Hive checks no collision and the
   session table has no uniqueness constraint.
3. `control.Manager.RenameSessionIfPresent` probes the current target with
   `has-session`, runs `rename-session`, and drops the control client
   registered under the old slug. An absent tmux session or unusable tmux is
   success: a hive session exists independently of terminal mode. Existence
   is probed rather than read from tmux's stderr text.
4. Ask hive to write the record. Its lifecycle call sees the source already
   renamed and succeeds. If the write fails, Desktop renames tmux back, but
   only when step 3 changed a live session.

tmux goes first so the likely failures (a collision, no tmux) abort before
anything is written.

The frontend follows a rename by session id: when the attached slug
disappears on reload, it looks up the id and re-navigates if the session
reappears under a new slug. That covers a rename from the CLI too.

## Consequences

- Rename is two writes and not atomic. A failed store write plus a failed
  rollback is reported with the tmux name it was left at.
- One-shot tmux commands go through the same `$TMUX` socket resolution as an
  attach, or they would address a different server.
- `Path` keeps the slug the session was cloned under, so after a rename the
  directory name and the slug differ. Nothing reads the path as a slug, and
  moving a live checkout is a much bigger operation.
- Desktop keeps `MetaTmuxSession` out of its frontend DTOs but uses it for
  status, collision checks and rename repair.
