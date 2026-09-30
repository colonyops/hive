# Queued terminal output coalesces in the broker

- **Status:** accepted
- **Date:** 2026-07-31

## Context

tmux caps one `%output` notification at 2 KB. Measured against real tmux, a
pane catting a large coloured log produces **14,500 events per second** at
19.4 MB/s, mean 1397 B per event.

Each of those became one `Output` event, one WebSocket frame, one webview
message task, one `decodeFrame`, and one `term.write`. Every fixed per-frame
cost in that chain — and the browser's own per-message allocation and event
dispatch, which no JS profiler attributes to us — was being multiplied by
14,500.

Batching on a timer was the obvious shape and the wrong one: a delay added to
every frame is a delay added to keystroke echo, which is the one thing a
terminal must not make slower.

## Decision

**The broker folds an incoming `Output` into the last queued event when both
are output for the same pane.** Merging happens in `publish`, under the lock
the backlog is already maintained under.

The property that makes this safe to do unconditionally is that **a merge is
only possible when a second event is already waiting** — which means the
subscriber is behind. A consumer keeping up sees a backlog of at most one and
therefore every event whole, at its original size, with nothing added to its
latency. The batching window is not a duration anyone tuned; it is exactly how
far behind the consumer already is.

Three bounds:

- **Never into the head.** `pump` reads `buf[0]`, unlocks, and hands its own
  copy to the subscriber, then charges the backlog the size it read. Growing
  the head after that would both lose the appended bytes and corrupt the byte
  accounting, so merging requires `len(buf) >= 2`.
- **Never across panes.** A frame carries one window id; merging across them
  would deliver one pane's bytes to another's emulator.
- **64 KB per merged frame**, so a stalled subscriber cannot be handed one
  enormous frame.

The merge copies into a fresh buffer rather than appending onto the tail's
slice, which can alias one the paint gate or the decoder still owns. The
tail's timestamp is kept rather than the incoming one, so the latency the
adapter reports stamps the oldest byte in the frame and stays a worst case.

## Consequences

Measured on the same burst, varying only how long the consumer spends per
frame:

| consumer cost/frame | frames | mean frame |
| --- | --- | --- |
| 0 (keeps up) | 586 | 1468 B |
| 100 µs | 229 (−61%) | 3758 B |
| 500 µs | 56 (−90%) | 15368 B |

Byte totals are identical across all three. The faster the consumer, the less
this does — which is the intended shape, and the reason it needs no
configuration.

The contract a subscriber gets is now explicitly a **byte stream**, not a
sequence of frames matching what was published. Tests that counted events had
to be re-pointed at the bytes; `TestBrokerCoalescesQueuedOutputWithoutLosingBytes`
and `TestBrokerNeverCoalescesAcrossWindows` pin the invariants that actually
matter.

Two things this does not change. Byte accounting is unchanged, so the overflow
bound still trips at the same volume — coalescing reduces frames, never the
bytes the backlog is charged. And a subscriber that connects to a backlog built
with no subscriber at all still sees unmerged events, because the first two
publishes cannot merge.
