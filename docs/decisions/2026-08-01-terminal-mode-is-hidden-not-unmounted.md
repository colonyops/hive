# Terminal mode is hidden on a trip to the hub, never unmounted

- **Status:** accepted
- **Date:** 2026-08-01

## Context

The attach pool (ADR terminal-attach-pool) keeps sessions live across a switch,
but unmounting terminal mode on a trip to the hub disposes the whole pool. Every
return then pays a cold attach per pooled session, and leaving pays the
teardown synchronously on the click. A `Terminal` is bound to the element it
was opened on and cannot outlive it.

## Decision

**Terminal mode is mounted the first time it is entered and never unmounted.**
`useAppMode` latches `terminalMounted`, and `App.vue` renders the mode with
`v-show` beside the hub rather than as a branch of it. Leaving the mode is a
`display` flip; the pool, its control clients, WebSockets and xterm screens stay
live. The first mount stays lazy, so the glyphs and the xterm chunk stay out of
the initial bundle.

- **`active` replaces mount and unmount for anything that must not run
  off-screen.** The mode takes it as a prop. Going true, it revalidates the
  availability probe and the session list, because the hub is where sessions
  are created, renamed and deleted. Going false, it stops the session-status
  poll and the window-listing sweep, both of which spawn tmux. The pool is
  untouched either way.
- **Each mode's toggle returns to the route that mode was last on**, so coming
  back does not pass through a picker route that would detach the pool entry it
  is about to show.

## Consequences

- Once opened, the app holds up to `terminal_pool_size` control clients for the
  rest of its life, and their streams keep draining.
- A hidden pane has no box, so no size vote is sent while the hub is up. The
  pane's `ResizeObserver` re-votes when the mode comes back.
- The hub is still unmounted while the terminal is up; that costs a re-render,
  not a re-attach.
- Terminal mode's view state (expanded rows, the find bar) survives a trip to
  the hub, on purpose.
