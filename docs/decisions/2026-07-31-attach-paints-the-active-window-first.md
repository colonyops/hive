# Attach paints the active window first and defers the rest

- **Status:** accepted
- **Date:** 2026-07-31

## Context

A first paint (ADR terminal-first-paint-carries-scrollback) runs a history
`capture-pane -pe -J` per pane, and `-e` is expensive: tmux rebuilds SGR
sequences, at a cost linear in scrollback depth. `-e` cannot be dropped, since
it is what keeps replayed scrollback in colour. Painting every window before
`Attach` answered made attach latency scale with window count, for panes nobody
could see yet.

## Decision

**A fresh attach paints the visible panes of the window tmux reports as active,
and answers. Every other pane is painted right after, on the client's own
lifetime.** `Repaint` takes the same shape (`internal/platform/tmux/control/client.go`).

- **Deferred panes are held before the synchronous paint starts.**
  `holdPanes` calls `paintGate.rehold`, which takes the gate unconditionally,
  unlike `hold`, which declines a pane already live. Output a deferred pane
  produces meanwhile buffers, and its mark is taken at capture time, so the
  replay still splits at "already inside the snapshot".
- **`openAll` skips a held pane**, keeping its buffer and mark. Flushing it
  there would put its scrollback on the stream behind the live output that
  followed it.
- **`Repaint` waits for an in-flight deferred pass** (`bgPaint`), so two
  snapshots of one pane never interleave.

The deferred pass runs under the client's lifetime context, because the attach
request's context ends when `Attach` answers. Teardown cancels that context
before joining the pass, so a capture parked on a reply that never arrives is
released. Every path releases its panes: a pane left held buffers its output
for the life of the client and renders nothing.

## Consequences

- A background tab's scrollback is not guaranteed to be on the stream when
  `Attach` returns, and the `attached` lifecycle event precedes the deferred
  paints. A caller must not assume "attached" means every window painted.
- Between attach and the last deferred paint, a background pane buffers rather
  than streams. The pass is bounded by `attachTimeout`, and the broker's byte
  and event bounds backstop the stream.
