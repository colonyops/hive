# A session rename renames its tmux session, keeping slug and tmux name in step

- **Status:** accepted
- **Date:** 2026-07-29

## Context

The desktop can now rename a hive session (#144). Hive's `RenameSession`
recomputes the slug from the new name and saves the record. The slug is also the
**tmux session name** — hive's spawner creates the tmux session as `slug`, and
every attach path in both products targets it: hive's TUI, `tmux attach -t
<slug>`, and this app's `tmuxcc` client map, control-plane routes, WebSocket
URL, `/terminal/:slug` route and resume snapshot.

So an unaccompanied rename leaves the stored slug naming a tmux session that
does not exist, and terminal attach silently fails from then on. The session
directory keeps its original slug in `Path`, so only the tmux name actually
diverges.

`internal/hivecore` is vendored read-only, so "stop re-slugging on rename" is not
available here — it would have to land in `colonyops/hive` and be re-vendored.

## Decision

**The invariant is `session.Slug == the live tmux session name`, and the desktop
maintains it.** `app.SessionsService.RenameSession` renames the tmux session and
then the record, in that order:

1. Validate the name and slugify it; a slug equal to the current one skips the
   tmux step entirely (a name-only change).
2. Reject a slug already held by another session. Hive validates the new name
   but checks no collision, and the session table has no uniqueness constraint,
   so nothing upstream prevents two sessions sharing one tmux name and one
   directory slug.
3. `tmuxcc.Manager.RenameSession` — probe with `has-session`, then
   `rename-session`. An absent tmux session (never spawned, or its server
   restarted) is success, not an error, and so is tmux being unusable: a hive
   session exists independently of terminal mode. Existence is *probed* rather
   than inferred from `rename-session`'s stderr, because the alternative is
   matching on tmux's message text.
4. Write the record. If that fails, the tmux rename is rolled back.

tmux goes first so the failures that are actually likely — a name collision, no
tmux — abort before anything is written, leaving one step that can fail and one
compensating action. The manager also drops the control client registered under
the old slug, so no live client keeps addressing a name tmux has dropped; the
frontend re-attaches under the new one.

The frontend follows the rename by **session id**: a reload whose attached slug
is missing looks the id up, re-navigating if it reappears under a new slug and
closing the attach only when the session is really gone. That covers a rename
made from the hive CLI as well as one made here, and it is the same rule that
handles a delete or recycle finishing as a job.

## Consequences

- Rename is safe for an attached session: the terminal reconnects under the new
  slug rather than breaking silently.
- Two writes are involved, so a rename is not atomic. The rollback narrows the
  window to a failed store write *and* a failed rollback, which is reported with
  the tmux name it was left at.
- `tmuxcc` now runs one-shot tmux commands as well as control clients. They go
  through the same `$TMUX` socket resolution as an attach, or they would address
  a different server.
- `Path` still keeps the slug the session was cloned under, so after a rename the
  directory name and the slug differ. That is hive's behaviour and nothing reads
  the path *as* a slug; it is left alone rather than compensated for, because
  moving a live worktree or clone is a much bigger operation than renaming a tmux
  session.
- Alternative rejected: decoupling attach from the slug via the
  `MetaTmuxSession` metadata key. Nothing writes that key today, so it would have
  to be backfilled; every slug-keyed surface in this app would need a second
  identity threaded through it; and hive's own attach would still be broken by a
  rename. The invariant is cheaper to keep than to remove.
