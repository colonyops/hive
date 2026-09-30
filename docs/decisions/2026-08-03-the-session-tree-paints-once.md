# The session tree paints once, from one tmux call

- **Status:** accepted
- **Date:** 2026-08-03

## Context

Entering terminal mode filled the sidebar tree in waves. The session list landed
first and every group and row animated in; the per-session window listings
landed second and every subtree animated open under them; the appearance setting
that decides whether those subtrees show at all hydrated separately and could
retract them again; the restored session's attach swapped its listed rows for
live ones. Each wave relayouted the whole panel.

Two things ran on top of that. The listing sweep asked per slug, and
`Manager.ListWindows` answered an unattached session with a `has-session` probe
and a `list-windows` — two process spawns per row, fanned out with
`Promise.all`, so the sidebar spawned twice as many tmux processes as it had
sessions in one burst, timed to land on the frames the tree was painting in.
And the selection rails re-measure per animation frame for 260 ms after any
layout change, driven by a `ResizeObserver` that fires per frame while a height
animates — so the chase loop forced synchronous layout continuously for the
whole fill, each wave extending its deadline.

## Decision

1. **One tmux call answers the whole sidebar.** `Manager.ListAllWindows` runs
   `list-windows -a` once for the server and buckets the rows by session name.
   A slug tmux has no session for is simply absent from the output, which is
   the answer the `has-session` probe was being paid for. Attached slugs still
   answer from their live control client, which is authoritative and already in
   memory.

   `POST /api/terminal/windows/list` takes `slugs` and answers `sessions` keyed
   by slug. The per-slug route and `Manager.ListWindows` are deleted rather than
   kept alongside it: the sweep was their only caller, and the bulk form
   subsumes them.

2. **The tree holds its first paint until a row's final shape is known** — the
   session list, the window listings, and the setting that decides whether the
   listings render. The placeholder that was already there covers the wait. The
   gate is one-shot: a later reload revalidates the tree on screen, and toggling
   the window listing must not blank it.

3. **Motion is suppressed for the first fill.** On the first paint every row is
   an enter, so the transitions animate the panel in as one block. They exist to
   make a *change* legible, and a first fill is not one. The rails' chase loop
   is skipped for the same window — nothing is moving to chase, and it forced
   layout every frame.

4. **The session list goes out with the availability probe, not behind it.**
   `ListSessions` is a SQLite read that knows nothing about tmux; awaiting it
   after `Available` and `Endpoint` put three serial round trips in front of the
   first row.

## Consequences

- **A sweep is one round trip and one process.** It was `2N` processes and `N`
  round trips.
- **`listWindows` on the frontend client takes a slug array and answers a map.**
  Callers that want one session ask for one.
- **The tree can now show `Loading…` where it used to show rows.** It shows the
  placeholder slightly longer and the finished tree instead of a partial one;
  a cold entry still gates on `Checking tmux…` ahead of that.
- **`useTerminalSessions` exposes `loaded` and `useTerminalShowWindows` exposes
  `ready`.** `loading` cannot say whether a read has ever finished — it is false
  both before the first one starts and after it lands — and the gate needs the
  difference. Anything else that must not paint before its data is real should
  use these rather than infer readiness from an empty list.
- **A sweep that fails still settles.** Otherwise the tree waits on an answer
  that is never coming and never paints; the last-known listings are kept.
