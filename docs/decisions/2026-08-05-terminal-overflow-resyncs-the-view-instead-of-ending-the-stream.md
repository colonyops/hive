# Terminal overflow resyncs the view instead of ending the stream

- **Status:** accepted
- **Date:** 2026-08-05

## Context

A session's broker bounds its undelivered backlog at 8 MiB, and crossing that
bound was fatal: the client was torn down, the slug freed, and an
`EXITED(overflow)` frame closed the socket (ADR
[terminal-transport](2026-07-28-terminal-transport.md) decision 4). The
frontend's only way forward is `reconnect()`, which disposes every tab and
attaches from scratch — so a burst of output cost the user their whole terminal
view: tabs, scroll position, focus.

Fatal-and-obvious was the right first call. A terminal byte stream cannot
drop-oldest without corrupting the emulator, and the alternative at the time
would have been a partial resync with no scrollback and no cursor restore to
build on. That is no longer true: a first paint is a pane's bounded scrollback,
its screen at full height and its cursor (ADR
[terminal-first-paint-carries-scrollback](2026-07-30-terminal-first-paint-carries-scrollback.md)),
and a repaint already runs that sequence mid-life against a client that never
detached, for the transport-drop case.

The bound itself is not the question. Raising it moves the volume at which this
happens without changing what happens.

## Decision

**Crossing the bound degrades the stream; it does not end it.** The broker's
overflow state stops the backlog dead and reports it once. `resync` then drops
the backlog and lifts that state **without touching the subscriber** — which is
the whole difference from `reset`, whose job is handing the snapshot to a
*replacement* subscriber. A pump parked on the head the resync removed reads a
backlog epoch alongside it, so it can tell "my event was dropped under me" from
"the backlog is empty because I emptied it" and stays attached either way.

**The repaint is attach's first paint, not a second implementation.** `Repaint`
and the overflow recovery are the same `paintEveryWindow` call over the same
paint gate; they differ only in what they do to the backlog first. That is what
makes the recovery non-lossy — the snapshot carries the pane's scrollback, so
the flood the user actually wants to read survives even though the bytes that
carried it did not.

**A new lifecycle kind, `degraded`, carries it on the wire.** It rides the
existing `0x02` control frame with the message `overflow`; no new frame type and
no wire-version bump, because frame parsing is unchanged and the kind vocabulary
is already open on both sides. It is an **edge, not a mode**: nothing clears it,
because the snapshot immediately behind it on the same stream *is* the recovery.

**The gap is stated in the UI, not hidden.** The frontend resets each pane's
emulator and its output writer on the frame — otherwise the snapshot's own
scrollback would replay lines the pane is already showing, with the gap between
them unmarked — and raises a dismissible notice saying output was dropped and
these panes were repainted from tmux. A recovery that hid the gap would be worse
than the teardown it replaces, which at least told the truth loudly.

**The fatal path stays reachable, on three conditions.** A resync already in
flight (the flood refilled the whole backlog inside one capture), a resync
inside `resyncCooldown` of the last one (the recovery did not hold), or a
repaint that errors (there is no control stream left to re-establish truth
from). All three end the stream with `EXITED(overflow)` exactly as before, and
the frontend's reconnect is unchanged.

This supersedes ADR
[terminal-transport](2026-07-28-terminal-transport.md) decision 4 in part: the
bound, the always-drain rule and the absence of tmux `pause-after` are
unchanged; what overflow *does* is not.

## Consequences

- **Overflow is now recoverable but still bounded.** The cooldown is what keeps
  a firehose from spending a `capture-pane` per window every few seconds to
  re-lose the same bytes. It is deliberately a time bound rather than an attempt
  count: what matters is whether the recovery held, not how many were tried.
- **A resync repaint runs while the pane is still producing.** The paint gate
  holds every pane for the length of the captures and replays the tail behind
  the snapshot, and that buffer is bounded in *time* (the capture's own timeout)
  rather than in bytes — as it already is on the attach path. A tail large
  enough to matter trips the broker bound the moment it is released, which is
  the fatal path doing its job, but a gate bound is the obvious next decision if
  that transient is ever measured to hurt.
- **Two frames now mean "you lost output"** — `degraded` and `exited(overflow)`
  — and the frontend does very different things with them. A change to one is
  not automatically a change to the other.
- The real-tmux test covers both halves in one run: a subscriber that reads
  nothing at all sees the resync happen and then sees the flood outrun it.
