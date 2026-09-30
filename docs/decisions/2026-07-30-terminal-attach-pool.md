# Terminal view pools live attaches and swaps sessions on first paint

- **Status:** accepted
- **Date:** 2026-07-30

## Context

Switching terminal sessions disposed the outgoing attach the moment a row was
clicked, then paid the full cold-attach cost before anything painted: spawn
`tmux -CC attach`, handshake, per-window `capture-pane`, socket open, first-paint
replay, xterm parse. The pane showed blank/"Attaching…" for the whole gap, and
the effect is worst for exactly the sessions this product exists for —
alternate-screen TUIs like a running agent, whose first paint is a full screen
of content the user watches rebuild. Snapping back to a session just left paid
it all again, because every switch also detached the control client.

`tmuxcc.Manager` already keeps one independent control client per slug and
reuses a live one on re-attach; only the frontend's detach-on-switch forced the
rebuild.

## Decision

1. **The terminal view keeps an LRU pool of live attaches**
   (`appearance.terminal_pool_size`, 1–6, default 3; healed by the frontend
   like the other appearance values, applied live on change). Switching
   sessions no longer detaches: the outgoing session keeps its control client,
   WebSocket and xterm terminals, with its panes hidden (`v-show`). The stream
   keeps draining while hidden — which also keeps the broker's overflow bound
   at bay — so snapping back is a v-show flip of an already-current screen.
   Detach now happens on pool eviction, explicit close, the session leaving
   the listing (deleted/recycled/renamed), and view unmount. ADR terminal-transport's rule
   that intentional teardown releases the control client stands; a switch is
   no longer a teardown.

2. **A cold attach holds the outgoing screen until the incoming one has
   something to show.** The pane swaps when the incoming session reports its
   first processed output (`painted`, flipped by xterm's write callback —
   `live` is too early, the socket opens before the capture replay lands), or
   ends, or a 300 ms cap fires. The cap exists because an empty screen sends
   no first paint at all, and because a genuinely slow attach must not read as
   a dead click. The sidebar selection moves immediately; only the pane lags.

## Consequences

- The app holds up to 3 tmux control clients, 3 WebSockets and their xterm
  buffers per window; hidden sessions cost parsing for whatever they print.
  Each window's atlas renderer holds a WebGL context, and browsers cap those —
  the existing context-loss fallback to the canvas renderer is the backstop,
  and the pool limit is deliberately small.
- Keystrokes during a hold go to the still-focused outgoing session for up to
  300 ms. Accepted: the alternative is focusing a hidden element, which no
  browser honours.
- tmux sees this app as an attached client on up to 3 sessions, not 1. Size
  votes are unaffected — every pooled session was voted by the same pane box.
- A pooled hidden session that dies (overflow, tmux exit) is re-attached fresh
  the next time it is selected; nothing repairs it in the background.
