# The session tree paints once, from one tmux call

- **Status:** accepted
- **Date:** 2026-08-03

## Context

The terminal sidebar tree filled in waves: the session list, then each
session's window listing, then the setting that decides whether listings show,
then the restored session's live rows. Each wave relayouted the panel. A
per-slug window listing cost a `has-session` and a `list-windows` per row, and
the selection rails forced layout every animation frame while heights animated.

## Decision

1. **One tmux call answers the whole sidebar.** `Manager.ListAllWindows`
   (`internal/platform/tmux/control`) runs `list-windows -a` once and buckets
   rows by session. A slug with no tmux session is absent from the answer.
   Attached slugs answer from their live control client.
   `POST /api/terminal/windows/list` takes `slugs` and answers `sessions` keyed
   by slug.
2. **The tree holds its first paint until every row's final shape is known**:
   the session list, the window listings, and the show-windows setting. The
   gate is one-shot; later reloads revalidate on screen.
3. **Motion and the rails' chase loop are off for the first fill.** On a first
   paint every row is an enter, and transitions exist to make a change legible.
4. **The session list loads in parallel with the tmux availability probe**,
   not behind it. It is a SQLite read.

## Consequences

- `useTerminalSessions` exposes `loaded` and `useTerminalShowWindows` exposes
  `ready`, because `loading` cannot say whether a read has ever finished.
  Anything that must not paint before its data is real should use these
  instead of inferring readiness from an empty list.
- A sweep that fails still settles and keeps the last-known listings, or the
  tree would never paint.
