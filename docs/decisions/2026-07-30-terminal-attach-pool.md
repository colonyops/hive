# Terminal view pools live attaches and swaps sessions on first paint

- **Status:** accepted
- **Date:** 2026-07-30

## Context

A cold attach to a tmux session is slow: spawn the control client, handshake,
`capture-pane` per window, open the socket, replay the first paint, parse it
in xterm. Detaching on every switch paid all of that again on the way back,
and the gap is worst for alternate-screen TUIs like a running agent.
`tmuxcc.Manager` already reuses a live control client per slug; only a
frontend detach forced the rebuild.

## Decision

1. **The terminal view keeps an LRU pool of live attaches**
   (`appearance.terminal_pool_size`, 1-6, default 3, applied live;
   `useTerminalPool.ts`). A switch does not detach: the outgoing session keeps
   its control client, WebSocket and xterm terminals with its panes hidden, and
   its stream keeps draining. Detach happens on pool eviction, explicit close,
   the session leaving the listing, and view unmount. A switch is not a
   teardown in the sense of ADR terminal-transport.

2. **A cold attach holds the outgoing screen until the incoming one has
   something to show.** The pane swaps when the incoming session reports its
   first processed output (`painted`, from xterm's write callback; `live` is
   too early because the socket opens before the capture replay lands), or
   ends, or `HOLD_MS` (300 ms) passes. The cap exists because an empty screen
   sends no first paint. The sidebar selection moves at once; only the pane
   lags.

## Consequences

- Each pooled window's renderer holds a WebGL context and browsers cap those,
  which is why the ceiling is small. The context-loss fallback to the canvas
  renderer is the backstop.
- Keystrokes during a hold go to the still-focused outgoing session for up to
  300 ms, because a hidden element cannot take focus.
- tmux sees this app attached to up to pool-size sessions. Size votes are
  unaffected, since every pooled session was voted by the same pane box.
- A pooled hidden session that dies is re-attached fresh the next time it is
  selected; nothing repairs it in the background.
