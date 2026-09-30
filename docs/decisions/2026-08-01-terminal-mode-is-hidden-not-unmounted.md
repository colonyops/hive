# Terminal mode is hidden on a trip to the hub, never unmounted

- **Status:** accepted
- **Date:** 2026-08-01

## Context

ADR terminal-attach-pool stopped a session switch from detaching, so the pane no longer blanks
while a cold attach runs. The mode boundary was left out of that: `App.vue`
rendered terminal mode as a branch of the shell's `v-if` chain, so the Hub
toggle unmounted it and `onBeforeUnmount` disposed the whole pool. Every trip
back paid, per pooled session, exactly what ADR terminal-attach-pool exists to avoid — spawn
`tmux -C attach`, handshake, `capture-pane`, socket open, scrollback replay
into a fresh emulator, `term.open()` re-measure, WebGL context claim — and the
trip *out* paid the mirror image of it synchronously on the click, disposing
every `Terminal` and its renderer before the hub could render.

The unmount had already been patched around three times, each time in the same
shape: `useTerminalAvailability`, `useTerminalSessions` and
`useTerminalWindowListings` all became module singletons so that the probe
answer, the session tree and the window listings would survive it. None of
that could cover the pool, because a `Terminal` is bound to the element it was
opened on and cannot outlive it.

`PopupTerminal` already had the rule this needed (ADR ephemeral-popup-terminals): mounted on first
use, hidden thereafter, because hiding a terminal must not end the shell
running inside it.

## Decision

**Terminal mode is mounted the first time it is entered and is never
unmounted.** `App.vue` holds a `terminalMounted` latch and renders the mode
with `v-show`, beside the hub rather than as a branch of it. Leaving the mode
is a `display` flip; the pool, its tmux control clients, their WebSockets and
their xterm screens are all still live when it comes back. The lazy first
mount is unchanged — the ~10 MB of glyphs and the xterm chunk still stay out of
the initial bundle until someone opens the mode.

Two things follow from mount no longer being the entry event:

- **`active` replaces mount/unmount for anything that must not run
  off-screen.** The mode takes it as a prop and, on it going true,
  revalidates — the availability probe and the session list, because the hub is
  where a session gets created, renamed or deleted. On it going false it stops
  the session-status poll and the per-session window-listing sweep, both of
  which spawn tmux and neither of which is on screen. The pool is untouched
  either way; `onBeforeUnmount` still disposes it, and now only ever runs on
  shell teardown.
- **Each mode's toggle returns to the route that mode was last on.** The hub
  side already did this; the terminal side went to the bare picker route and
  let the resume snapshot navigate off it, which detached the pool entry on the
  way past and blanked the pane it was about to show.

## Consequences

- Once the mode has been opened, the app holds up to `terminal_pool_size` tmux
  control clients for the rest of its life, not just for as long as the mode is
  on screen — and their streams keep draining, which is what keeps the broker's
  overflow bound at bay. This extends ADR terminal-attach-pool's consequence, it does not
  change its bound: the pool limit is the cap either way.
- A hidden pane has no box, so `voteSize` sends nothing while the hub is up. A
  window resized during that time is re-voted by the pane's own
  `ResizeObserver` when the mode comes back.
- The hub subtree is still unmounted while the terminal is up. That side costs
  a re-render, not a re-attach, and keeping two live reactive subtrees over the
  hours a terminal stays open is the worse trade.
- Terminal mode's own view state — expanded rows, the find bar, the loaded
  action catalog — now survives a trip to the hub. That is the intended
  behaviour, not a side effect to design around.
