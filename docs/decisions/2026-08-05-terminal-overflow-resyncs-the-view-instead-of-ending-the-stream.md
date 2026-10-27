# Terminal overflow resyncs the view instead of ending the stream

- **Status:** accepted
- **Date:** 2026-08-05

## Context

A session's broker bounds its undelivered backlog at 8 MiB
(`defaultBufferBytes`), and the reader never pauses tmux
(ADR terminal-transport). A terminal byte stream cannot drop-oldest without
corrupting the emulator, so ending the stream on overflow was the safe default.
But the frontend's only recovery from an ended stream is `reconnect()`, which
throws away every tab, scroll position, and focus. A repaint can rebuild a pane
mid-life from its scrollback, screen, and cursor
(ADR terminal-first-paint-carries-scrollback), so overflow can be recovered
instead.

## Decision

**Crossing the bound degrades the stream; it does not end it.** The broker
stops the backlog and reports it once. `resync` drops the backlog and clears the
overflow state without replacing the subscriber (unlike `reset`). A backlog
epoch lets a pump parked on a dropped head tell "dropped under me" from "empty
because I drained it", so it stays attached.

**The repaint is attach's first paint.** Recovery calls the same
`paintEveryWindow` behind the same paint gate as `Repaint`. The snapshot carries
the pane's scrollback, so the output the user wants to read survives even though
the bytes that carried it did not.

**A `degraded` lifecycle event carries it on the wire**, on the existing `0x02`
control frame with message `overflow`. No new frame type or wire version. It is
an edge, not a mode: the snapshot right behind it is the recovery.

**The gap is shown in the UI.** The frontend resets each pane's emulator and
output writer on the frame (or the snapshot would replay lines already shown)
and raises a dismissible notice that output was dropped and the panes were
repainted.

**The fatal path stays** for three cases: a resync already in flight, a resync
within `resyncCooldown` (10s) of the last one, or a repaint that errors. Those
end the stream with `EXITED(overflow)` and the frontend reconnects.

## Consequences

- Overflow is recoverable but bounded. The cooldown is a time bound, not an
  attempt count, so a firehose cannot spend a `capture-pane` per window every
  few seconds.
- The paint gate buffers the tail behind the snapshot for the length of the
  captures, bounded in time, not bytes. A large tail trips the broker bound
  again and takes the fatal path.
- Two signals mean "you lost output", `degraded` and `exited(overflow)`, and the
  frontend handles them very differently. A change to one is not a change to the
  other.
