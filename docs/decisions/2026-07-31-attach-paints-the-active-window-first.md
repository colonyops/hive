# Attach paints the active window first and defers the rest

- **Status:** accepted
- **Date:** 2026-07-31

## Context

ADR terminal-first-paint-carries-scrollback established what a first paint is: a pane's bounded scrollback, its
screen at the window's full height, and its cursor. The attach sequence ran one
per window, synchronously, before `Attach` answered.

That made attach latency scale with a session's window count for panes the
renderer could not show. Measured against real tmux, on a session whose panes
hold a saturated 2000-line history of wide, coloured log lines:

| windows | attach | bytes published |
| --- | --- | --- |
| 1 | 11.6 ms | 294 KB |
| 4 | 33.7 ms | 1.18 MB |
| 8 | 62.1 ms | 2.35 MB |

The cost is almost entirely one command. Timed on an already-attached client,
per pane:

| command | time |
| --- | --- |
| `display-message` (cursor) | 37 µs |
| `capture-pane -pe -J` (history) | 6.56 ms |
| `capture-pane -pe` (screen) | 296 µs |
| `list-windows` | 54 µs |

`capture-pane` **with `-e`** is 7.5× the cost of the same capture without it
(6.56 ms vs 0.93 ms for identical rows): the price is tmux reconstructing SGR
sequences, and it scales linearly with scrollback depth at roughly 3.5 µs per
line. Dropping `-e` is not available — it is what makes replayed scrollback
keep its colour.

So the lever is not the command, it is how many panes run it before the user
can see anything. Only one window is on screen at a time; the other seven paints
were latency and bytes spent on tabs nobody had selected.

## Decision

**A fresh attach paints the window tmux reports as active, and answers. The
remaining windows are painted immediately afterwards on the client's own
lifetime.** `Repaint` takes the same shape.

Three things hold it together:

- **Deferred panes are held from before the synchronous paint starts**, not
  from when their capture is issued. `paintGate.rehold` takes the gate
  unconditionally — unlike `hold`, which declines a pane that is already live
  and is what the reconcile path needs. Output a deferred pane produces in the
  meantime buffers, and its mark is taken at capture time, so the replay still
  splits at "already inside the snapshot".
- **`openAll` skips a held pane**, keeping its buffer and its mark. Opening the
  gate while background captures are still in flight is now ordinary rather
  than impossible, and flushing a held pane there would put its own scrollback
  on the stream behind the live output that followed it.
- **`Repaint` waits for an in-flight deferred pass** before starting its own.
  Two passes interleaving would put two snapshots of one pane on the stream in
  an order neither chose.

The deferred pass runs under the client's lifetime context, not the attach
request's — the request's context is cancelled the moment `Attach` answers,
which is the point. Teardown cancels that context before joining the pass, so a
capture parked on a reply that will never arrive is released by cancellation
rather than waited out. Every path releases its panes: one left held buffers its
output for the life of the client and renders nothing.

## Consequences

Attach latency stops scaling with window count, and so does the volume the
renderer has to decode before a pane is readable:

| windows | attach before | after | allocated before | after |
| --- | --- | --- | --- | --- |
| 1 | 11.6 ms | 11.5 ms | 1.12 MiB | 1.12 MiB |
| 4 | 33.7 ms | 10.9 ms (−67.6%) | 4.11 MiB | 1.12 MiB (−72.8%) |
| 8 | 62.1 ms | 11.0 ms (−82.3%) | 8.10 MiB | 1.12 MiB (−86.1%) |

(`benchstat`, n=5, p=0.008 for both improved cases.)

What is given up: a background tab's scrollback is no longer guaranteed to be
on the stream by the time `Attach` returns. It arrives within the deferred pass
— milliseconds later, and still before a user could plausibly switch tabs — but
a caller that assumed "attached implies every window painted" no longer holds.
The `attached` lifecycle event now precedes the deferred paints rather than
following all of them, which is what `TestAttachFirstPaintsEachWindow` had to
be re-pointed at.

A session with many windows now has a window between attach and the last
deferred paint during which a background pane is buffering rather than
streaming. That buffer is unbounded within the pass, bounded in practice by the
pass being bounded by `attachTimeout`, and the broker's own byte and event
bounds still backstop the whole stream.
