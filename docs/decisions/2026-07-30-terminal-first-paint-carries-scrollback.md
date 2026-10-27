# First paint carries bounded scrollback and restores input state

- **Status:** accepted
- **Date:** 2026-07-30

## Context

A first paint of only the visible screen leaves everything a session did before
the attach unreachable and unsearchable, though tmux still holds it. For an
agent that has worked for hours, that is most of what there is to read.

Trimming the screen's trailing blank rows also breaks the alternate screen. An
emulator pins its viewport to the last rows written, so a short screen seats
the pane's row 0 partway down the viewport, and cursor-addressed output lands
that many rows off for the rest of the attach. This replaces decision 5 of ADR
terminal-transport.

## Decision

1. **A first paint is three commands per pane:** pane state
   (`display-message -p` with the cursor and `mouse_*_flag` formats), history
   (`capture-pane -pe -J -S -2000 -E -1`), then the visible screen
   (`capture-pane -pe -S 0`). The replay writes the history rows, the screen at
   exactly the pane's height, the pane's mouse modes, and a cursor-position
   escape. Nothing homes or clears first: history plus a full screen scrolls
   the history into scrollback by itself.

2. **The bound is 2000 lines** (`historyLines` in
   `internal/platform/tmux/control/client.go`), tmux's own default
   `history-limit`. Unbounded replay would make attach latency and broker
   pressure a property of someone's `.tmux.conf`. The frontend's xterm
   `scrollback` (5000) stays above it.

3. **The screen is written at exactly the pane height, and `-J` is used on the
   history only.** The height keeps the emulator's viewport and the pane's grid
   on the same rows. `-J` rejoins wrapped scrollback rows so search reads one
   line, but a joined screen row would re-wrap into more rows and break the
   height.

4. **The emulator stays in its normal buffer even when the program is on the
   alternate screen.** tmux keeps the scrollback in the normal buffer, and
   switching the emulator to its alternate buffer would discard it.

5. **Mouse reporting is restored from tmux's flags.** A program may have
   enabled it before this client attached, so its enable sequence is absent
   from every captured row. The replay emits the DECSET sequence after the
   screen.

6. **The cursor is read first, after the paint gate's mark.** Output produced
   between the read and the captures is then present in the replay that
   follows, which redraws it and carries the cursor along. Read before the
   mark, that output would be discarded and the position never corrected.

7. **Search is `@xterm/addon-search`, one addon per pane, one bar over the
   active tab.** The bar floats over the pane because a bar in the flex column
   would shrink the box this client votes tmux's size from. Switching tabs
   re-runs the query. It opens on `terminal.find` (Cmd+F on macOS, Ctrl+Shift+F
   elsewhere); a bare Ctrl+F stays readline's forward-char.

## Consequences

- Attach costs one history capture per pane, a few hundred KB at most against
  the 8 MiB per-session broker.
- Overflow still kills the client and a fresh attach is the resync (ADR
  terminal-transport). The pieces a non-destructive partial resync would need
  now exist; using them is a separate decision.
- The bound is a constant. A session with a much higher `history-limit`
  replays only its last 2000 lines.
